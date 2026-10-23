package imgproc

import (
	"image"
	"image/draw"
)

const (
	// borderLevel: una riga o colonna con luminosita' media (nella fascia centrale) da qui in su e' bordo chiaro:
	// sfondo bianco del driver TWAIN (255) o cornice trasparente del libretto (240-250). La pagina sta sotto.
	borderLevel = 236
	// borderRun: quante righe o colonne consecutive devono stare sotto borderLevel perche' si dica "qui comincia
	// il documento". Salta le righe scure sottili e isolate dell'orlo.
	borderRun = 8
	// borderMaxFrac: non si toglie mai piu' di questa frazione per lato.
	borderMaxFrac = 0.2
)

// TrimWhite toglie dai quattro lati il bordo bianco o molto chiaro attorno al documento. Misura la luminosita'
// media di ogni riga e colonna sulla fascia centrale (60%) e taglia dove comincia il documento. Su un fondo grigio
// (WIA), o se un lato non ha bordo chiaro, quel lato resta com'e'.
func TrimWhite(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 64 || h < 64 {
		return img
	}
	lumAt := func(x, y int) float64 {
		r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
		return float64(lum(uint8(r>>8), uint8(g>>8), uint8(bl>>8)))
	}
	colMean := make([]float64, w)
	y0, y1 := h*20/100, h*80/100
	for x := 0; x < w; x++ {
		var s float64
		for y := y0; y < y1; y++ {
			s += lumAt(x, y)
		}
		colMean[x] = s / float64(y1-y0)
	}
	rowMean := make([]float64, h)
	x0, x1 := w*20/100, w*80/100
	for y := 0; y < h; y++ {
		var s float64
		for x := x0; x < x1; x++ {
			s += lumAt(x, y)
		}
		rowMean[y] = s / float64(x1-x0)
	}
	left, right := edgeOffset(colMean, false), edgeOffset(colMean, true)
	top, bottom := edgeOffset(rowMean, false), edgeOffset(rowMean, true)
	if left+right+top+bottom == 0 {
		return img
	}
	r := image.Rect(left, top, w-right, h-bottom)
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, image.Pt(b.Min.X+r.Min.X, b.Min.Y+r.Min.Y), draw.Src)
	return out
}

// edgeOffset restituisce quante righe (o colonne) di bordo chiaro ci sono a partire da un'estremita' del profilo:
// la prima posizione dopo cui seguono borderRun valori sotto borderLevel. 0 se l'estremita' non e' chiara.
func edgeOffset(mean []float64, fromEnd bool) int {
	n := len(mean)
	at := func(i int) float64 {
		if fromEnd {
			return mean[n-1-i]
		}
		return mean[i]
	}
	max := int(float64(n) * borderMaxFrac)
	if at(0) < borderLevel && at(1) < borderLevel && at(2) < borderLevel {
		return 0
	}
	run := 0
	for i := 0; i < max+borderRun; i++ {
		if at(i) < borderLevel {
			run++
			if run == borderRun {
				off := i - borderRun + 1
				if off > max {
					return max
				}
				return off
			}
		} else {
			run = 0
		}
	}
	return max
}
