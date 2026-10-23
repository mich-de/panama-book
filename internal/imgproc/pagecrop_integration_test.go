//go:build integration

package imgproc

// Test di integrazione sulle immagini reali (saltano se mancano). Per ispezionare i ritagli a occhio:
// PAGECROP_OUT=<cartella> go test -tags integration -run IntegrationCropDataPage -v ./internal/imgproc

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationCropDataPage(t *testing.T) {
	root := filepath.Join("..", "..")
	casi := []struct {
		path string
		rot  int // rotazione per portare la pagina dritta
	}{
		{filepath.Join(root, "scans", "prova1.png"), 180},
		{filepath.Join(root, "facsimili", "panama", "libreta_2020_pagina-dati_QR.jpg"), 0},
		{filepath.Join(root, "facsimili", "panama", "libreta_2026_anexo8_pagina-dati_A.jpg"), 0},
		{filepath.Join(root, "facsimili", "panama", "libreta_2026_anexo8_pagina-dati_B.jpg"), 0},
		{filepath.Join(root, "facsimili", "panama", "panama.png"), 0},
		{filepath.Join(root, "scans", "a4-1.png"), 180},
		{filepath.Join(root, "scans", "libretto-2.png"), 0},
		{filepath.Join(root, "scans", "librettook1.png"), 0},
		{filepath.Join(root, "scans", "librettook4.png"), 0},
		{filepath.Join(root, "scans", "librettook7.png"), 0},
		// libretti veri: fondo scanner (~240) visibile a destra e a sinistra, MRZ non fuori centro per colpa dei bordi
		{filepath.Join(root, "scans", "libretto-vero1.png"), 0},
		{filepath.Join(root, "scans", "libretto-vero2.png"), 0},
	}
	out := os.Getenv("PAGECROP_OUT")
	for _, c := range casi {
		t.Run(filepath.Base(c.path), func(t *testing.T) {
			if _, err := os.Stat(c.path); err != nil {
				t.Skipf("immagine assente: %s", c.path)
			}
			src, err := Load(c.path)
			if err != nil {
				t.Fatal(err)
			}
			base, _ := Deskew(CropContent(src, 10)) // stesso raddrizzamento di libreta.DataPage
			img := Rotate(base, c.rot)
			g, ok := DataPageGeometry(img)
			if !ok {
				t.Fatal("righe MRZ non trovate")
			}
			crop, ok := CropDataPage(img)
			if !ok {
				t.Fatal("CropDataPage fallita")
			}
			cb := crop.Bounds()
			aspetto := float64(cb.Dx()) / float64(cb.Dy())
			b := img.Bounds()
			t.Logf("immagine %dx%d, scala %.2f px/mm, rect %v, bordi trovati L=%v R=%v B=%v, ritaglio %dx%d, aspetto %.3f (atteso %.3f)",
				b.Dx(), b.Dy(), g.Scale, g.Rect, g.EdgeL, g.EdgeR, g.EdgeB, cb.Dx(), cb.Dy(), aspetto, 125.0/88.0)
			if filepath.Base(c.path) == "libretto-vero1.png" || filepath.Base(c.path) == "libretto-vero2.png" {
				if !g.EdgeL || !g.EdgeR || !g.EdgeB {
					t.Errorf("bordi della pagina non trovati sul libretto vero: L=%v R=%v B=%v", g.EdgeL, g.EdgeR, g.EdgeB)
				}
			}
			// le due righe MRZ devono stare per intero nel rettangolo (coordinate dell'immagine)
			in := image.Rect(g.R1.X0, g.R1.Y0, g.R2.X1, g.R2.Y1)
			if !in.In(g.Rect.Intersect(b)) {
				t.Errorf("righe MRZ %v non interamente nel ritaglio %v", in, g.Rect.Intersect(b))
			}
			// aspetto 125:88 entro il 3% (se il rettangolo non e' stato limitato ai bordi dell'immagine)
			if g.Rect.In(b) || (g.EdgeL && g.EdgeR) {
				if d := math.Abs(aspetto/(125.0/88.0) - 1); d > 0.03 {
					t.Errorf("aspetto %.3f fuori dal 3%% da 125:88", aspetto)
				}
			} else {
				t.Logf("rettangolo limitato ai bordi dell'immagine: aspetto non verificabile")
			}
			if out != "" && !strings.Contains(filepath.Base(c.path), "vero") { // niente copie dei documenti veri fuori da scans\
				_ = os.MkdirAll(out, 0o755)
				_ = SavePNG(crop, filepath.Join(out, "crop_"+filepath.Base(c.path)+".png"))
			}
		})
	}
}
