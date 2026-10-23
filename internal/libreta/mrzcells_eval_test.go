//go:build glyphgen

package libreta

// Generazione della libreria di glifi e valutazione "leave-one-booklet-out" del lettore a template (build tag glyphgen).
//
//	TPL_LABELS=<labels.json> go test -tags glyphgen -run TestGlyphgenEval -v ./internal/libreta       (tabella di accuratezza)
//	TPL_LABELS=<labels.json> TPL_WRITE=1 go test -tags glyphgen -run TestGlyphgenBuild ./internal/libreta   (scrive glyphs.bin)
//
// labels.json associa a ogni immagine del corpus (percorso relativo alla radice del progetto, con '/') la rotazione che
// la mette dritta e il testo MRZ validato: {"scans/x.png": {"rot": 0, "line1": "...", "line2": "..."}}. Contiene dati
// personali: tenerlo FUORI dal repository. La riga 2 si valida con i check digit; la riga 1 no, quindi va controllata a
// occhio (le righe lette dall'OCR di Windows hanno spesso buchi sui '<' e sui nomi lunghi). Per ricavarlo dal motore
// OCR si puo' lanciare TestGlyphgenDump (mrzcells_glyphgen_test.go) e correggere le righe 1 sbagliate.
//
// Libretto = stessa riga 2 (numero e data di nascita): ogni scansione dello stesso libretto sta nello stesso gruppo, e
// nella valutazione la libreria non contiene nessuna cella del gruppo letto.

import (
	"encoding/json"
	"fmt"
	"image"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"scanlibretto/internal/imgproc"
)

type tplLabel struct {
	Rot   int    `json:"rot"`
	Line1 string `json:"line1"`
	Line2 string `json:"line2"`
}

// tplEvalImage e' un'immagine del corpus con la sua etichetta, orientata e con le celle gia' estratte.
type tplEvalImage struct {
	file  string
	group uint8
	truth [2]string
	img   image.Image // immagine dritta
	band  *tplBand
}

func tplLoadLabels(t testing.TB) map[string]tplLabel {
	t.Helper()
	path := os.Getenv("TPL_LABELS")
	if path == "" {
		t.Skip("TPL_LABELS non impostata")
	}
	js, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]tplLabel
	if err := json.Unmarshal(js, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// tplOrient porta l'immagine nell'orientamento in cui la legge Extract (ritaglio, raddrizzamento, rotazione).
func tplOrient(path string, rot int) (image.Image, error) {
	src, err := imgproc.Load(path)
	if err != nil {
		return nil, err
	}
	base, _ := imgproc.Deskew(imgproc.CropContent(src, 10))
	return imgproc.Rotate(base, rot), nil
}

// tplBuildPool estrae le celle di tutte le immagini etichettate e ne fa la riserva di glifi (non potata).
func tplBuildPool(t testing.TB, labels map[string]tplLabel) ([]tplEvalImage, []tplGlyph) {
	t.Helper()
	var files []string
	for f := range labels {
		files = append(files, f)
	}
	sort.Strings(files)
	keys := map[string]uint8{}
	var ks []string
	for _, f := range files {
		l := labels[f]
		if len(l.Line1) == tplCells && len(l.Line2) == tplCells {
			ks = append(ks, l.Line2[0:10]+l.Line2[13:20])
		}
	}
	sort.Strings(ks)
	for _, k := range ks {
		if _, ok := keys[k]; !ok {
			keys[k] = uint8(len(keys))
		}
	}
	var imgs []tplEvalImage
	var pool []tplGlyph
	for _, f := range files {
		l := labels[f]
		if len(l.Line1) != tplCells || len(l.Line2) != tplCells {
			t.Logf("%s: etichetta non valida", f)
			continue
		}
		img, err := tplOrient(filepath.Join("..", "..", filepath.FromSlash(f)), l.Rot)
		if err != nil {
			t.Logf("%s: %v", f, err)
			continue
		}
		im2, bin, pairs := tplFindRows(img)
		if len(pairs) == 0 {
			t.Logf("%s: nessuna coppia di righe", f)
			continue
		}
		band, ok := tplMakeBand(im2, bin, pairs[0][0], pairs[0][1])
		if !ok {
			t.Logf("%s: griglia non adattata", f)
			continue
		}
		t.Logf("spessore %.2f %s", band.stroke, f)
		e := tplEvalImage{file: f, group: keys[l.Line2[0:10]+l.Line2[13:20]], truth: [2]string{l.Line1, l.Line2}, img: img, band: band}
		imgs = append(imgs, e)
		for r := 0; r < 2; r++ {
			for k := 0; k < tplCells; k++ {
				pool = append(pool, tplGlyph{sym: uint8(tplSym(e.truth[r][k])), group: e.group, v: tplReduce(band.frames[r][k])})
			}
		}
	}
	if os.Getenv("TPL_NOSYNTH") == "" {
		pool = append(pool, tplSynthPool(imgs)...)
	}
	return imgs, pool
}

// tplSynthW compone una 'W' sintetica da una 'V' reale: due mezze 'V' compresse in orizzontale e affiancate, con il
// vertice centrale in alto (la W dell'OCR-B e' fatta cosi'). Il corpus non ha nessuna 'W': senza questo glifo la W di
// un cognome verrebbe letta come la lettera piu' vicina (V, N, M) e segnalata come incerta. frame e' il riquadro
// canonico (tplFrame x tplFrame) di una 'V'; restituisce un riquadro dello stesso tipo (nil se la V e' vuota).
func tplSynthW(frame []uint8) []uint8 {
	x0, x1 := tplFrame, -1
	for x := 0; x < tplFrame; x++ {
		for y := tplCropV0; y < tplCropV1; y++ {
			if frame[y*tplFrame+x] > 77 { // copertura > 0,3
				x0, x1 = min(x0, x), max(x1, x)
			}
		}
	}
	if x1 <= x0 {
		return nil
	}
	const wWidth = 17.0 // larghezza dell'inchiostro della W in px del riquadro (0,85 passi)
	half := wWidth / 2
	out := make([]uint8, len(frame))
	for side := 0; side < 2; side++ {
		left := float64(tplFrame)/2 - half + float64(side)*half // sinistra della mezza V
		for x := int(math.Floor(left)); x < int(math.Ceil(left+half)); x++ {
			tt := (float64(x) + 0.5 - left) / half // 0..1 dentro la mezza V
			if x < 0 || x >= tplFrame || tt < 0 || tt > 1 {
				continue
			}
			sx := float64(x0) + tt*float64(x1-x0+1) - 0.5 // colonna sorgente
			i := int(math.Floor(sx))
			f := sx - float64(i)
			for y := 0; y < tplFrame; y++ {
				var a, b float64
				if i >= 0 && i < tplFrame {
					a = float64(frame[y*tplFrame+i])
				}
				if i+1 >= 0 && i+1 < tplFrame {
					b = float64(frame[y*tplFrame+i+1])
				}
				if v := uint8(a*(1-f) + b*f + 0.5); v > out[y*tplFrame+x] {
					out[y*tplFrame+x] = v
				}
			}
		}
	}
	return out
}

// tplSynthPool ricava le W sintetiche dalle V reali del corpus (una per ogni V) e le riporta allo spessore del tratto
// delle scansioni (la compressione lo assottiglia).
func tplSynthPool(imgs []tplEvalImage) []tplGlyph {
	var frames [][]uint8
	for _, e := range imgs {
		for r := 0; r < 2; r++ {
			for k := 0; k < tplCells; k++ {
				if e.truth[r][k] == 'V' {
					if w := tplSynthW(e.band.frames[r][k]); w != nil {
						frames = append(frames, w)
					}
				}
			}
		}
	}
	rad := tplThicknessRadius(tplStrokeWidth(frames))
	var out []tplGlyph
	for _, f := range frames {
		if rad != 0 {
			f = tplThicken(f, rad)
		}
		out = append(out, tplGlyph{sym: uint8(tplSym('W')), group: tplSynthetic, v: tplReduce(f)})
	}
	return out
}

// tplPerSymbol: glifi per simbolo nella libreria potata.
func tplPerSymbol() int {
	var n int
	if _, err := fmt.Sscan(os.Getenv("TPL_CAP"), &n); err == nil && n > 0 {
		return n
	}
	return 24
}

func TestGlyphgenBuild(t *testing.T) {
	labels := tplLoadLabels(t)
	imgs, pool := tplBuildPool(t, labels)
	lib := tplPrune(pool, tplPerSymbol())
	var count [tplNSym]int
	for _, g := range lib {
		count[g.sym]++
	}
	var sb strings.Builder
	for s := 0; s < tplNSym; s++ {
		fmt.Fprintf(&sb, "%c:%d ", tplAlphabet[s], count[s])
	}
	t.Logf("%d immagini, %d celle nella riserva, %d glifi nella libreria potata (per simbolo: %s)", len(imgs), len(pool), len(lib), sb.String())
	if os.Getenv("TPL_WRITE") == "" {
		t.Skip("TPL_WRITE non impostata: libreria non scritta")
	}
	f, err := os.Create("glyphs.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := tplWriteLibrary(f, lib); err != nil {
		t.Fatal(err)
	}
}

// tplScoreCells conta i caratteri uguali alla verita'.
func tplScoreCells(got, want string) int {
	n := 0
	for i := 0; i < len(got) && i < len(want); i++ {
		if got[i] == want[i] {
			n++
		}
	}
	return n
}

func TestGlyphgenEval(t *testing.T) {
	labels := tplLoadLabels(t)
	imgs, pool := tplBuildPool(t, labels)
	cap := tplPerSymbol()
	libs := map[uint8]*tplLibrary{} // libreria senza il gruppo, potata come in produzione
	libFor := func(g uint8) *tplLibrary {
		if l, ok := libs[g]; ok {
			return l
		}
		var gl []tplGlyph
		for _, x := range pool {
			if x.group != g {
				gl = append(gl, x)
			}
		}
		l := newTplLibrary(tplPrune(gl, cap))
		libs[g] = l
		return l
	}
	t.Logf("%-46s %3s %6s %6s %6s %7s %5s %6s", "immagine", "gr", "riga1", "riga2", "(raw1)", "check5", "score", "ms")
	var tot, ok5, plausible int
	var sumMs float64
	var c1, c2, raw1, raw2, flaggedWrong, flaggedRight, silentWrong int
	perGroup := map[uint8][2]int{}
	for _, e := range imgs {
		lib := libFor(e.group)
		start := time.Now()
		rd, _ := tplReadImage(e.img, lib, "")
		ms := float64(time.Since(start).Microseconds()) / 1000
		sumMs += ms
		tot++
		if rd == nil || rd.line[0] == "" {
			t.Logf("%-46s %3d  nessuna lettura (%.0f ms)", e.file, e.group, ms)
			continue
		}
		a, b := tplScoreCells(rd.line[0], e.truth[0]), tplScoreCells(rd.line[1], e.truth[1])
		ra, rb := tplScoreCells(rd.raw[0], e.truth[0]), tplScoreCells(rd.raw[1], e.truth[1])
		for r := 0; r < 2; r++ {
			for k := 0; k < tplCells; k++ {
				unsure := tplUnsure(rd, r, k)
				wrong := rd.line[r][k] != e.truth[r][k]
				switch {
				case unsure && wrong:
					flaggedWrong++
				case unsure:
					flaggedRight++
				case wrong:
					silentWrong++
				}
			}
		}
		c1 += a
		c2 += b
		raw1 += ra
		raw2 += rb
		all := rd.d.Checks.AllOK()
		if all && rd.score > 0 && a == tplCells && b == tplCells {
			ok5++
		}
		if rd.score > 0 {
			plausible++
		}
		pg := perGroup[e.group]
		pg[0] += a + b
		pg[1] += 2 * tplCells
		perGroup[e.group] = pg
		t.Logf("%-46s %3d %3d/44 %3d/44 %3d/44 %7v %5d %6.0f  post=%.2f/%.2f med=%.2f costo=%.1f/%.1f", e.file, e.group, a, b, ra, all, rd.score, ms, rd.post[0], rd.post[1], rd.med, rd.cost[0], rd.cost[1])
		if os.Getenv("TPL_DIFF") != "" { // posizioni (non i caratteri) delle celle sbagliate
			for r := 0; r < 2; r++ {
				for k := 0; k < tplCells; k++ {
					if rd.line[r][k] != e.truth[r][k] {
						kind := func(c byte) string {
							switch {
							case c == '<':
								return "fill"
							case c >= '0' && c <= '9':
								return "digit"
							}
							return "letter"
						}
						t.Logf("    riga%d cella %d: letto %s, vero %s (piu' vicino: %s) p=%.2f dmin=%.2f", r+1, k, kind(rd.line[r][k]), kind(e.truth[r][k]), kind(rd.raw[r][k]), math.Exp(rd.lp[r][k][tplSym(rd.line[r][k])]), rd.dmin[r][k])
					}
				}
			}
		}
	}
	n := float64(tot * tplCells)
	t.Logf("TOTALE %d immagini: riga1 %.2f%% riga2 %.2f%% (senza vincoli: %.2f%% / %.2f%%); MRZ completa e giusta %d/%d; plausibili %d; %.0f ms in media",
		tot, 100*float64(c1)/n, 100*float64(c2)/n, 100*float64(raw1)/n, 100*float64(raw2)/n, ok5, tot, plausible, sumMs/float64(tot))
	t.Logf("celle incerte (tplUnsure): %d sbagliate segnalate, %d giuste segnalate, %d sbagliate NON segnalate", flaggedWrong, flaggedRight, silentWrong)
	for g, v := range perGroup {
		t.Logf("  libretto %d: %.2f%% dei caratteri giusti", g, 100*float64(v[0])/float64(v[1]))
	}
}

// TestGlyphgenNegative: le immagini del corpus ruotate di 180/90/270 gradi (e le pagine senza MRZ) non devono dare una
// lettura plausibile (score > 0): il lettore viene chiamato su ogni rotazione.
func TestGlyphgenNegative(t *testing.T) {
	labels := tplLoadLabels(t)
	imgs, pool := tplBuildPool(t, labels)
	lib := newTplLibrary(tplPrune(pool, tplPerSymbol())) // libreria completa: caso peggiore (riconosce tutti i glifi)
	var n, falsePos int
	var worst float64
	for _, e := range imgs {
		for _, rot := range []int{180, 90, 270} {
			start := time.Now()
			rd, band := tplReadImage(imgproc.Rotate(e.img, rot), lib, "")
			ms := float64(time.Since(start).Microseconds()) / 1000
			n++
			desc := "nessuna griglia"
			if rd != nil && band != nil {
				desc = fmt.Sprintf("score=%d q=%.3f med=%.2f post=%.3f/%.3f", rd.score, band.quality, rd.med, rd.post[0], rd.post[1])
				if rd.score > 0 {
					falsePos++
					t.Errorf("%s ruotata di %d: lettura plausibile (%s)", e.file, rot, desc)
				}
			}
			worst = math.Max(worst, ms)
			if rd != nil && band != nil && (os.Getenv("TPL_DIFF") != "") {
				t.Logf("%-46s rot %3d: %s (%.0f ms)", e.file, rot, desc, ms)
			}
		}
	}
	t.Logf("%d letture su rotazioni sbagliate, %d falsi positivi, tempo massimo %.0f ms", n, falsePos, worst)
	// pagine del corpus senza etichetta (non MRZ o MRZ illeggibile): solo informativo
	for _, f := range tplCorpus(t) {
		rel := filepath.ToSlash(strings.TrimPrefix(f, filepath.Join("..", "..")+string(filepath.Separator)))
		if _, ok := labels[rel]; ok {
			continue
		}
		src, err := imgproc.Load(f)
		if err != nil {
			continue
		}
		base, _ := imgproc.Deskew(imgproc.CropContent(src, 10))
		for _, rot := range []int{0, 180, 90, 270} {
			rd, band := tplReadImage(imgproc.Rotate(base, rot), lib, "")
			if rd != nil && band != nil {
				t.Logf("senza etichetta %-70s rot %3d: score=%d q=%.3f med=%.2f post=%.3f/%.3f", rel, rot, rd.score, band.quality, rd.med, rd.post[0], rd.post[1])
			}
		}
	}
}

// ---------------------------------------------------------------------------------------------------------------
// Prove di robustezza: la stessa valutazione leave-one-booklet-out su immagini degradate artificialmente.

func tplToGray(img image.Image) *image.Gray {
	b := img.Bounds()
	g := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, gg, bb, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			g.Pix[y*g.Stride+x] = uint8((299*(r>>8) + 587*(gg>>8) + 114*(bb>>8)) / 1000)
		}
	}
	return g
}

type tplStress struct {
	name string
	fn   func(g *image.Gray) image.Image
}

func tplBlur(g *image.Gray, sigma float64) *image.Gray {
	rad := int(3*sigma) + 1
	k := make([]float64, 2*rad+1)
	var sum float64
	for i := range k {
		d := float64(i - rad)
		k[i] = math.Exp(-d * d / (2 * sigma * sigma))
		sum += k[i]
	}
	w, h := g.Bounds().Dx(), g.Bounds().Dy()
	tmp := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var s float64
			for j, kv := range k {
				s += kv * float64(g.Pix[y*g.Stride+min(max(x+j-rad, 0), w-1)])
			}
			tmp[y*w+x] = s / sum
		}
	}
	out := image.NewGray(g.Bounds())
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var s float64
			for j, kv := range k {
				s += kv * tmp[min(max(y+j-rad, 0), h-1)*w+x]
			}
			out.Pix[y*out.Stride+x] = uint8(s/sum + 0.5)
		}
	}
	return out
}

// tplMorph applica un filtro minimo (ink=true: inchiostro piu' spesso) o massimo 3x3.
func tplMorph(g *image.Gray, thicker bool) *image.Gray {
	w, h := g.Bounds().Dx(), g.Bounds().Dy()
	out := image.NewGray(g.Bounds())
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := g.Pix[y*g.Stride+x]
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					u := g.Pix[min(max(y+dy, 0), h-1)*g.Stride+min(max(x+dx, 0), w-1)]
					if (thicker && u < v) || (!thicker && u > v) {
						v = u
					}
				}
			}
			out.Pix[y*out.Stride+x] = v
		}
	}
	return out
}

// tplNoise aggiunge rumore gaussiano di deviazione standard sigma (livelli di grigio, 0..255).
func tplNoise(g *image.Gray, sigma float64) *image.Gray {
	rng := rand.New(rand.NewSource(1))
	out := image.NewGray(g.Bounds())
	for i, v := range g.Pix {
		out.Pix[i] = uint8(min(255, max(0, int(v)+int(rng.NormFloat64()*sigma))))
	}
	return out
}

var tplStresses = []tplStress{
	{"nessuna", func(g *image.Gray) image.Image { return g }},
	{"risoluzione 70%", func(g *image.Gray) image.Image { return imgproc.Scale(g, 0.7) }},
	{"risoluzione 50%", func(g *image.Gray) image.Image { return imgproc.Scale(g, 0.5) }},
	{"sfocatura 1 px", func(g *image.Gray) image.Image { return tplBlur(g, 1.0) }},
	{"sfocatura 1,6 px", func(g *image.Gray) image.Image { return tplBlur(g, 1.6) }},
	{"rumore 8 livelli", func(g *image.Gray) image.Image { return tplNoise(g, 8) }},
	{"rumore 20 livelli", func(g *image.Gray) image.Image { return tplNoise(g, 20) }},
	{"contrasto 50%", func(g *image.Gray) image.Image {
		out := image.NewGray(g.Bounds())
		for i, v := range g.Pix {
			out.Pix[i] = uint8(255 - (255-int(v))/2)
		}
		return out
	}},
	{"inclinata 1,2 gradi", func(g *image.Gray) image.Image { return imgproc.RotateAngle(g, 1.2) }},
	{"tratto piu' spesso", func(g *image.Gray) image.Image { return tplMorph(g, true) }},
	{"tratto piu' sottile", func(g *image.Gray) image.Image { return tplMorph(g, false) }},
}

func TestGlyphgenStress(t *testing.T) {
	labels := tplLoadLabels(t)
	imgs, pool := tplBuildPool(t, labels)
	cap := tplPerSymbol()
	libs := map[uint8]*tplLibrary{}
	libFor := func(g uint8) *tplLibrary {
		if l, ok := libs[g]; ok {
			return l
		}
		var gl []tplGlyph
		for _, x := range pool {
			if x.group != g {
				gl = append(gl, x)
			}
		}
		l := newTplLibrary(tplPrune(gl, cap))
		libs[g] = l
		return l
	}
	// una scansione per libretto e risoluzione diversa (la piu' piccola e la piu' grande di ogni gruppo)
	seen := map[uint8]int{}
	var sel []tplEvalImage
	for _, e := range imgs {
		if seen[e.group] < 2 {
			seen[e.group]++
			sel = append(sel, e)
		}
	}
	for _, st := range tplStresses {
		// due categorie: scansioni a 300 dpi (passo >= 25 px) e immagini a bassa risoluzione (facsimili, passo ~15 px)
		var c1, c2, full, plaus, n [2]int
		for _, e := range sel {
			cat := 0
			if e.band.pitch < 25 {
				cat = 1
			}
			g := tplToGray(e.img)
			si := st.fn(g)
			rd, band := tplReadImage(si, libFor(e.group), "")
			n[cat]++
			if rd == nil || rd.line[0] == "" {
				if os.Getenv("TPL_DIFF") != "" {
					_, _, pairs := tplFindRows(si)
					t.Logf("   [%s] %s: nessuna lettura (coppie di righe trovate: %d)", st.name, e.file, len(pairs))
				}
				continue
			}
			if os.Getenv("TPL_DIFF") != "" && rd.score == 0 && band != nil {
				t.Logf("   [%s] %s: non plausibile q=%.3f med=%.2f post=%.3f/%.3f", st.name, e.file, band.quality, rd.med, rd.post[0], rd.post[1])
			}
			a, b := tplScoreCells(rd.line[0], e.truth[0]), tplScoreCells(rd.line[1], e.truth[1])
			c1[cat] += a
			c2[cat] += b
			if a == tplCells && b == tplCells {
				full[cat]++
			}
			if rd.score > 0 {
				plaus[cat]++
			}
		}
		line := func(c int) string {
			if n[c] == 0 {
				return "-"
			}
			return fmt.Sprintf("riga1 %.1f%% riga2 %.1f%% MRZ intera %d/%d plausibili %d/%d", 100*float64(c1[c])/float64(n[c]*tplCells), 100*float64(c2[c])/float64(n[c]*tplCells), full[c], n[c], plaus[c], n[c])
		}
		t.Logf("%-22s scansioni 300 dpi: %s | facsimili: %s", st.name, line(0), line(1))
	}
}

// TestGlyphgenTiming: tempo delle fasi del lettore su alcune immagini (libreria incorporata).
func TestGlyphgenTiming(t *testing.T) {
	labels := tplLoadLabels(t)
	lib := tplEmbedded()
	for _, f := range []string{"scans/provamrz.png", "scans/DEFINITIVO_spero.png", "scans/vecchi test WIA/libretto-vero1-grezza.png", "scans/vecchi test WIA/libretto-2.png", "facsimili/panama/panama.png"} {
		l, ok := labels[f]
		if !ok {
			continue
		}
		img, err := tplOrient(filepath.Join("..", "..", filepath.FromSlash(f)), l.Rot)
		if err != nil {
			t.Fatal(err)
		}
		const n = 5
		var tFind, tBand, tRead, tAll time.Duration
		for i := 0; i < n; i++ {
			t0 := time.Now()
			im2, bin, pairs := tplFindRows(img)
			t1 := time.Now()
			band, _ := tplMakeBand(im2, bin, pairs[0][0], pairs[0][1])
			t2 := time.Now()
			tplRead(band, lib)
			t3 := time.Now()
			readMRZCells(img, "")
			t4 := time.Now()
			tFind += t1.Sub(t0)
			tBand += t2.Sub(t1)
			tRead += t3.Sub(t2)
			tAll += t4.Sub(t3)
		}
		ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 / n }
		t.Logf("%-52s %dx%d: righe %.0f ms, griglia e celle %.0f ms, classificazione e decodifica %.0f ms; readMRZCells intera %.0f ms",
			f, img.Bounds().Dx(), img.Bounds().Dy(), ms(tFind), ms(tBand), ms(tRead), ms(tAll))
	}
}
