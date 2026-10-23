//go:build integration

package libreta

// Test di integrazione: richiedono il motore OCR di Windows (PowerShell) e le immagini in facsimili\panama.
// Eseguire con: go test -tags integration -run Integration -v ./internal/libreta

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"scanlibretto/internal/imgproc"
)

// provaImmagine riproduce il ciclo di Extract (ritaglio e rotazioni) fermandosi alla prima rotazione con check completi.
func provaImmagine(t *testing.T, nome string) (score int, allOK bool, rot int, dur time.Duration) {
	t.Helper()
	return provaPercorso(t, filepath.Join("..", "..", "facsimili", "panama", nome))
}

// provaPercorso e' provaImmagine su un percorso qualsiasi (skip se il file manca).
func provaPercorso(t *testing.T, path string) (score int, allOK bool, rot int, dur time.Duration) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("immagine assente: %s", path)
	}
	src, err := imgproc.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	base := imgproc.CropContent(src, 10)
	start := time.Now()
	for _, r := range []int{0, 180, 90, 270} {
		d, _, sc, err := readMRZBand(context.Background(), imgproc.Rotate(base, r), "")
		if err != nil {
			t.Fatal(err)
		}
		if sc > score {
			score, rot = sc, r
		}
		if d != nil && d.Checks.AllOK() {
			return sc, true, r, time.Since(start)
		}
	}
	return score, false, rot, time.Since(start)
}

func TestIntegrationMRZLeggibili(t *testing.T) {
	for _, nome := range []string{
		"libreta_2020_pagina-dati_QR.jpg",
		"libreta_2026_anexo8_pagina-dati_A.jpg",
		"libreta_2026_anexo8_pagina-dati_B.jpg",
		"panama.png",
	} {
		t.Run(nome, func(t *testing.T) {
			score, ok, rot, dur := provaImmagine(t, nome)
			t.Logf("score=%d check_tutti_ok=%v rotazione=%d tempo=%.1fs", score, ok, rot, dur.Seconds())
			if !ok {
				t.Errorf("check digit non tutti validi (score %d)", score)
			}
		})
	}
}

// Scansioni reali (stampante laser a 300 dpi, tratto spesso: il motore OCR non legge le righe intere binarizzate
// normalmente). Ognuna ha la sua finestra di inchiostro leggibile: deve bastare la lettura adattiva. Check validi
// alla rotazione attesa e in tempi contenuti (tetto maxBandTime per chiamata, due rotazioni al massimo).
func TestIntegrationScansioniReali(t *testing.T) {
	for _, c := range []struct {
		nome string
		rot  int
	}{{"prova1.png", 180}, {"a4-1.png", -1}, {"libretto-2.png", 0}, {"librettook1.png", 0}, {"librettook4.png", 0}, {"librettook7.png", 0},
		// libretti veri: il bordo della pagina finisce nella fascia della riga 2 (barra sottile isolata, vedi
		// imgproc.TrimRowEdgeBars); senza il taglio la barra sta accanto ai check digit e vero1 dava 3/5
		{"libretto-vero1.png", 0}, {"libretto-vero2.png", 0},
		// scansioni TWAIN dello stesso libretto (2026): il motore OCR non legge mai le righe intere, anche se i ritagli
		// sono perfetti (provamrz: solo i pezzi di 5-15 caratteri); scana2 ha una foto scura sopra la MRZ, che con
		// ClearLines vecchio tagliava l'inizio delle righe; DEFINITIVO_spero ha le righe inclinate di ~1,2 gradi
		// (sotto DeskewMinAngle); vero5 ha un nome lungo con tante '<' che il motore non legge (fillFiller)
		{"provamrz.png", 0}, {"scana2.png", 0}, {"DEFINITIVO_spero.png", 0}, {"vero16.png", 0}, {"libretto-vero5.png", 0}} {
		t.Run(c.nome, func(t *testing.T) {
			path := filepath.Join("..", "..", "scans", c.nome)
			if _, err := os.Stat(path); err != nil { // le prime scansioni di prova stanno in scansecchi test WIA
				path = filepath.Join("..", "..", "scans", "vecchi test WIA", c.nome)
			}
			score, ok, rot, dur := provaPercorso(t, path)
			t.Logf("score=%d check_tutti_ok=%v rotazione=%d tempo=%.1fs", score, ok, rot, dur.Seconds())
			if !ok || score < scoreSure || (c.rot >= 0 && rot != c.rot) { // rot < 0: orientazione qualsiasi (scansione sostituita piu' volte)
				t.Errorf("attesi check validi, emittente PAN e riga 1 esatta a %d gradi: ok=%v rot=%d score=%d", c.rot, ok, rot, score)
			}
			if dur > 2*maxBandTime {
				t.Errorf("troppo lenta: %.1fs", dur.Seconds())
			}
		})
	}
}

// Le pagine con dati oscurati da AMP possono avere MRZ illeggibile: l'esito atteso e' nessun crash
// e un punteggio basso (non una lettura completa).
func TestIntegrationMRZOscurate(t *testing.T) {
	for _, nome := range []string{
		"libreta_2019_appendice6_pagina-dati.jpg",
		"libreta_2017_pagina-dati_PDF417_oscurata.jpg",
	} {
		t.Run(nome, func(t *testing.T) {
			score, ok, _, dur := provaImmagine(t, nome)
			t.Logf("score=%d check_tutti_ok=%v tempo=%.1fs", score, ok, dur.Seconds())
			if score >= 9 {
				t.Errorf("punteggio troppo alto per una pagina oscurata: %d", score)
			}
		})
	}
}

// Una pagina capovolta non deve dare una lettura (e deve costare poco: una sola chiamata OCR).
func TestIntegrationCapovoltaNonLetta(t *testing.T) {
	nome := "libreta_2026_anexo8_pagina-dati_B.jpg"
	src, err := imgproc.Load(filepath.Join("..", "..", "facsimili", "panama", nome))
	if err != nil {
		t.Skip("immagine assente")
	}
	img := imgproc.Rotate(imgproc.CropContent(src, 10), 180)
	start := time.Now()
	d, _, score, err := readMRZBand(context.Background(), img, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("score=%d tempo=%.1fs", score, time.Since(start).Seconds())
	if d != nil || score != 0 {
		t.Errorf("pagina capovolta: atteso nessun MRZ, score %d", score)
	}
}
