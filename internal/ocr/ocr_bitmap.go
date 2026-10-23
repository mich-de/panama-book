package ocr

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strconv"
)

// Lettura dell'immagine e conversione nei pixel che il motore OCR riceve. Il percorso PowerShell passa da BitmapDecoder,
// che per ogni PNG/JPEG restituisce un SoftwareBitmap Bgra8 con alfa "Straight" (verificato su grigio 8 bit, RGB e RGBA):
// qui si producono gli stessi byte partendo dai pixel decodificati da image/png.

// bitmapSize legge solo le dimensioni, per il controllo di MaxImageDimension prima di decodificare.
func bitmapSize(path string) (w, h int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, fmt.Errorf("%s: %w", path, err)
	}
	return cfg.Width, cfg.Height, nil
}

// bitmapLoad decodifica PNG, JPEG o GIF (altri formati: errore, e il chiamante ripiega su PowerShell).
func bitmapLoad(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}

// fillBGRA scrive in dst (lunga Dx*Dy*4) i pixel di img come Bgra8 con alfa non premoltiplicato (Straight).
func fillBGRA(dst []byte, img image.Image) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	switch m := img.(type) {
	case *image.Gray:
		for y := 0; y < h; y++ {
			src := m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y):]
			d := dst[y*w*4 : (y+1)*w*4]
			for x := 0; x < w; x++ {
				v := src[x]
				d[4*x], d[4*x+1], d[4*x+2], d[4*x+3] = v, v, v, 255
			}
		}
		return
	case *image.NRGBA:
		for y := 0; y < h; y++ {
			src := m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y):]
			d := dst[y*w*4 : (y+1)*w*4]
			for x := 0; x < w; x++ {
				d[4*x], d[4*x+1], d[4*x+2], d[4*x+3] = src[4*x+2], src[4*x+1], src[4*x], src[4*x+3]
			}
		}
		return
	case *image.RGBA:
		for y := 0; y < h; y++ {
			src := m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y):]
			d := dst[y*w*4 : (y+1)*w*4]
			for x := 0; x < w; x++ {
				r, g, bl, a := src[4*x], src[4*x+1], src[4*x+2], src[4*x+3]
				if a != 255 { // alfa premoltiplicato -> Straight, come color.NRGBAModel
					c := color.NRGBAModel.Convert(color.RGBA{r, g, bl, a}).(color.NRGBA)
					r, g, bl = c.R, c.G, c.B
				}
				d[4*x], d[4*x+1], d[4*x+2], d[4*x+3] = bl, g, r, a
			}
		}
		return
	}
	// altri tipi (palette, 16 bit, YCbCr): conversione generica pixel per pixel
	for y := 0; y < h; y++ {
		d := dst[y*w*4 : (y+1)*w*4]
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			d[4*x], d[4*x+1], d[4*x+2], d[4*x+3] = c.B, c.G, c.R, c.A
		}
	}
}

// floatShortest riporta un float32 del motore al float64 "corto" (12.3 e non 12.300000190734863): e' il valore che
// scriveva il percorso PowerShell nel JSON.
func floatShortest(f float32) float64 {
	v, err := strconv.ParseFloat(strconv.FormatFloat(float64(f), 'g', -1, 32), 64)
	if err != nil {
		return float64(f)
	}
	return v
}
