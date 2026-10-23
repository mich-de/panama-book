//go:build integration

package libreta

// Diagnostica (non asserisce nulla): per ogni scansione reale in scans\ misura quali varianti assottigliate fanno
// leggere la riga 2 dal motore OCR, intera o a pezzi. Serve a tarare thinInkTargets. Si lancia con
// go test -tags integration -run Sweep -v ./internal/libreta

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/mrz"
)

func TestSweepPezzi(t *testing.T) {
	cases := []struct {
		nome string
		rot  int
	}{{"libretto-2.png", 0}, {"a4-1.png", 0}, {"prova1.png", 180}}
	variants := []struct {
		e int
		f float64
	}{{0, 0.14}, {0, 0.18}, {0, 0.22}, {1, 0.16}, {1, 0.20}, {1, 0.24}, {1, 0.28}}
	for _, c := range cases {
		path := filepath.Join("..", "..", "scans", c.nome)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		src, err := imgproc.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		img := imgproc.Rotate(imgproc.CropContent(src, 10), c.rot)
		bin := imgproc.Binarize(img)
		imgproc.ClearLines(bin)
		pairs := imgproc.MRZPairs(imgproc.TextRows(bin, 0.35), img.Bounds().Dx())
		if len(pairs) == 0 {
			t.Logf("%s: nessuna coppia", c.nome)
			continue
		}
		g2 := imgproc.GrayCrop(img, pairs[0][1], 0, 0)
		for _, v := range variants {
			im, _ := imgproc.ThinToInk(g2, v.e, v.f)
			row := imgproc.PadWhite(im, 30)
			grid, ok := newCellGrid(row)
			if !ok {
				t.Logf("%s e%d f%.2f: griglia non costruita", c.nome, v.e, v.f)
				continue
			}
			pim, pnm, slots := grid.pieces(1, "")
			txt, err := ocrImages(context.Background(), pim, pnm, "")
			if err != nil {
				t.Fatal(err)
			}
			var pv pieceVotes
			pv.add(slots, txt)
			line, _ := voteLine(pv[1])
			n := 0
			for _, tx := range txt {
				if len(mrzClean(tx)) >= 4 {
					n++
				}
			}
			d, _, err := mrz.Repair("ISPAN<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<", line)
			ok2 := err == nil && d.Checks.DocNumber && d.Checks.Birth && d.Checks.Expiry && d.Checks.Composite
			t.Logf("%s e%d f%.2f: pezzi letti %d/%d, check riga2 ok=%v", c.nome, v.e, v.f, n, len(txt), ok2)
		}
	}
}
