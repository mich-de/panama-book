//go:build glyphgen

package libreta

// Strumenti opt-in (build tag glyphgen) del lettore a template: estrazione dei riquadri delle celle dal corpus di
// scansioni. Servono solo a chi rigenera la libreria di glifi o misura l'accuratezza; non fanno parte dei test normali.
//
//	TPL_OUT=<cartella> go test -tags glyphgen -run TestGlyphgenDump -timeout 60m ./internal/libreta
//
// Le etichette (testo MRZ di ogni scansione) si ricavano con la pipeline OCR attuale e restano SOLO nella cartella
// TPL_OUT (fuori dal repository): contengono dati personali. La libreria di glifi prodotta non ne contiene.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"scanlibretto/internal/imgproc"
)

// tplCorpus elenca le immagini del corpus (scans, scans/vecchi test WIA, facsimili/panama), ordinate per percorso.
func tplCorpus(t testing.TB) []string {
	t.Helper()
	root := filepath.Join("..", "..")
	var out []string
	for _, dir := range []string{"scans", filepath.Join("scans", "vecchi test WIA"), filepath.Join("facsimili", "panama")} {
		for _, pat := range []string{"*.png", "*.jpg"} {
			m, _ := filepath.Glob(filepath.Join(root, dir, pat))
			out = append(out, m...)
		}
	}
	sort.Strings(out)
	return out
}

// tplMeta e' la scheda di un'immagine nella cartella di estrazione.
type tplMeta struct {
	ID      int     `json:"id"`
	File    string  `json:"file"`
	Rot     int     `json:"rot"`
	Labeled bool    `json:"labeled"`
	Line1   string  `json:"line1,omitempty"`
	Line2   string  `json:"line2,omitempty"`
	Pitch   float64 `json:"pitch"`
	Quality float64 `json:"quality"`
	RowH    [2]int  `json:"row_h"`
	Width   int     `json:"width"`
	OCRSec  float64 `json:"ocr_sec"`
	BandMs  float64 `json:"band_ms"`
	OCRDone bool    `json:"ocr_done"` // l'etichetta (o la sua assenza) e' gia' stata cercata con l'OCR
	// geometria misurata (px dell'immagine)
	YC    [2]float64 `json:"yc"`
	Slope [2]float64 `json:"slope"`
	Hgt   [2]float64 `json:"height"`
}

func TestGlyphgenDump(t *testing.T) {
	out := os.Getenv("TPL_OUT")
	if out == "" {
		t.Skip("TPL_OUT non impostata")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	only := os.Getenv("TPL_ONLY") // sottostringa del percorso: limita le immagini
	files := tplCorpus(t)
	var metas []tplMeta
	for id, path := range files {
		if only != "" && !strings.Contains(path, only) {
			continue
		}
		meta := tplMeta{ID: id, File: filepath.ToSlash(strings.TrimPrefix(path, filepath.Join("..", "..")+string(filepath.Separator)))}
		src, err := imgproc.Load(path)
		if err != nil {
			t.Logf("%3d %s: %v", id, meta.File, err)
			continue
		}
		base := imgproc.CropContent(src, 10)
		base, _ = imgproc.Deskew(base)
		rot := -1
		var prev tplMeta // scheda precedente: le etichette trovate con l'OCR si riusano (l'OCR costa minuti)
		if js, err := os.ReadFile(filepath.Join(out, fmt.Sprintf("%03d.json", id))); err == nil && os.Getenv("TPL_RELABEL") == "" {
			_ = json.Unmarshal(js, &prev)
		}
		if prev.OCRDone || prev.Labeled {
			meta.Labeled, meta.Line1, meta.Line2, meta.OCRSec, rot = prev.Labeled, prev.Line1, prev.Line2, prev.OCRSec, prev.Rot
			if !meta.Labeled {
				rot = -1
			}
		} else {
			start := time.Now()
			for _, r := range []int{0, 180, 90, 270} {
				img := imgproc.Rotate(base, r)
				d, _, _, err := readMRZBand(context.Background(), img, "")
				if err != nil {
					t.Fatal(err)
				}
				if d != nil && d.Checks.AllOK() {
					rot = r
					meta.Labeled, meta.Line1, meta.Line2 = true, d.Line1, d.Line2
					break
				}
			}
			meta.OCRSec = time.Since(start).Seconds()
		}
		meta.OCRDone = true
		// orientamento per l'estrazione: quello dell'etichetta, altrimenti il primo con una coppia di righe
		var img = imgproc.Rotate(base, max(rot, 0))
		if rot < 0 {
			for _, r := range []int{0, 180, 90, 270} {
				im := imgproc.Rotate(base, r)
				if _, _, pairs := tplFindRows(im); len(pairs) > 0 {
					img, rot = im, r
					break
				}
			}
		}
		meta.Rot = rot
		if rot < 0 {
			t.Logf("%3d %-70s nessuna coppia di righe (%.1fs)", id, meta.File, meta.OCRSec)
			continue
		}
		t0 := time.Now()
		im2, bin, pairs := tplFindRows(img)
		if len(pairs) == 0 {
			t.Logf("%3d %-70s nessuna coppia", id, meta.File)
			continue
		}
		band, ok := tplMakeBand(im2, bin, pairs[0][0], pairs[0][1])
		meta.BandMs = float64(time.Since(t0).Microseconds()) / 1000
		if !ok {
			t.Logf("%3d %-70s griglia non adattata", id, meta.File)
			continue
		}
		meta.Pitch, meta.Quality = band.pitch, band.quality
		meta.YC, meta.Slope, meta.Hgt = band.yc, band.slope, band.height
		meta.RowH = [2]int{band.rows[0].Height(), band.rows[1].Height()}
		meta.Width = im2.Bounds().Dx()
		var raw []byte
		for r := 0; r < 2; r++ {
			for k := 0; k < tplCells; k++ {
				raw = append(raw, band.frames[r][k]...)
			}
		}
		base2 := filepath.Join(out, fmt.Sprintf("%03d", id))
		if err := os.WriteFile(base2+".frames", raw, 0o644); err != nil {
			t.Fatal(err)
		}
		js, _ := json.MarshalIndent(meta, "", " ")
		if err := os.WriteFile(base2+".json", js, 0o644); err != nil {
			t.Fatal(err)
		}
		metas = append(metas, meta)
		t.Logf("%3d %-70s rot=%3d label=%v pitch=%.2f q=%.3f rowH=%v ocr=%.1fs band=%.0fms", id, meta.File, rot, meta.Labeled, meta.Pitch, meta.Quality, meta.RowH, meta.OCRSec, meta.BandMs)
	}
	tplWriteLabels(t, out, metas)
}

// tplWriteLabels scrive labels.json (formato di TestGlyphgenEval/Build) con il testo MRZ di ogni immagine letta
// dall'OCR. Le scansioni dello stesso libretto (stessa riga 2) votano cella per cella: la riga 2 e' validata dai
// check digit, la riga 1 no (l'OCR di Windows lascia spesso buchi sui '<' e sui nomi lunghi), quindi il voto la
// corregge solo se le scansioni sono abbastanza. Si segnalano i libretti con meno di 3 scansioni o con celle della riga
// 1 discordanti: la loro riga 1 va controllata a occhio (e corretta nel file) prima di rigenerare la libreria.
func tplWriteLabels(t *testing.T, out string, metas []tplMeta) {
	groups := map[string][]tplMeta{}
	for _, m := range metas {
		if m.Labeled && len(m.Line1) == tplCells && len(m.Line2) == tplCells {
			k := m.Line2[0:10] + m.Line2[13:20]
			groups[k] = append(groups[k], m)
		}
	}
	labels := map[string]tplLabel{}
	var keys []string
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for gi, k := range keys {
		mem := groups[k]
		var l1, l2 [tplCells]byte
		disagree := 0
		for c := 0; c < tplCells; c++ {
			for r, dst := range [2]*[tplCells]byte{&l1, &l2} {
				votes := map[byte]int{}
				for _, m := range mem {
					line := [2]string{m.Line1, m.Line2}[r]
					votes[line[c]]++
				}
				best, bestN := byte(0), 0
				for ch, n := range votes {
					if n > bestN || (n == bestN && ch < best) {
						best, bestN = ch, n
					}
				}
				dst[c] = best
				if r == 0 && len(votes) > 1 {
					disagree++
				}
			}
		}
		for _, m := range mem {
			labels[m.File] = tplLabel{Rot: m.Rot, Line1: string(l1[:]), Line2: string(l2[:])}
		}
		note := ""
		if len(mem) < 3 || disagree > 0 {
			note = "  <-- controllare a occhio la riga 1"
		}
		t.Logf("libretto %d: %d scansioni, %d celle della riga 1 discordanti%s", gi, len(mem), disagree, note)
	}
	js, _ := json.MarshalIndent(labels, "", " ")
	if err := os.WriteFile(filepath.Join(out, "labels.json"), js, 0o644); err != nil {
		t.Fatal(err)
	}
}
