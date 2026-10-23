//go:build vizlab

package libreta

// Laboratorio, parte 2: l'intero percorso VIZ (prepareVIZImage + OCR + extractVIZ + crossCheck + finalizeVIZ) su tutto il
// corpus, con cache su disco dell'OCR e MRZ presa dai JSON di una prova gia' fatta (LAB_MRZJSON, cartella di NOME.json
// prodotti da `libretto extract`): cosi' le modifiche al voto e a finalizeVIZ si misurano in secondi, e solo i ritagli
// nuovi vanno all'OCR. La cache contiene il testo OCR delle immagini del corpus (dati personali): sta in LAB_CACHE
// (cartella temporanea fuori dal progetto), mai nel repository.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/mrz"
	"scanlibretto/internal/ocr"
	"scanlibretto/internal/verify"
)

func labCacheDir() string {
	d := os.Getenv("LAB_CACHE")
	if d == "" {
		d = filepath.Join(os.TempDir(), "vizlab-ocrcache")
	}
	_ = os.MkdirAll(d, 0o755)
	return d
}

var labStats struct{ hit, miss int }

// labRecognizeMany legge i file con la cache: le immagini gia' lette (stesso contenuto) non vanno all'OCR.
func labRecognizeMany(ctx context.Context, paths []string) ([]*ocr.Result, error) {
	dir := labCacheDir()
	out := make([]*ocr.Result, len(paths))
	keys := make([]string, len(paths))
	var missPaths []string
	var missIdx []int
	for i, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		keys[i] = hex.EncodeToString(sum[:16])
		if raw, err := os.ReadFile(filepath.Join(dir, keys[i]+".json")); err == nil {
			var r ocr.Result
			if json.Unmarshal(raw, &r) == nil {
				out[i] = &r
				labStats.hit++
				continue
			}
		}
		missPaths = append(missPaths, p)
		missIdx = append(missIdx, i)
		labStats.miss++
	}
	const batch = 200
	for a := 0; a < len(missPaths); a += batch {
		b := min(len(missPaths), a+batch)
		rs, err := ocr.RecognizeMany(ctx, missPaths[a:b])
		if err != nil {
			return nil, err
		}
		for j, r := range rs {
			i := missIdx[a+j]
			out[i] = r
			raw, _ := json.Marshal(r)
			_ = os.WriteFile(filepath.Join(dir, keys[i]+".json"), raw, 0o644)
		}
	}
	return out, nil
}

func labRecognize(ctx context.Context, path string) (*ocr.Result, error) {
	rs, err := labRecognizeMany(ctx, []string{path})
	if err != nil {
		return nil, err
	}
	return rs[0], nil
}

// labMRZFromJSON legge rotazione e MRZ dal JSON di una prova gia' fatta.
func labMRZFromJSON(dir, name string) (rot int, d *mrz.Data, notes []string, ok bool) {
	raw, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return 0, nil, nil, false
	}
	var o struct {
		Rotation int       `json:"rotation_deg"`
		MRZ      *mrz.Data `json:"mrz"`
		Notes    []string  `json:"mrz_corrections"`
	}
	if json.Unmarshal(raw, &o) != nil {
		return 0, nil, nil, false
	}
	return o.Rotation, o.MRZ, o.Notes, true
}

// labRunVIZ esegue il percorso VIZ di Extract su un'immagine (MRZ e rotazione dai JSON).
func labRunVIZ(t *testing.T, li *labImg, mrzDir string) *Result {
	t.Helper()
	rot, d, notes, ok := labMRZFromJSON(mrzDir, li.Name)
	if !ok {
		t.Fatalf("JSON MRZ assente per %s", li.Name)
	}
	src, err := imgproc.Load(li.Path)
	if err != nil {
		t.Fatal(err)
	}
	base := imgproc.CropContent(src, 10)
	base, _ = imgproc.Deskew(base)
	var img image.Image = base
	res := &Result{Fields: map[string]string{}}
	if d != nil || rot != 0 {
		img = imgproc.Rotate(base, rot)
		res.Rotation, res.MRZ, res.MRZCorrections = rot, d, notes
	}
	scaled := prepareVIZImage(img)
	tmp, err := os.CreateTemp("", "labpage-*.png")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if err := imgproc.SavePNG(scaled, tmp.Name()); err != nil {
		t.Fatal(err)
	}
	page, err := labRecognize(context.Background(), tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	if res.MRZ == nil {
		findMRZ(page, res)
	}
	extractVIZ(page, res)
	crossCheck(res)
	finalizeVIZ(res)
	return res
}

var labKeyMap = map[string]string{
	"tipo": "type", "paese": "country_code", "nacionalidad": "nationality", "luogo_nascita": "place_of_birth",
	"data_emissione": "date_of_issue", "data_scadenza": "date_of_expiry", "data_nascita": "date_of_birth", "livello": "level",
	"cargo": "capacity", "numero_libretto": "seamans_book", "identificazione": "id_number", "refrendo": "certificate",
	"id_marino": "seafarers_id", "regionale": "regional",
}

var labFieldOrder = []string{"type", "country_code", "nationality", "place_of_birth", "date_of_issue", "date_of_expiry", "date_of_birth",
	"level", "capacity", "seamans_book", "id_number", "certificate", "seafarers_id", "regional"}

// labOutFields traduce i campi interni nei nomi inglesi del JSON.
func labOutFields(r *Result) map[string]string {
	out := map[string]string{}
	for k, v := range r.Fields {
		if n, ok := labKeyMap[k]; ok {
			out[n] = v
		}
	}
	return out
}

// TestLabFull: tabella per campo giusti/con verita' sull'intero corpus (LAB_MRZJSON obbligatoria, LAB_ONLY facoltativa).
func TestLabFull(t *testing.T) {
	mrzDir := os.Getenv("LAB_MRZJSON")
	if mrzDir == "" {
		t.Skip("LAB_MRZJSON")
	}
	vizOCRMany = labRecognizeMany
	defer func() { vizOCRMany = ocr.RecognizeMany }()
	only := os.Getenv("LAB_ONLY")
	vizNoCalib = os.Getenv("LAB_CALIB") == ""
	if v := os.Getenv("LAB_WW"); v != "" {
		vizWordWeight, _ = strconv.ParseFloat(v, 64)
	}
	type cnt struct{ g, s, m, x int }
	tot := map[string]*cnt{}
	for _, k := range labFieldOrder {
		tot[k] = &cnt{}
	}
	start := time.Now()
	var bad []string
	flagGood, flagBad := 0, 0
	var silentBad []string
	nimg := 0
	var jsons map[string]map[string]string
	if os.Getenv("LAB_DUMP") != "" {
		jsons = map[string]map[string]string{}
	}
	for _, li := range labList(t, "") {
		if only != "" && !strings.Contains(","+only+",", ","+li.Name+",") {
			continue
		}
		if tk := os.Getenv("LAB_TRACE"); tk != "" {
			vizTrace = func(key, variant string, f map[string]string) {
				if key == tk {
					fmt.Println("     ", li.Name, key, variant, f[key])
				}
			}
		}
		res := labRunVIZ(t, li, mrzDir)
		vizTrace = nil
		nimg++
		if os.Getenv("LAB_SHOW") != "" {
			var ks []string
			for k := range res.Fields {
				ks = append(ks, k)
			}
			sort.Strings(ks)
			for _, k := range ks {
				fmt.Println("  ", k, "=", res.Fields[k])
			}
			fmt.Println("   avvisi:", res.Warnings)
		}
		got := labOutFields(res)
		if jsons != nil {
			jsons[li.Name] = got
		}
		var wrong []string
		flagged := map[string]bool{}
		for _, w := range res.Warnings {
			if rest, ok := strings.CutPrefix(w, "campo incerto: "); ok {
				k, _, _ := strings.Cut(rest, " ")
				flagged[labKeyMap[k]] = true
			}
		}
		for _, k := range labFieldOrder {
			want := li.Truth.Fields[k]
			g := got[k]
			switch {
			case want == "" && g == "":
			case want == "":
				tot[k].x++
			case g == "":
				tot[k].m++
				wrong = append(wrong, k+"(M)")
			case verify.Equal(k, g, want):
				tot[k].g++
				if flagged[k] {
					flagGood++
				}
			default:
				tot[k].s++
				wrong = append(wrong, k+"(S)")
				if flagged[k] {
					flagBad++
				} else {
					silentBad = append(silentBad, li.Name+":"+k)
				}
			}
		}
		if len(wrong) > 0 {
			bad = append(bad, fmt.Sprintf("%-40s %s", li.Name, strings.Join(wrong, " ")))
		}
		_ = res
	}
	var g, n int
	for _, k := range labFieldOrder {
		c := tot[k]
		fmt.Printf("%-16s %3d/%3d  S=%2d M=%2d X=%2d\n", k, c.g, c.g+c.s+c.m, c.s, c.m, c.x)
		g += c.g
		n += c.g + c.s + c.m
	}
	fmt.Printf("totale %d/%d (%.1f%%), %d immagini, %.0f s, cache: %d letti, %d nuovi\n", g, n, 100*float64(g)/float64(max(1, n)), nimg, time.Since(start).Seconds(), labStats.hit, labStats.miss)
	fmt.Printf("avvisi: campi giusti con avviso %d, campi sbagliati con avviso %d, sbagliati SENZA avviso %d\n", flagGood, flagBad, len(silentBad))
	if os.Getenv("LAB_V") != "" {
		fmt.Println("senza avviso:", strings.Join(silentBad, " "))
	}
	sort.Strings(bad)
	if os.Getenv("LAB_V") != "" {
		for _, b := range bad {
			fmt.Println(b)
		}
	}
	if jsons != nil {
		raw, _ := json.MarshalIndent(jsons, "", " ")
		_ = os.WriteFile(os.Getenv("LAB_DUMP"), raw, 0o644)
	}
}

// TestLabOffsets: scostamento verticale (in millesimi di W) della riga del valore rispetto alla posizione attesa, per fascia, trovato
// con la verita': tra le righe OCR di un ritaglio a finestra larga si prende quella il cui testo somiglia (>= 0,75) al valore noto.
func TestLabOffsets(t *testing.T) {
	keys := []string{"nacionalidad", "luogo_nascita", "data_emissione", "data_scadenza", "livello", "cargo", "id_marino", "numero_libretto", "identificazione", "data_nascita", "regionale"}
	vwide := vizVariant{1300, 2.0, 2.0, false, 0}
	vizOCRMany = labRecognizeMany
	defer func() { vizOCRMany = ocr.RecognizeMany }()
	type item struct {
		li   *labImg
		key  string
		exp  float64
		want string
	}
	var items []item
	var crops []*image.Gray
	for _, li := range labList(t, "") {
		labPrep(t, li)
		if !li.ok {
			continue
		}
		for _, key := range keys {
			want := li.Truth.Fields[labKeyMap[key]]
			if want == "" {
				continue
			}
			cs, exp := labBandCrops(li, labBand(key), []labVar{{V: vwide}})
			items = append(items, item{li, key, exp[0], want})
			crops = append(crops, cs[0])
		}
	}
	dir, _ := os.MkdirTemp("", "laboff")
	defer os.RemoveAll(dir)
	var paths []string
	for i, c := range crops {
		p := filepath.Join(dir, fmt.Sprintf("c%05d.png", i))
		_ = imgproc.SavePNG(c, p)
		paths = append(paths, p)
	}
	rs, err := labRecognizeMany(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	last := ""
	line := ""
	var all = map[string][]float64{}
	for i, it := range items {
		if it.li.Name != last {
			if line != "" {
				fmt.Println(line)
			}
			last = it.li.Name
			line = fmt.Sprintf("%-32s", it.li.Name)
		}
		ws := vizWords(rs[i])
		best, bs := 0.0, 0.0
		for _, l := range vizGroupLines(ws) {
			for _, cand := range []string{l.text, vizFirstSegment(l)} {
				if sc := labSim(cand, it.want); sc > bs {
					bs, best = sc, l.cy()
				}
			}
		}
		if bs < 0.75 {
			line += fmt.Sprintf(" %s:--", it.key[:3])
			continue
		}
		off := (best - it.exp) / vwide.unit * 1000
		all[it.key] = append(all[it.key], off)
		line += fmt.Sprintf(" %s:%+.0f", it.key[:3], off)
	}
	fmt.Println(line)
	for _, k := range keys {
		v := all[k]
		sort.Float64s(v)
		if len(v) > 0 {
			fmt.Printf("%-16s n=%2d mediana %+.0f  min %+.0f max %+.0f\n", k, len(v), v[len(v)/2], v[0], v[len(v)-1])
		}
	}
}
