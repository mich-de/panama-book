package imgproc

import (
	"image"
	"image/draw"
)

// maxHeaderMM e' la distanza massima dal bordo alto della pagina ritagliata entro cui si cerca il titolo
// "REPUBLICA DE PANAMA" (nelle pagine reali sta tra 2 e 6 mm).
const maxHeaderMM = 12.0

// HeaderTop restituisce la riga (px) in cui comincia il testo blu "REPUBLICA DE PANAMA" in una pagina dati
// gia' ritagliata e dritta (125 mm di larghezza). Guarda solo la fascia centrale, dove stanno le lettere:
// ai lati ci sono le righe ondulate del bordo. ok=false se non trova il testo.
func HeaderTop(page image.Image) (int, bool) {
	b := page.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 100 || h < 100 {
		return 0, false
	}
	x0, x1 := b.Min.X+w*30/100, b.Min.X+w*70/100
	limit := int(float64(w) / td3PageW * maxHeaderMM)
	if limit > h/3 {
		limit = h / 3
	}
	thr := (x1 - x0) * 4 / 100
	run := 0
	for y := 0; y < limit; y++ {
		n := 0
		for x := x0; x < x1; x++ {
			r, g, bl, _ := page.At(x, b.Min.Y+y).RGBA()
			_ = g
			r8, b8 := int(r>>8), int(bl>>8)
			if b8-r8 > 40 && r8 < 130 {
				n++
			}
		}
		if n > thr {
			run++
			if run == 3 {
				return y - 2, true
			}
		} else {
			run = 0
		}
	}
	return 0, false
}

// TrimAboveHeader toglie dalla pagina dati la parte alta, lasciando marginMM sopra il testo del titolo.
// Se il titolo non si trova, o e' gia' piu' vicino al bordo del margine, restituisce la pagina com'e'.
func TrimAboveHeader(page image.Image, marginMM float64) image.Image {
	if marginMM < 0 {
		return page
	}
	y, ok := HeaderTop(page)
	if !ok {
		return page
	}
	b := page.Bounds()
	cut := y - int(marginMM*float64(b.Dx())/td3PageW+0.5)
	if cut <= 0 {
		return page
	}
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()-cut))
	draw.Draw(out, out.Bounds(), page, image.Pt(b.Min.X, b.Min.Y+cut), draw.Src)
	return out
}
