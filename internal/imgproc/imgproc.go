// Package imgproc contiene le poche elaborazioni di immagine usate prima dell'OCR (solo libreria standard).
package imgproc

import (
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
)

// Load legge un PNG o JPEG.
func Load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// SavePNG salva l'immagine in PNG.
func SavePNG(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func gray(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 257
}

// Rotate ruota in senso orario di 0, 90, 180 o 270 gradi.
func Rotate(src image.Image, deg int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	switch deg {
	case 90:
		dst := image.NewRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(h-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 180:
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(w-1-x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 270:
		dst := image.NewRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(y, w-1-x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	}
	return src
}

// CropContent ritaglia il rettangolo che contiene il documento, scartando lo sfondo bianco dello scanner.
// Una riga o colonna conta come "contenuto" se almeno l'1% dei suoi pixel non e' bianco (ignora i puntini).
func CropContent(src image.Image, margin int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	rows := make([]int, h)
	cols := make([]int, w)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if gray(src.At(b.Min.X+x, b.Min.Y+y)) < 225 {
				rows[y]++
				cols[x]++
			}
		}
	}
	thrR, thrC := int(0.01*float64(w)), int(0.01*float64(h))
	y0, y1, x0, x1 := 0, h-1, 0, w-1
	for y0 < h && rows[y0] <= thrR {
		y0++
	}
	for y1 > y0 && rows[y1] <= thrR {
		y1--
	}
	for x0 < w && cols[x0] <= thrC {
		x0++
	}
	for x1 > x0 && cols[x1] <= thrC {
		x1--
	}
	if x1 <= x0 || y1 <= y0 {
		return src
	}
	r := image.Rect(maxi(0, x0-margin), maxi(0, y0-margin), mini(w, x1+margin), mini(h, y1+margin)).Add(b.Min)
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			dst.Set(x, y, src.At(r.Min.X+x, r.Min.Y+y))
		}
	}
	return dst
}

// Scale ridimensiona con interpolazione bilineare.
func Scale(src image.Image, f float64) image.Image {
	if f == 1 {
		return src
	}
	b := src.Bounds()
	w, h := int(float64(b.Dx())*f), int(float64(b.Dy())*f)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := (float64(y)+0.5)/f - 0.5
		y0 := int(math.Floor(sy))
		fy := sy - float64(y0)
		for x := 0; x < w; x++ {
			sx := (float64(x)+0.5)/f - 0.5
			x0 := int(math.Floor(sx))
			fx := sx - float64(x0)
			var r, g, bl float64
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					wgt := (1 - math.Abs(float64(dx)-fx)) * (1 - math.Abs(float64(dy)-fy))
					px := b.Min.X + clampi(x0+dx, 0, b.Dx()-1)
					py := b.Min.Y + clampi(y0+dy, 0, b.Dy()-1)
					cr, cg, cb, _ := src.At(px, py).RGBA()
					r += wgt * float64(cr>>8)
					g += wgt * float64(cg>>8)
					bl += wgt * float64(cb>>8)
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r + 0.5), uint8(g + 0.5), uint8(bl + 0.5), 255})
		}
	}
	return dst
}

func mini(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampi(v, lo, hi int) int { return maxi(lo, mini(v, hi)) }
