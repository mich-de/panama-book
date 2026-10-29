package imgproc

// Raddrizzamento di un quadrilatero scelto a mano: i quattro angoli del libretto (anche storto o in prospettiva) diventano un
// rettangolo dritto.

import (
	"fmt"
	"image"
	"image/color"
	"math"
)

// WarpQuad ritaglia da img il quadrilatero q (angoli in pixel dell'immagine, in ordine: alto-sinistra, alto-destra,
// basso-destra, basso-sinistra) e lo restituisce come rettangolo dritto. La misura e' la media dei lati opposti. Fuori
// dall'immagine il fondo e' bianco. Errore se il quadrilatero e' troppo piccolo, storto o non convesso.
func WarpQuad(img image.Image, q [4][2]float64) (image.Image, error) {
	dist := func(a, b [2]float64) float64 { return math.Hypot(a[0]-b[0], a[1]-b[1]) }
	w := (dist(q[0], q[1]) + dist(q[3], q[2])) / 2
	h := (dist(q[0], q[3]) + dist(q[1], q[2])) / 2
	if w < 50 || h < 50 {
		return nil, fmt.Errorf("il taglio è troppo piccolo")
	}
	if w > 12000 || h > 12000 {
		return nil, fmt.Errorf("il taglio è troppo grande")
	}
	// convesso: i prodotti vettoriali di lati consecutivi hanno tutti lo stesso segno
	var pos, neg int
	for i := 0; i < 4; i++ {
		a, b, c := q[i], q[(i+1)%4], q[(i+2)%4]
		cr := (b[0]-a[0])*(c[1]-b[1]) - (b[1]-a[1])*(c[0]-b[0])
		if cr > 0 {
			pos++
		} else if cr < 0 {
			neg++
		}
	}
	if pos != 4 && neg != 4 {
		return nil, fmt.Errorf("i quattro angoli non formano un quadrilatero (incrocio dei lati)")
	}
	// omografia dal quadrato unitario al quadrilatero (Heckbert)
	x0, y0, x1, y1, x2, y2, x3, y3 := q[0][0], q[0][1], q[1][0], q[1][1], q[2][0], q[2][1], q[3][0], q[3][1]
	dx1, dx2, dx3 := x1-x2, x3-x2, x0-x1+x2-x3
	dy1, dy2, dy3 := y1-y2, y3-y2, y0-y1+y2-y3
	var a, b, c, d, e, f, g, hh float64
	if math.Abs(dx3) < 1e-9 && math.Abs(dy3) < 1e-9 {
		a, b, c, d, e, f = x1-x0, x3-x0, x0, y1-y0, y3-y0, y0
	} else {
		den := dx1*dy2 - dx2*dy1
		if math.Abs(den) < 1e-12 {
			return nil, fmt.Errorf("i quattro angoli sono su una riga")
		}
		g = (dx3*dy2 - dx2*dy3) / den
		hh = (dx1*dy3 - dx3*dy1) / den
		a, b, c = x1-x0+g*x1, x3-x0+hh*x3, x0
		d, e, f = y1-y0+g*y1, y3-y0+hh*y3, y0
	}
	W, H := int(math.Round(w)), int(math.Round(h))
	b0 := img.Bounds()
	rd := rgbReader(img)
	out := image.NewRGBA(image.Rect(0, 0, W, H))
	for y := 0; y < H; y++ {
		v := (float64(y) + 0.5) / float64(H)
		for x := 0; x < W; x++ {
			u := (float64(x) + 0.5) / float64(W)
			den := g*u + hh*v + 1
			sx := (a*u+b*v+c)/den - 0.5 + float64(b0.Min.X)
			sy := (d*u+e*v+f)/den - 0.5 + float64(b0.Min.Y)
			ix, iy := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-float64(ix), sy-float64(iy)
			var acc [3]float64
			for j := 0; j < 2; j++ {
				for i := 0; i < 2; i++ {
					wt := (1 - math.Abs(float64(i)-fx)) * (1 - math.Abs(float64(j)-fy))
					px, py := ix+i, iy+j
					if px < b0.Min.X || py < b0.Min.Y || px >= b0.Max.X || py >= b0.Max.Y {
						acc[0] += wt * 255
						acc[1] += wt * 255
						acc[2] += wt * 255
						continue
					}
					r, gg, bb := rd(px, py)
					acc[0] += wt * float64(r)
					acc[1] += wt * float64(gg)
					acc[2] += wt * float64(bb)
				}
			}
			out.SetRGBA(x, y, color.RGBA{uint8(acc[0] + 0.5), uint8(acc[1] + 0.5), uint8(acc[2] + 0.5), 255})
		}
	}
	return out, nil
}
