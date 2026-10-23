//go:build integration

package libreta

// Test di integrazione della lettura dei campi stampati: richiedono il motore OCR di Windows (PowerShell) e le
// immagini in facsimili\panama (saltano se mancano). Riportano solo CONTEGGI, mai i valori dei campi
// (contengono dati personali); per ispezionare i valori impostare VIZ_SHOW=1 in locale.
// Eseguire con: go test -tags integration -run VIZIntegration -v ./internal/libreta

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/ocr"
)

var vizAllKeys = []string{"tipo", "paese", "nacionalidad", "luogo_nascita", "data_emissione", "data_scadenza",
	"data_nascita", "livello", "cargo", "numero_libretto", "identificazione", "refrendo", "id_marino", "regionale"}

// vizProva esegue prepareVIZImage + OCR + extractVIZ + finalizeVIZ come Extract (senza MRZ: l'MRZ e' dell'altro agente).
func vizProva(t *testing.T, nome string) (*Result, bool) {
	t.Helper()
	return vizProvaModo(t, nome, false)
}

// vizProvaModo: con pagina=true salta il foglio e legge la pagina intera ingrandita (modo di ripiego).
func vizProvaModo(t *testing.T, nome string, pagina bool) (*Result, bool) {
	t.Helper()
	return vizProvaFile(t, filepath.Join("..", "..", "facsimili", "panama", nome), nome, 0, pagina)
}

// vizProvaFile legge l'immagine in path, la ruota di rot gradi (come fa Extract dopo aver trovato la MRZ) e
// poi esegue il percorso VIZ.
func vizProvaFile(t *testing.T, path, nome string, rot int, pagina bool) (*Result, bool) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("immagine assente: %s", path)
	}
	src, err := imgproc.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	deskewed, _ := imgproc.Deskew(imgproc.CropContent(src, 10)) // come Extract: prima delle rotazioni
	base := imgproc.Rotate(deskewed, rot)
	var prep image.Image
	if pagina {
		prep = vizPreparePage(vizToGray(base), autoScale(base.Bounds().Dx()))
	} else {
		prep = prepareVIZImage(base)
	}
	dbg := filepath.Join("..", "..", "_out", "viz_t")
	_ = os.MkdirAll(dbg, 0o755)
	if os.Getenv("VIZ_SHOW") != "" {
		if cs := vizStash; cs != nil { // ritagli singoli, per ispezione
			cd := filepath.Join(dbg, "crops_"+nome)
			_ = os.MkdirAll(cd, 0o755)
			for slot, g := range cs.crops {
				_ = imgproc.SavePNG(g, filepath.Join(cd, fmt.Sprintf("c%03d.png", slot)))
			}
		}
	}
	tmp := filepath.Join(dbg, "sheet_"+nome+".png")
	if err := imgproc.SavePNG(prep, tmp); err != nil {
		t.Fatal(err)
	}
	page, err := ocr.Recognize(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	isSheet := page.Width == vizSheetW && page.Height == vizSheetH()
	if os.Getenv("VIZ_SHOW") != "" {
		_ = os.WriteFile(filepath.Join(dbg, "sheet_"+nome+".txt"), []byte(dump(page)), 0o644)
	}
	res := &Result{Fields: map[string]string{}}
	extractVIZ(page, res)
	finalizeVIZ(res)
	return res, isSheet
}

// vizScanPath cerca la scansione in scans\ e, se manca, in scans\vecchi test*\ (dove sono finite le prime).
func vizScanPath(nome string) string {
	for _, d := range []string{"", "vecchi test", "vecchi test WIA"} {
		if p := filepath.Join("..", "..", "scans", d, nome); fileExists(p) {
			return p
		}
	}
	return filepath.Join("..", "..", "scans", nome)
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// vizRotazione trova la rotazione (0 o 180) con cui la MRZ si legge con tutti i check digit, come fa Extract:
// le scansioni vengono salvate a volte capovolte e a volte no, non si puo' fissare la rotazione nel test.
func vizRotazione(t *testing.T, path string) int {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("immagine assente: %s", path)
	}
	src, err := imgproc.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	base := imgproc.CropContent(src, 10)
	for _, rot := range []int{0, 180} {
		d, _, _, err := readMRZBand(context.Background(), imgproc.Rotate(base, rot), "")
		if err != nil {
			t.Fatal(err)
		}
		if d != nil && d.Checks.AllOK() {
			return rot
		}
	}
	t.Logf("%s: nessuna rotazione con MRZ valida, si usa 0", filepath.Base(path))
	return 0
}

func vizRiporta(t *testing.T, nome string, res *Result, isSheet bool, attesi []string) {
	t.Helper()
	var missing []string
	found := 0
	for _, k := range vizAllKeys {
		if res.Fields[k] != "" {
			found++
		} else {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	t.Logf("%s: modo foglio=%v, campi trovati %d/%d, mancanti %v, avvisi %d", nome, isSheet, found, len(vizAllKeys), missing, len(res.Warnings))
	if os.Getenv("VIZ_SHOW") != "" {
		for _, k := range vizAllKeys {
			t.Logf("   %s = %q", k, res.Fields[k])
		}
		for _, w := range res.Warnings {
			t.Logf("   avviso: %s", w)
		}
	}
	// verita' di terreno facoltativa (file JSON fuori dal repo, VIZ_TRUTH): riporta solo i NOMI dei campi sbagliati
	if tp := os.Getenv("VIZ_TRUTH"); tp != "" {
		if raw, err := os.ReadFile(tp); err == nil {
			var truth map[string]map[string]string
			if json.Unmarshal(raw, &truth) == nil {
				if want, ok := truth[nome]; ok {
					var wrong []string
					okN := 0
					for _, k := range vizAllKeys {
						if res.Fields[k] == want[k] {
							okN++
						} else {
							wrong = append(wrong, k)
						}
					}
					t.Logf("%s: CORRETTI %d/%d, errati o mancanti %v", nome, okN, len(vizAllKeys), wrong)
				}
			}
		}
	}
	for _, k := range attesi {
		if res.Fields[k] == "" {
			t.Errorf("%s: campo atteso non letto: %s", nome, k)
		}
	}
}

func TestVIZIntegrationImmagini(t *testing.T) {
	for _, nome := range []string{
		"libreta_2020_pagina-dati_QR.jpg",
		"libreta_2026_anexo8_pagina-dati_A.jpg",
		"libreta_2026_anexo8_pagina-dati_B.jpg",
		"panama.png",
		"libreta_2019_appendice6_pagina-dati.jpg",
		"libreta_2017_pagina-dati_PDF417_oscurata.jpg",
	} {
		t.Run(nome, func(t *testing.T) {
			res, isSheet := vizProva(t, nome)
			vizRiporta(t, nome, res, isSheet, nil)
		})
	}
}

// TestVIZIntegrationScansioneReale prova le scansioni reali a 300 dpi (scans\prova1.png e scans\libretto-2.png: stampa
// di Anexo 8 A; scans\a4-1.png: stampa di Anexo 8 B; l'orientamento delle scansioni cambia, la rotazione la trova
// Extract): la stampante deposita un nero piu' spesso di quello dei facsimili e il motore OCR non legge il testo se i
// tratti non vengono assottigliati (vedi vizAdaptiveThreshold). Salta se il file manca. Con VIZ_TRUTH controlla i
// valori (riporta solo i nomi dei campi sbagliati).
func TestVIZIntegrationScansioneReale(t *testing.T) {
	for _, nome := range []string{"prova1.png", "a4-1.png", "libretto-2.png", "libretto-vero1.png", "libretto-vero2.png", "vero.png"} {
		t.Run(nome, func(t *testing.T) {
			path := vizScanPath(nome)
			if _, err := os.Stat(path); err != nil {
				t.Skipf("immagine assente: %s", path)
			}
			// percorso completo (MRZ + campi stampati), come il comando extract: l'MRZ calibra il voto e prevale
			start := time.Now()
			res, err := Extract(context.Background(), path, Options{})
			if err != nil {
				t.Fatal(err)
			}
			sec := time.Since(start).Seconds()
			vizRiporta(t, nome, res, true, nil)
			found := 0
			for _, k := range vizAllKeys {
				if res.Fields[k] != "" {
					found++
				}
			}
			t.Logf("%s: rotazione %d, check digit MRZ tutti validi %v, %.1f s in tutto, avvisi: %v", nome, res.Rotation,
				res.MRZ != nil && res.MRZ.Checks.AllOK(), sec, res.Warnings)
			// soglie relative: la stampa A ha il refrendo, la B no (13 campi stampati al massimo)
			if found < 12 {
				t.Errorf("%s: campi trovati %d/%d, attesi almeno 12", nome, found, len(vizAllKeys))
			}
			if sec > 15 {
				t.Errorf("%s: estrazione troppo lenta: %.1f s", nome, sec)
			}
		})
	}
}

// TestVIZIntegrationPaginaIntera prova il modo di ripiego (OCR della pagina intera, senza il foglio dei ritagli).
func TestVIZIntegrationPaginaIntera(t *testing.T) {
	for _, nome := range []string{
		"libreta_2020_pagina-dati_QR.jpg",
		"libreta_2026_anexo8_pagina-dati_A.jpg",
		"libreta_2026_anexo8_pagina-dati_B.jpg",
		"panama.png",
	} {
		t.Run(nome, func(t *testing.T) {
			res, isSheet := vizProvaModo(t, nome, true)
			vizRiporta(t, nome, res, isSheet, nil)
		})
	}
}
