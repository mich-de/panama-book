package imgproc

import (
	"image"
	"image/color"
	"testing"
)

func TestTrimWhite(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if x >= 20 && x < 370 && y >= 5 && y < 270 {
				c = color.RGBA{150, 170, 150, 255}
			}
			img.Set(x, y, c)
		}
	}
	out := TrimWhite(img).Bounds()
	if out.Dx() < 348 || out.Dx() > 352 || out.Dy() < 263 || out.Dy() > 267 {
		t.Errorf("dimensioni %dx%d, attese circa 350x265", out.Dx(), out.Dy())
	}
	grey := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for i := range grey.Pix {
		grey.Pix[i] = 200
	}
	if TrimWhite(grey).Bounds() != grey.Bounds() {
		t.Error("sfondo grigio: nessun taglio")
	}
}
