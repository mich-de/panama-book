//go:build integration

package libreta

// Misura dello spessore del tratto sulle immagini reali: la scansione a 300 dpi deve risultare piu' spessa dei
// facsimili (soglia adattiva piu' bassa) e il facsimile a tratto sottile non deve ricevere varianti binarizzate.
// Riporta solo numeri (soglie e rapporti), mai valori dei campi. Salta se le immagini mancano.
// Eseguire con: go test -tags integration -run VIZIntegrationSpessore -v ./internal/libreta

import (
	"os"
	"path/filepath"
	"testing"

	"scanlibretto/internal/imgproc"
)

func vizSoglia(t *testing.T, path string, rot int) (thr int, ref float64) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("immagine assente: %s", path)
	}
	src, err := imgproc.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	g := vizToGray(imgproc.Rotate(imgproc.CropContent(src, 10), rot))
	geo, ok := vizLocateMRZ(g)
	if !ok {
		t.Fatalf("%s: MRZ non trovata", filepath.Base(path))
	}
	thr, ref = vizAdaptiveThreshold(g, geo)
	t.Logf("%s: soglia adattiva %d, spessore/altezza a soglia 100 = %.3f", filepath.Base(path), thr, ref)
	return thr, ref
}

func TestVIZIntegrationSpessore(t *testing.T) {
	fac := filepath.Join("..", "..", "facsimili", "panama")
	real, _ := vizSoglia(t, filepath.Join("..", "..", "scans", "prova1.png"), 180)
	panama, _ := vizSoglia(t, filepath.Join(fac, "panama.png"), 0)
	qr, _ := vizSoglia(t, filepath.Join(fac, "libreta_2020_pagina-dati_QR.jpg"), 0)
	vizSoglia(t, filepath.Join(fac, "libreta_2026_anexo8_pagina-dati_A.jpg"), 0)
	if real >= panama || panama >= qr {
		t.Errorf("ordine degli spessori inatteso: scansione %d, panama.png %d, QR 2020 %d (attesa scansione < panama < QR)", real, panama, qr)
	}
	if v := vizExtraVariants(qr); v != nil {
		t.Errorf("il facsimile a tratto sottile non dovrebbe avere varianti binarizzate (%d)", len(v))
	}
	if v := vizExtraVariants(real); len(v) == 0 {
		t.Error("la scansione a tratto spesso dovrebbe avere varianti binarizzate")
	}
}
