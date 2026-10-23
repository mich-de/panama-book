package imgproc

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// paginaTesto disegna una pagina sintetica: due colonne di "righe di testo" (tratti verticali alternati) e un
// blocco di righe lunghe in fondo, su fondo bianco.
func paginaTesto() *image.RGBA {
	w, h := 1200, 1500
	g := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	riga := func(x0, x1, y0 int) {
		for y := y0; y < y0+22; y++ {
			for x := x0; x < x1; x++ {
				if (x/4)%2 == 0 {
					g.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
				}
			}
		}
	}
	for k := 0; k < 14; k++ {
		riga(100, 560, 120+k*45)
		riga(640, 1100, 120+k*45)
	}
	for k := 0; k < 8; k++ {
		riga(100, 1100, 850+k*50)
	}
	return g
}

func TestEstimateSkewAngoliNoti(t *testing.T) {
	base := paginaTesto()
	if a := EstimateSkew(base); math.Abs(a) > 0.5 {
		t.Errorf("pagina dritta: stimato %.1f", a)
	}
	for _, want := range []float64{-12, -9, -5.6, -1.5, 1.5, 5.6, 9, 12} {
		rot := RotateAngle(base, want)
		if got := EstimateSkew(rot); math.Abs(got-want) > 0.5 {
			t.Errorf("angolo %.1f: stimato %.1f", want, got)
		}
	}
}

func TestDeskewRaddrizza(t *testing.T) {
	rot := RotateAngle(paginaTesto(), 5.6)
	fix, a := Deskew(rot)
	if math.Abs(a-5.6) > 0.5 {
		t.Errorf("angolo restituito %.1f", a)
	}
	if res := EstimateSkew(fix); math.Abs(res) > 0.5 {
		t.Errorf("dopo Deskew resta inclinazione %.1f", res)
	}
	// pagina quasi dritta: non si tocca
	small := RotateAngle(paginaTesto(), 0.6)
	if out, a := Deskew(small); a != 0 || out != image.Image(small) {
		t.Errorf("sotto la soglia non deve ruotare (angolo %.1f)", a)
	}
	// pagina bianca: nessun angolo
	if a := EstimateSkew(image.NewRGBA(image.Rect(0, 0, 400, 400))); a != 0 {
		t.Errorf("pagina bianca: %.1f", a)
	}
}

func TestRotateAngleEspandeSenzaPerdite(t *testing.T) {
	g := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for i := range g.Pix {
		g.Pix[i] = 0 // tutto nero
		if i%4 == 3 {
			g.Pix[i] = 255
		}
	}
	r := RotateAngle(g, 30)
	if r.Bounds().Dx() <= 200 || r.Bounds().Dy() <= 100 {
		t.Errorf("dimensioni non espanse: %v", r.Bounds())
	}
	// area nera conservata (entro l'interpolazione ai bordi): ~200*100 px
	n := 0
	for y := 0; y < r.Bounds().Dy(); y++ {
		for x := 0; x < r.Bounds().Dx(); x++ {
			if gray(r.At(x, y)) < 128 {
				n++
			}
		}
	}
	if n < 19000 || n > 21000 {
		t.Errorf("area nera %d, attesa ~20000", n)
	}
	z := RotateAngle(g, 0)
	if z.Bounds().Dx() != 200 || z.Bounds().Dy() != 100 {
		t.Errorf("0 gradi: %v", z.Bounds())
	}
}

// Due righe tipo MRZ (tratti verticali alti 36 px) inclinate di deg gradi: EstimateRowsSkew deve ritrovare
// l'angolo anche sotto DeskewMinAngle (0,5-1,5 gradi, il caso che fa restituire testo vuoto al motore OCR).
func TestEstimateRowsSkewRigheMRZ(t *testing.T) {
	disegna := func(deg float64) (*image.Gray, []Row) {
		g := image.NewGray(image.Rect(0, 0, 1500, 300))
		for i := range g.Pix {
			g.Pix[i] = 255
		}
		tg := math.Tan(deg * math.Pi / 180)
		for _, y0 := range []int{100, 170} {
			for x := 100; x < 1400; x++ {
				if (x/14)%2 != 0 || x%14 > 5 {
					continue
				}
				dy := int(math.Round(float64(x-750) * tg))
				for y := y0; y < y0+36; y++ {
					g.Pix[(y+dy)*g.Stride+x] = 0
				}
			}
		}
		return g, []Row{{Y0: 70, Y1: 140, X0: 100, X1: 1400}, {Y0: 150, Y1: 220, X0: 100, X1: 1400}}
	}
	for _, want := range []float64{-1.25, -0.5, 0, 0.5, 1.0, 1.5} {
		g, rows := disegna(want)
		if got := EstimateRowsSkew(g, rows...); math.Abs(got-want) > 0.15 {
			t.Errorf("inclinazione %.2f: stimata %.2f", want, got)
		}
	}
	if got := EstimateRowsSkew(image.NewGray(image.Rect(0, 0, 100, 100)), Row{0, 20, 0, 90}); got != 0 {
		t.Errorf("immagine senza inchiostro: stimata %.2f", got)
	}
}

// ShearRows: righe inclinate di 1,2 gradi tornano dritte (inclinazione residua ~0, righe piu' basse) senza
// interpolare: l'inchiostro resta identico (stesso numero di pixel scuri).
func TestShearRowsRaddrizzaSenzaInterpolare(t *testing.T) {
	const w, h = 1500, 300
	g := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	tg := math.Tan(1.2 * math.Pi / 180)
	for _, y0 := range []int{100, 170} {
		for x := 100; x < 1400; x++ {
			if (x/14)%2 != 0 || x%14 > 5 {
				continue
			}
			dy := int(math.Round(float64(x-750) * tg))
			for y := y0; y < y0+36; y++ {
				g.SetRGBA(x, y+dy, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	bin := Binarize(g)
	rows := TextRows(bin, 0.35)
	if len(rows) != 2 || rows[0].Height() < 50 {
		t.Fatalf("prima: attese 2 righe inclinate (alte >= 50), trovate %v", rows)
	}
	if got := EstimateRowsSkew(bin, rows...); math.Abs(got-1.2) > 0.15 {
		t.Fatalf("inclinazione stimata %.2f", got)
	}
	fix := ShearRows(g, 1.2)
	fb := Binarize(fix)
	frows := TextRows(fb, 0.35)
	if len(frows) != 2 || frows[0].Height() > 40 || frows[1].Height() > 40 {
		t.Fatalf("dopo: attese 2 righe alte ~36, trovate %v", frows)
	}
	if got := EstimateRowsSkew(fb, frows...); math.Abs(got) > 0.15 {
		t.Errorf("inclinazione residua %.2f", got)
	}
	ink := func(b *image.Gray) int {
		n := 0
		for _, v := range b.Pix {
			if v == 0 {
				n++
			}
		}
		return n
	}
	if a, b := ink(bin), ink(fb); a != b {
		t.Errorf("pixel di inchiostro %d -> %d: lo scorrimento non deve cambiarli", a, b)
	}
}
