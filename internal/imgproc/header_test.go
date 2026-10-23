package imgproc

import (
	"image"
	"image/color"
	"testing"
)

// pagina sintetica: sfondo chiaro, testo blu da y0 a y0+20 nella fascia centrale.
func headerPage(w, h, y0 int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{225, 230, 225, 255})
		}
	}
	for y := y0; y < y0+20; y++ {
		for x := w * 35 / 100; x < w*65/100; x++ {
			img.Set(x, y, color.RGBA{40, 50, 160, 255})
		}
	}
	return img
}

func TestHeaderTopETrim(t *testing.T) {
	w, h := 1250, 880 // 10 px/mm
	img := headerPage(w, h, 40)
	y, ok := HeaderTop(img)
	if !ok || y < 38 || y > 42 {
		t.Fatalf("HeaderTop = %d,%v, atteso circa 40", y, ok)
	}
	out := TrimAboveHeader(img, 2) // 2 mm = 20 px sopra il testo
	if got := out.Bounds().Dy(); got < h-22 || got > h-18 {
		t.Errorf("altezza dopo il taglio %d, attesa circa %d", got, h-20)
	}
	if same := TrimAboveHeader(img, -1); same.Bounds().Dy() != h {
		t.Error("margine negativo: la pagina non va tagliata")
	}
	if near := TrimAboveHeader(headerPage(w, h, 10), 2); near.Bounds().Dy() != h {
		t.Error("testo gia' vicino al bordo: nessun taglio")
	}
	if _, ok := HeaderTop(headerPage(w, h, 400)); ok {
		t.Error("testo troppo in basso: non e' il titolo")
	}
}
