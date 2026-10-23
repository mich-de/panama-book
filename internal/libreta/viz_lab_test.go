//go:build vizlab

package libreta

// Laboratorio del VIZ (solo sviluppo, build tag vizlab): prova varianti di ritaglio e di lettura su tutto il corpus con la
// verita' (scans\verita), senza passare dall'intero Extract. Stampa solo conteggi e, per i campi non personali (qualifica),
// le letture. Eseguire con: go test -tags vizlab -run Lab -v -timeout 60m ./internal/libreta
// La rotazione di ogni immagine si legge dalla baseline dell'harness (docs\baseline-regressione.tsv).

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/ocr"
	"scanlibretto/internal/verify"
)

type labImg struct {
	Name, Path string
	Rot        int
	Truth      verify.Truth
	g          *image.Gray
	geo        vizGeom
	ok         bool
}

var (
	labMu    sync.Mutex
	labCache = map[string]*labImg{}
)

// labList legge l'elenco delle immagini (percorso, rotazione) dalla baseline dell'harness; salta le senza pagina dati
// (verita' tutta vuota).
func labList(t *testing.T, onlyWithField string) []*labImg {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "baseline-regressione.tsv"))
	if err != nil {
		t.Skip("baseline assente")
	}
	var out []*labImg
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(l, "#") || strings.HasPrefix(l, "file\t") || strings.TrimSpace(l) == "" {
			continue
		}
		c := strings.Split(l, "\t")
		path := filepath.Join("..", "..", filepath.FromSlash(c[0]))
		rot, _ := strconv.Atoi(c[1])
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		tr, err := verify.LoadTruth(filepath.Join("..", "..", "scans", "verita", name+".json"))
		if err != nil {
			continue
		}
		if onlyWithField != "" && tr.Fields[onlyWithField] == "" {
			continue
		}
		out = append(out, &labImg{Name: name, Path: path, Rot: rot, Truth: tr})
	}
	return out
}

// labPrep carica l'immagine come Extract (ritaglio, raddrizzamento, rotazione) e trova la MRZ.
func labPrep(t *testing.T, li *labImg) {
	t.Helper()
	labMu.Lock()
	if c, ok := labCache[li.Path]; ok {
		li.g, li.geo, li.ok = c.g, c.geo, c.ok
		labMu.Unlock()
		return
	}
	labMu.Unlock()
	src, err := imgproc.Load(li.Path)
	if err != nil {
		t.Fatal(err)
	}
	base := imgproc.CropContent(src, 10)
	base, _ = imgproc.Deskew(base)
	g := vizToGray(imgproc.Rotate(base, li.Rot))
	geo, ok := vizLocateMRZ(g)
	li.g, li.geo, li.ok = g, geo, ok
	labMu.Lock()
	labCache[li.Path] = &labImg{g: g, geo: geo, ok: ok}
	labMu.Unlock()
}

// labOCR esegue l'OCR di tanti ritagli (in grigi) con una sola chiamata e restituisce le parole di ciascuno.
func labOCR(t *testing.T, crops []*image.Gray) [][]word {
	t.Helper()
	dir, err := os.MkdirTemp("", "vizlab")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	var paths []string
	for i, g := range crops {
		p := filepath.Join(dir, fmt.Sprintf("c%04d.png", i))
		if err := imgproc.SavePNG(g, p); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	out := make([][]word, len(crops))
	const batch = 250
	for a := 0; a < len(paths); a += batch {
		b := min(len(paths), a+batch)
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		rs, err := ocr.RecognizeMany(ctx, paths[a:b])
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		for i, r := range rs {
			out[a+i] = vizWords(r)
		}
	}
	return out
}

type labVar struct {
	Name   string
	V      vizVariant
	Ox, Oy float64
}

// labBandCrops costruisce il ritaglio del campo b per ogni variante.
func labBandCrops(li *labImg, b vizBand, vars []labVar) ([]*image.Gray, []float64) {
	var crops []*image.Gray
	var expY []float64
	for _, lv := range vars {
		vals, w, h := vizPrepCrop(li.g, li.geo, vizBandRect(li.geo, b, lv.V, lv.Ox, lv.Oy), lv.V, 0, 0)
		if vals == nil {
			crops = append(crops, image.NewGray(image.Rect(0, 0, 16, 16)))
			expY = append(expY, 0)
			continue
		}
		up, _ := vizWindow(b, lv.V)
		crops = append(crops, vizPadGray(vals, w, h))
		expY = append(expY, float64(vizCropMargin)+(up-lv.Oy)*lv.V.unit)
	}
	return crops, expY
}

func labBand(key string) vizBand {
	for _, b := range vizBands {
		if b.key == key {
			return b
		}
	}
	panic("fascia sconosciuta " + key)
}

// labThr e' la soglia adattiva dell'immagine (come in vizBuildSheet).
func labThr(li *labImg) int {
	thr, _ := vizAdaptiveThreshold(li.g, li.geo)
	return thr
}

// labBaseVars: le varianti di produzione (base + aggiuntive + jitter) per l'immagine.
func labBaseVars(li *labImg) []labVar {
	var vs []labVar
	for i, v := range vizVariants {
		vs = append(vs, labVar{Name: fmt.Sprintf("v%d", i), V: v})
	}
	thr := labThr(li)
	extra := vizExtraVariants(thr)
	for k, v := range extra {
		vs = append(vs, labVar{Name: fmt.Sprintf("x%d", k), V: v})
	}
	if extra != nil {
		for k, j := range vizJitterJobs(thr) {
			vs = append(vs, labVar{Name: fmt.Sprintf("j%d", k), V: j.v, Ox: j.ox, Oy: j.oy})
		}
	}
	return vs
}

func labSortedKeys(m map[string]int) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// labHasRankCode: la qualifica termina con una sigla di grado (numerale romano/cifra, es. "II/2").
func labHasRankCode(s string) bool {
	f := strings.Fields(strings.ToUpper(s))
	if len(f) == 0 {
		return false
	}
	last := f[len(f)-1]
	return strings.ContainsAny(last, "/") && strings.ContainsAny(last, "IVX0123456789")
}

func TestLabCapacity(t *testing.T) {
	imgs := labList(t, "capacity")
	b := labBand("cargo")
	type stat struct{ ok, n int }
	perVar := map[string]*stat{}
	perVarCode := map[string]*stat{}
	var voteOK, voteN, voteCodeOK, voteCodeN int
	for _, li := range imgs {
		labPrep(t, li)
		if !li.ok {
			fmt.Printf("%-40s MRZ non localizzata\n", li.Name)
			continue
		}
		vars := labBaseVars(li)
		crops, expY := labBandCrops(li, b, vars)
		ws := labOCR(t, crops)
		want := li.Truth.Fields["capacity"]
		code := labHasRankCode(want)
		var cands []vizCand
		var line []string
		for i, lv := range vars {
			tmp := &Result{Fields: map[string]string{}}
			vizReadBand(b, ws[i], expY[i], tmp)
			got := tmp.Fields["cargo"]
			cands = append(cands, vizCand{id: i, f: tmp.Fields})
			st, sc := perVar[lv.Name], perVarCode[lv.Name]
			if st == nil {
				st, sc = &stat{}, &stat{}
				perVar[lv.Name], perVarCode[lv.Name] = st, sc
			}
			target := st
			if code {
				target = sc
			}
			target.n++
			if verify.Equal("capacity", got, want) {
				target.ok++
			}
			if got != "" {
				line = append(line, got)
			}
		}
		out := &Result{Fields: map[string]string{}}
		vizVote(b, cands, func(int) float64 { return 1 }, out)
		got := out.Fields["cargo"]
		okv := verify.Equal("capacity", got, want)
		if code {
			voteCodeN++
			if okv {
				voteCodeOK++
			}
		} else {
			voteN++
			if okv {
				voteOK++
			}
		}
		fmt.Printf("%-40s code=%v voto=%v verita=%q voto=%q\n", li.Name, code, okv, want, got)
		if os.Getenv("LAB_V") != "" {
			fmt.Printf("      letture: %q\n", line)
		}
	}
	fmt.Printf("voto: senza sigla %d/%d, con sigla %d/%d\n", voteOK, voteN, voteCodeOK, voteCodeN)
	for _, k := range labSortedKeys(map[string]int{"v0": 0, "v1": 0, "v2": 0, "x0": 0, "x1": 0, "x2": 0, "x3": 0, "x4": 0, "x5": 0, "x6": 0, "x7": 0, "x8": 0, "x9": 0, "x10": 0, "j0": 0, "j1": 0, "j2": 0, "j3": 0, "j4": 0, "j5": 0, "j6": 0, "j7": 0}) {
		if s := perVar[k]; s != nil {
			fmt.Printf("variante %-4s senza sigla %2d/%2d  con sigla %2d/%2d\n", k, s.ok, s.n, perVarCode[k].ok, perVarCode[k].n)
		}
	}
}

// TestLabDump salva i ritagli di una fascia per alcune immagini (LAB_IMGS=nome1,nome2 LAB_BAND=cargo) in _out\lab\.
func TestLabDump(t *testing.T) {
	names := strings.Split(os.Getenv("LAB_IMGS"), ",")
	key := os.Getenv("LAB_BAND")
	if key == "" || names[0] == "" {
		t.Skip("LAB_IMGS e LAB_BAND")
	}
	b := labBand(key)
	dir := filepath.Join("..", "..", "_out", "lab")
	_ = os.MkdirAll(dir, 0o755)
	for _, li := range labList(t, "") {
		found := false
		for _, n := range names {
			if li.Name == n {
				found = true
			}
		}
		if !found {
			continue
		}
		labPrep(t, li)
		vars := labBaseVars(li)
		crops, _ := labBandCrops(li, b, vars)
		for i, c := range crops {
			if err := imgproc.SavePNG(c, filepath.Join(dir, fmt.Sprintf("%s_%s_%s.png", li.Name, key, vars[i].Name))); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestLabRaw stampa l'OCR grezzo (righe e parole con riquadri) dei ritagli di una fascia per un'immagine.
func TestLabRaw(t *testing.T) {
	name := os.Getenv("LAB_IMG")
	key := os.Getenv("LAB_BAND")
	if name == "" || key == "" {
		t.Skip("LAB_IMG e LAB_BAND")
	}
	b := labBand(key)
	for _, li := range labList(t, "") {
		if li.Name != name {
			continue
		}
		labPrep(t, li)
		vars := labBaseVars(li)
		crops, _ := labBandCrops(li, b, vars)
		ws := labOCR(t, crops)
		for i, lv := range vars {
			var parts []string
			for _, w := range ws[i] {
				parts = append(parts, fmt.Sprintf("%q@%.0f,%.0f(%.0fx%.0f)", w.text, w.x, w.y, w.w, w.h))
			}
			fmt.Printf("%-4s %dx%d  %s\n", lv.Name, crops[i].Bounds().Dx(), crops[i].Bounds().Dy(), strings.Join(parts, " "))
		}
	}
}

// labGrid: griglia di varianti (scala x soglia x larghezza della rampa x assottigliamento) per la fascia indicata.
func labGrid(units []float64, thrs []int, thin []bool, win [2]float64) []labVar {
	var vs []labVar
	for _, u := range units {
		for _, th := range thrs {
			for _, tn := range thin {
				vs = append(vs, labVar{Name: fmt.Sprintf("u%.0f-t%d-th%v", u, th, tn), V: vizVariant{u, win[0], win[1], tn, th}})
			}
		}
	}
	return vs
}

func labSim(a, b string) float64 {
	a, b = verify.Norm(a), verify.Norm(b)
	if a == "" && b == "" {
		return 1
	}
	n := max(len([]rune(a)), len([]rune(b)))
	return 1 - float64(vizLev(a, b))/float64(n)
}

func TestLabGrid(t *testing.T) {
	key := os.Getenv("LAB_BAND")
	if key == "" {
		key = "cargo"
	}
	field := map[string]string{"cargo": "capacity", "luogo_nascita": "place_of_birth", "nacionalidad": "nationality", "regionale": "regional"}[key]
	b := labBand(key)
	imgs := labList(t, field)
	type agg struct {
		ok, n int
		sim   float64
	}
	res := map[string]*agg{}
	resCode := map[string]*agg{}
	var order []string
	vars := labGrid([]float64{1300, 1650, 1950, 2600}, []int{0, 40, 70, 100, 130, 160, 200}, []bool{false, true}, [2]float64{0.53, 0.53})
	for _, li := range imgs {
		labPrep(t, li)
		if !li.ok {
			continue
		}
		crops, expY := labBandCrops(li, b, vars)
		ws := labOCR(t, crops)
		want := li.Truth.Fields[field]
		code := labHasRankCode(want)
		for i, lv := range vars {
			tmp := &Result{Fields: map[string]string{}}
			vizReadBand(b, ws[i], expY[i], tmp)
			got := tmp.Fields[key]
			m := res
			if code {
				m = resCode
			}
			a := m[lv.Name]
			if a == nil {
				a = &agg{}
				m[lv.Name] = a
				if !code {
					order = append(order, lv.Name)
				}
			}
			a.n++
			if verify.Equal(field, got, want) {
				a.ok++
			}
			a.sim += labSim(got, want)
		}
	}
	for _, v := range vars {
		a, c := res[v.Name], resCode[v.Name]
		if a == nil {
			a = &agg{}
		}
		if c == nil {
			c = &agg{}
		}
		fmt.Printf("%-18s senza sigla %2d/%2d sim %.2f | con sigla %2d/%2d sim %.2f\n", v.Name, a.ok, a.n, a.sim/float64(max(1, a.n)), c.ok, c.n, c.sim/float64(max(1, c.n)))
	}
	_ = order
}

// labWordCrops: individua le parole della riga del valore (nel ritaglio gia' pronto) con la proiezione verticale
// dell'inchiostro scuro e restituisce un ritaglio per parola.
func labWordSplit(crop *image.Gray, darkThr uint8, expY float64) (words []*image.Gray, boxes []image.Rectangle) {
	b := crop.Bounds()
	w, h := b.Dx(), b.Dy()
	// righe: trova la banda di righe con piu' inchiostro scuro (il valore, piu' scuro dell'etichetta)
	rowSum := make([]int, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if crop.Pix[y*crop.Stride+x] < darkThr {
				rowSum[y]++
			}
		}
	}
	// bande di righe con inchiostro (> 1/6 del massimo); si sceglie quella piu' vicina alla posizione attesa del valore
	maxv := 0
	for _, v := range rowSum {
		if v > maxv {
			maxv = v
		}
	}
	type band struct{ a, b int }
	var bands []band
	for y := 0; y < h; y++ {
		if rowSum[y] > maxv/6 {
			if len(bands) > 0 && bands[len(bands)-1].b == y-1 {
				bands[len(bands)-1].b = y
			} else {
				bands = append(bands, band{y, y})
			}
		}
	}
	if len(bands) == 0 {
		return
	}
	// il valore e' in grassetto: tra le bande vince quella con piu' inchiostro scuro (con un lieve peso della vicinanza)
	bi, bestScore := 0, -1.0
	for i, bd := range bands {
		mass := 0
		for y := bd.a; y <= bd.b; y++ {
			mass += rowSum[y]
		}
		sc := float64(mass) / (1 + 0.15*absf(float64(bd.a+bd.b)/2-expY)/float64(h))
		if sc > bestScore {
			bi, bestScore = i, sc
		}
	}
	y0, y1 := bands[bi].a, bands[bi].b
	colSum := make([]int, w)
	for x := 0; x < w; x++ {
		for y := y0; y <= y1; y++ {
			if crop.Pix[y*crop.Stride+x] < darkThr {
				colSum[x]++
			}
		}
	}
	lineH := y1 - y0 + 1
	gapMin := int(0.45 * float64(lineH))
	start, last := -1, -1
	flush := func() {
		if start >= 0 && last-start >= lineH/4 {
			r := image.Rect(max(0, start-4), max(0, y0-4), min(w, last+5), min(h, y1+5))
			boxes = append(boxes, r)
		}
		start = -1
	}
	for x := 0; x < w; x++ {
		if colSum[x] > 0 {
			if start >= 0 && x-last > gapMin {
				flush()
			}
			if start < 0 {
				start = x
			}
			last = x
		}
	}
	flush()
	for _, r := range boxes {
		sub := image.NewGray(image.Rect(0, 0, r.Dx()+2*vizCropMargin, r.Dy()+2*vizCropMargin))
		for i := range sub.Pix {
			sub.Pix[i] = 255
		}
		for y := 0; y < r.Dy(); y++ {
			copy(sub.Pix[(y+vizCropMargin)*sub.Stride+vizCropMargin:], crop.Pix[(r.Min.Y+y)*crop.Stride+r.Min.X:(r.Min.Y+y)*crop.Stride+r.Max.X])
		}
		words = append(words, sub)
	}
	return
}

func TestLabWords(t *testing.T) {
	b := labBand("cargo")
	vars := []labVar{
		{Name: "u1950-t70", V: vizVariant{1950, 0.53, 0.53, false, 70}},
		{Name: "u1950-t0", V: vizVariant{1950, 0.53, 0.53, false, 0}},
	}
	for _, li := range labList(t, "capacity") {
		if !labHasRankCode(li.Truth.Fields["capacity"]) {
			continue
		}
		labPrep(t, li)
		if !li.ok {
			continue
		}
		crops, exp := labBandCrops(li, b, vars)
		for vi, c := range crops {
			words, boxes := labWordSplit(c, 110, exp[vi])
			if len(words) == 0 {
				fmt.Printf("%-30s %s: nessuna parola\n", li.Name, vars[vi].Name)
				continue
			}
			// ingrandimenti diversi di ogni parola
			var all []*image.Gray
			for _, wc := range words {
				all = append(all, wc)
			}
			ws := labOCR(t, all)
			var out []string
			for i := range words {
				var parts []string
				for _, w := range ws[i] {
					parts = append(parts, w.text)
				}
				out = append(out, fmt.Sprintf("%v:%q", boxes[i].Dx(), strings.Join(parts, "+")))
			}
			fmt.Printf("%-30s %s: %s\n", li.Name, vars[vi].Name, strings.Join(out, " | "))
		}
	}
}

func TestLabWordsDump(t *testing.T) {
	b := labBand("cargo")
	vars := []labVar{{Name: "u1950-t70", V: vizVariant{1950, 0.53, 0.53, false, 70}}}
	dir := filepath.Join("..", "..", "_out", "lab")
	for _, li := range labList(t, "capacity") {
		if li.Name != os.Getenv("LAB_IMG") {
			continue
		}
		labPrep(t, li)
		crops, exp := labBandCrops(li, b, vars)
		_ = imgproc.SavePNG(crops[0], filepath.Join(dir, "w_full.png"))
		words, _ := labWordSplit(crops[0], 110, exp[0])
		for i, w := range words {
			_ = imgproc.SavePNG(w, filepath.Join(dir, fmt.Sprintf("w_%d.png", i)))
		}
	}
}

// labLastWord: ritaglio dell'ultima parola della riga del valore (con margine bianco di pad pixel).
func labLastWord(crop *image.Gray, expY float64, pad int) *image.Gray {
	words, boxes := labWordSplit(crop, 110, expY)
	if len(words) == 0 {
		return nil
	}
	r := boxes[len(boxes)-1]
	sub := image.NewGray(image.Rect(0, 0, r.Dx()+2*pad, r.Dy()+2*pad))
	for i := range sub.Pix {
		sub.Pix[i] = 255
	}
	for y := 0; y < r.Dy(); y++ {
		copy(sub.Pix[(y+pad)*sub.Stride+pad:], crop.Pix[(r.Min.Y+y)*crop.Stride+r.Min.X:(r.Min.Y+y)*crop.Stride+r.Max.X])
	}
	return sub
}

func TestLabCodeToken(t *testing.T) {
	b := labBand("cargo")
	var imgs []*labImg
	for _, li := range labList(t, "capacity") {
		if labHasRankCode(li.Truth.Fields["capacity"]) && li.Name != "panama" {
			imgs = append(imgs, li)
		}
	}
	type vr struct {
		name string
		v    vizVariant
		pad  int
		sc   float64
	}
	var vs []vr
	for _, u := range []float64{1300, 1950, 2600} {
		for _, th := range []int{0, 40, 70, 100} {
			for _, sc := range []float64{0.5, 1, 1.5, 2} {
				for _, pad := range []int{16, 48} {
					vs = append(vs, vr{fmt.Sprintf("u%.0f t%d x%.1f pad%d", u, th, sc, pad), vizVariant{u, 0.53, 0.53, false, th}, pad, sc})
				}
			}
		}
	}
	counts := map[string]map[string]int{}
	for _, li := range imgs {
		labPrep(t, li)
		if !li.ok {
			continue
		}
		var crops []*image.Gray
		var names []string
		for _, v := range vs {
			cs, exp := labBandCrops(li, b, []labVar{{Name: v.name, V: v.v}})
			wc := labLastWord(cs[0], exp[0], v.pad)
			if wc == nil {
				continue
			}
			if v.sc != 1 {
				wc = vizToGray(imgproc.Scale(wc, v.sc))
			}
			crops = append(crops, wc)
			names = append(names, v.name)
		}
		ws := labOCR(t, crops)
		for i, n := range names {
			var parts []string
			for _, w := range ws[i] {
				parts = append(parts, w.text)
			}
			r := strings.Join(parts, "+")
			if counts[n] == nil {
				counts[n] = map[string]int{}
			}
			counts[n][r]++
		}
	}
	for _, v := range vs {
		m := counts[v.name]
		type kv struct {
			k string
			n int
		}
		var l []kv
		for k, n := range m {
			l = append(l, kv{k, n})
		}
		sort.Slice(l, func(i, j int) bool { return l[i].n > l[j].n })
		var parts []string
		for i, x := range l {
			if i >= 5 {
				break
			}
			parts = append(parts, fmt.Sprintf("%q×%d", x.k, x.n))
		}
		fmt.Printf("%-28s %s\n", v.name, strings.Join(parts, " "))
	}
}

func TestLabGeom(t *testing.T) {
	for _, li := range labList(t, "") {
		labPrep(t, li)
		b := li.g.Bounds()
		if !li.ok {
			fmt.Printf("%-42s %dx%d rot %d: MRZ NON localizzata\n", li.Name, b.Dx(), b.Dy(), li.Rot)
			continue
		}
		thr, ref := vizAdaptiveThreshold(li.g, li.geo)
		fmt.Printf("%-42s %dx%d rot %3d x0=%4.0f w=%4.0f (%.2f del lato) y0=%4.0f (%.2f) skew %+.4f thr %3d ref %.3f\n", li.Name, b.Dx(), b.Dy(), li.Rot, li.geo.x0, li.geo.w, li.geo.w/float64(b.Dx()), li.geo.y0, li.geo.y0/float64(b.Dy()), li.geo.skew, thr, ref)
	}
}

func TestLabWidth(t *testing.T) {
	for _, li := range labList(t, "") {
		if !strings.Contains(os.Getenv("LAB_IMGS"), li.Name) {
			continue
		}
		src, _ := imgproc.Load(li.Path)
		base := imgproc.CropContent(src, 10)
		base, ang := imgproc.Deskew(base)
		g := vizToGray(imgproc.Rotate(base, li.Rot))
		vizTraceWidth = func(m, p float64) {
			fmt.Printf("%s: deskew %.2f, larghezza misurata %.0f, dal passo %.0f\n", li.Name, ang, m, p)
		}
		r1, r2, ok := imgproc.LowestPair(vizBinRows(g), g.Bounds().Dx())
		fmt.Printf("%s: imgproc ok=%v %+v %+v\n", li.Name, ok, r1, r2)
		vizLocateMRZ(g)
	}
	vizTraceWidth = nil
}

// labAssemble: parole (ritagli) della riga del valore lette una per una e riunite con lo spazio.
func labWordLevel(t *testing.T, li *labImg, b vizBand, v vizVariant) (text string, nWords int, words []string) {
	cs, exp := labBandCrops(li, b, []labVar{{V: v}})
	wcs, _ := labWordSplit(cs[0], 110, exp[0])
	if len(wcs) == 0 {
		return "", 0, nil
	}
	ws := labOCR(t, wcs)
	for i := range wcs {
		var parts []string
		for _, w := range ws[i] {
			parts = append(parts, w.text)
		}
		words = append(words, strings.Join(parts, ""))
	}
	return strings.Join(words, " "), len(wcs), words
}

func TestLabWordLevel(t *testing.T) {
	b := labBand("cargo")
	vs := []vizVariant{
		{1950, 0.53, 0.53, false, 40}, {1950, 0.53, 0.53, false, 70}, {1950, 0.53, 0.53, false, 100},
		{1650, 0.53, 0.53, false, 40}, {1650, 0.53, 0.53, false, 70}, {2600, 0.53, 0.53, false, 70},
	}
	okN := make([]int, len(vs))
	n := 0
	for _, li := range labList(t, "capacity") {
		want := li.Truth.Fields["capacity"]
		if labHasRankCode(want) {
			continue
		}
		labPrep(t, li)
		if !li.ok {
			continue
		}
		n++
		var outs []string
		for i, v := range vs {
			text, _, _ := labWordLevel(t, li, b, v)
			if verify.Equal("capacity", text, want) {
				okN[i]++
			}
			outs = append(outs, text)
		}
		fmt.Printf("%-34s verita=%q  %q\n", li.Name, want, outs)
	}
	fmt.Println("esatti (senza sigla) su", n, ":", okN)
}

// TestLabBandReads: letture di una fascia per un elenco di immagini (LAB_IMGS, LAB_BAND), con la verita'.
func TestLabBandReads(t *testing.T) {
	key := os.Getenv("LAB_BAND")
	names := os.Getenv("LAB_IMGS")
	if key == "" || names == "" {
		t.Skip("LAB_BAND e LAB_IMGS")
	}
	field := map[string]string{"cargo": "capacity", "luogo_nascita": "place_of_birth", "nacionalidad": "nationality", "regionale": "regional",
		"refrendo": "certificate", "identificazione": "id_number", "id_marino": "seafarers_id", "numero_libretto": "seamans_book",
		"livello": "level", "data_emissione": "date_of_issue", "data_scadenza": "date_of_expiry", "data_nascita": "date_of_birth", "tipo": "type"}[key]
	b := labBand(key)
	for _, li := range labList(t, "") {
		if !strings.Contains(","+names+",", ","+li.Name+",") {
			continue
		}
		labPrep(t, li)
		if !li.ok {
			fmt.Printf("%s: MRZ non localizzata\n", li.Name)
			continue
		}
		vars := labBaseVars(li)
		crops, expY := labBandCrops(li, b, vars)
		ws := labOCR(t, crops)
		var cands []vizCand
		var line []string
		for i, lv := range vars {
			tmp := &Result{Fields: map[string]string{}}
			vizReadBand(b, ws[i], expY[i], tmp)
			cands = append(cands, vizCand{id: i, f: tmp.Fields})
			if v := tmp.Fields[key]; v != "" {
				line = append(line, lv.Name+"="+v)
			}
		}
		out := &Result{Fields: map[string]string{}}
		vizVote(b, cands, func(int) float64 { return 1 }, out)
		fmt.Printf("%-30s verita=%q voto=%q\n   %s\n", li.Name, li.Truth.Fields[field], out.Fields[key], strings.Join(line, " "))
	}
}

func TestLabSplitDump(t *testing.T) {
	b := labBand("cargo")
	vars := []labVar{{Name: "u1950-t70", V: vizVariant{1950, 0.53, 0.53, false, 70}}, {Name: "u1950-t100", V: vizVariant{1950, 0.53, 0.53, false, 100}}}
	dir := filepath.Join("..", "..", "_out", "lab")
	for _, li := range labList(t, "capacity") {
		if !strings.Contains(","+os.Getenv("LAB_IMGS")+",", ","+li.Name+",") {
			continue
		}
		labPrep(t, li)
		crops, exp := labBandCrops(li, b, vars)
		for vi, c := range crops {
			_ = imgproc.SavePNG(c, filepath.Join(dir, fmt.Sprintf("sp_%s_%d.png", li.Name, vi)))
			words, boxes := labWordSplit(c, 110, exp[vi])
			fmt.Printf("%s %s: expY %.0f, crop %dx%d, %d parole %v\n", li.Name, vars[vi].Name, exp[vi], c.Bounds().Dx(), c.Bounds().Dy(), len(words), boxes)
		}
	}
}
