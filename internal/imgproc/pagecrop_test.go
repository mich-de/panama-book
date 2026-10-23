package imgproc

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// mrzSintetica disegna su sfondo bianco una "pagina dati" TD3 virtuale a scala s px/mm con angolo in alto a
// sinistra in (px,py): due righe da 44 rettangoli neri a passo 2,54 mm (larghezza glifo 1,94 mm (due aste), altezza 3 mm,
// interlinea 5 mm), la seconda riga con il fondo a td3BottomMM dal bordo inferiore della pagina.
// Restituisce l'immagine e il rettangolo vero della pagina.
func mrzSintetica(w, h int, s float64, px, py int) (*image.Gray, image.Rectangle) {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	page := image.Rect(px, py, px+int(td3PageW*s+0.5), py+int(td3PageH*s+0.5))
	bottom2 := float64(page.Max.Y) - td3BottomMM*s
	left := float64(px) + ((td3PageW-td3Chars*td3Pitch)/2+td3InkSlack/2)*s
	for line := 0; line < 2; line++ {
		yb := bottom2 - float64(1-line)*5*s // fondo della riga
		y0, y1 := int(yb-3*s+0.5), int(yb+0.5)
		for c := 0; c < td3Chars; c++ {
			x0 := int(left + float64(c)*td3Pitch*s + 0.5)
			x1 := int(left + (float64(c)*td3Pitch+1.94)*s + 0.5)
			bar := max(1, int(0.5*s)) // glifo fatto di due aste: ClearLines scarta le righe quasi tutte scure
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					if (x < x0+bar || x >= x1-bar) && x >= 0 && x < w && y >= 0 && y < h {
						img.SetGray(x, y, color.Gray{0})
					}
				}
			}
		}
	}
	return img, page
}

func TestDataPageGeometrySintetica(t *testing.T) {
	for _, s := range []float64{4, 5.9, 8, 11.8} {
		w, h := int(125*s)+200, int(88*s)+200
		img, page := mrzSintetica(w, h, s, 100, 100)
		g, ok := DataPageGeometry(img)
		if !ok {
			t.Fatalf("scala %.1f: righe MRZ non trovate", s)
		}
		if math.Abs(g.Scale/s-1) > 0.01 {
			t.Errorf("scala %.2f: ricavata %.3f (oltre 1%%)", s, g.Scale)
		}
		tol := int(s) + 1 // ~1 mm
		if abs(g.Rect.Min.X-page.Min.X) > tol || abs(g.Rect.Min.Y-page.Min.Y) > tol ||
			abs(g.Rect.Max.X-page.Max.X) > tol || abs(g.Rect.Max.Y-page.Max.Y) > tol {
			t.Errorf("scala %.1f: rettangolo %v, atteso %v", s, g.Rect, page)
		}
		crop, ok := CropDataPage(img)
		if !ok {
			t.Fatalf("scala %.1f: CropDataPage fallita", s)
		}
		cb := crop.Bounds()
		if cb.Min != (image.Point{}) {
			t.Errorf("il ritaglio deve avere origine (0,0): %v", cb)
		}
		if a := float64(cb.Dx()) / float64(cb.Dy()); math.Abs(a/(125.0/88.0)-1) > 0.01 {
			t.Errorf("scala %.1f: aspetto %.3f, atteso 125:88", s, a)
		}
	}
}

func TestCropDataPageLimitataAiBordi(t *testing.T) {
	// pagina che sporge a sinistra e in alto: il rettangolo va limitato all'immagine
	s := 5.0
	img, _ := mrzSintetica(int(125*s)+20, int(88*s)+20, s, 3, -10)
	crop, ok := CropDataPage(img)
	if !ok {
		t.Fatal("CropDataPage fallita")
	}
	if b := crop.Bounds(); b.Min.X != 0 || b.Min.Y != 0 || b.Dx() > img.Bounds().Dx() || b.Dy() > img.Bounds().Dy() {
		t.Errorf("ritaglio fuori dai bordi: %v su immagine %v", b, img.Bounds())
	}
}

func TestCropDataPageSenzaMRZ(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 800, 600))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	if c, ok := CropDataPage(img); ok || c != nil {
		t.Error("immagine bianca: atteso (nil,false)")
	}
	// una sola riga di testo: nessuna coppia
	for y := 300; y < 320; y++ {
		for x := 100; x < 700; x += 2 {
			img.SetGray(x, y, color.Gray{0})
		}
	}
	if c, ok := CropDataPage(img); ok || c != nil {
		t.Error("una sola riga: atteso (nil,false)")
	}
	// immagine minuscola
	if _, ok := CropDataPage(image.NewGray(image.Rect(0, 0, 50, 50))); ok {
		t.Error("immagine minuscola: atteso false")
	}
}

// pittura il fondo dello scanner (240, piatto) fuori da page e una carta con testura (170/210 a blocchi di 3 px)
// dentro, lasciando intatto l'inchiostro (pixel scuri).
func dipingiPagina(img *image.Gray, page image.Rectangle) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.GrayAt(x, y).Y != 255 {
				continue
			}
			v := uint8(240)
			if (image.Pt(x, y)).In(page) {
				v = 170 + uint8(((x/3)+(y/3))%2)*40
			}
			img.SetGray(x, y, color.Gray{v})
		}
	}
}

// Libretto vero: la MRZ non e' al centro della pagina e c'e' fondo scanner a lato. Il rettangolo segue i bordi
// reali (sinistro, destro, inferiore) e non la geometria centrata.
func TestDataPageGeometryBordiReali(t *testing.T) {
	s := 6.0
	img, page := mrzSintetica(1100, 700, s, 150, 40)
	vera := image.Rect(page.Min.X+30, page.Min.Y, page.Max.X+30, page.Max.Y-12)
	dipingiPagina(img, vera)
	g, ok := DataPageGeometry(img)
	if !ok {
		t.Fatal("righe MRZ non trovate")
	}
	if !g.EdgeL || !g.EdgeR || !g.EdgeB {
		t.Fatalf("bordi non trovati: L=%v R=%v B=%v", g.EdgeL, g.EdgeR, g.EdgeB)
	}
	if abs(g.Rect.Min.X-vera.Min.X) > 3 || abs(g.Rect.Max.X-vera.Max.X) > 3 || abs(g.Rect.Max.Y-vera.Max.Y) > 3 {
		t.Errorf("rettangolo %v, bordi veri %v", g.Rect, vera)
	}
	if a := float64(g.Rect.Dx()) / float64(g.Rect.Dy()); math.Abs(a/(125.0/88.0)-1) > 0.01 {
		t.Errorf("aspetto %.3f, atteso 125:88", a)
	}
}

// Pagina larga 145 mm invece di 125 (+16%): i bordi non sono quelli di una pagina dati, si usa la geometria MRZ.
func TestDataPageGeometryBordiIncoerenti(t *testing.T) {
	s := 6.0
	img, page := mrzSintetica(1200, 700, s, 150, 40)
	dipingiPagina(img, image.Rect(page.Min.X-60, page.Min.Y, page.Max.X+60, page.Max.Y))
	g, ok := DataPageGeometry(img)
	if !ok {
		t.Fatal("righe MRZ non trovate")
	}
	if g.EdgeL || g.EdgeR {
		t.Errorf("bordi incoerenti usati: L=%v R=%v", g.EdgeL, g.EdgeR)
	}
	if abs(g.Rect.Min.X-page.Min.X) > 6 || abs(g.Rect.Max.X-page.Max.X) > 6 {
		t.Errorf("rettangolo %v, atteso vicino a %v", g.Rect, page)
	}
}

// Una barra verticale sottile e isolata (bordo della pagina) vicino alla fine di una riga non fa parte della riga;
// un carattere sottile attaccato al testo (cifra '1' in fondo) si'.
func TestTrimRowEdgeBars(t *testing.T) {
	g := image.NewGray(image.Rect(0, 0, 900, 120))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	ink := func(x0, x1, y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				g.SetGray(x, y, color.Gray{0})
			}
		}
	}
	for c := 0; c < 20; c++ { // riga 1: 20 aste, passo 30 px, altezza 40
		ink(100+30*c, 100+30*c+10, 20, 60)
	}
	ink(100+30*19+75, 100+30*19+78, 20, 60) // barra a 75 px dal testo (> 0,9 altezze = 36 px)
	// riga 2: finisce con un carattere sottile a ~20 px dal precedente (< 36 px): resta
	for c := 0; c < 20; c++ {
		ink(100+30*c, 100+30*c+10, 80, 120)
	}
	ink(100+30*19+30, 100+30*19+33, 80, 120)
	rows := []Row{{Y0: 20, Y1: 60, X0: 100, X1: 100 + 30*19 + 78}, {Y0: 80, Y1: 120, X0: 100, X1: 100 + 30*19 + 33}}
	out := TrimRowEdgeBars(g, rows)
	if want := 100 + 30*19 + 10; out[0].X1 != want {
		t.Errorf("riga con barra: X1=%d, atteso %d", out[0].X1, want)
	}
	if out[1].X1 != rows[1].X1 {
		t.Errorf("riga con carattere sottile attaccato modificata: X1=%d, atteso %d", out[1].X1, rows[1].X1)
	}
	if rows[0].X1 == out[0].X1 {
		t.Error("l'input non deve essere modificato")
	}
}
