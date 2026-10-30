package pdftext

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"regexp"
	"strings"
)

var (
	reWidth  = regexp.MustCompile(`/Width\s+(\d+)`)
	reHeight = regexp.MustCompile(`/Height\s+(\d+)`)
	reBPC    = regexp.MustCompile(`/BitsPerComponent\s+(\d+)`)
)

const (
	minSquare = 60
	maxSquare = 2000
)

// squareImage dice se l'oggetto e' un'immagine quasi quadrata di dimensione plausibile per un codice QR.
func squareImage(dict string) bool {
	w, h := atoi(reWidth, dict), atoi(reHeight, dict)
	if w < minSquare || h < minSquare || w > maxSquare || h > maxSquare {
		return false
	}
	return w*10 >= h*9 && h*10 >= w*9
}

// Images restituisce le immagini quasi quadrate del PDF (JPEG o Flate a 8 bit, grigi o RGB): tra queste c'e' il codice QR dei
// certificati. Il PDF e' trattato come non fidato: solo dimensioni piccole e formati semplici.
func Images(data []byte) ([]image.Image, error) {
	if len(data) > maxFile || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, ErrUnsupported
	}
	objs := parseObjects(data)
	if err := open(data, objs); err != nil {
		return nil, err
	}
	var out []image.Image
	for _, o := range objs {
		if o.stream == nil || !strings.Contains(o.dict, "/Image") || !squareImage(o.dict) || len(out) >= 8 {
			continue
		}
		if strings.Contains(o.dict, "/DCTDecode") {
			if img, err := jpeg.Decode(bytes.NewReader(o.stream)); err == nil {
				out = append(out, img)
			}
			continue
		}
		w, h := atoi(reWidth, o.dict), atoi(reHeight, o.dict)
		if atoi(reBPC, o.dict) != 8 {
			continue
		}
		var n int
		switch {
		case strings.Contains(o.dict, "/DeviceRGB"):
			n = 3
		case strings.Contains(o.dict, "/DeviceGray"):
			n = 1
		default:
			continue
		}
		if len(o.stream) < w*h*n {
			continue
		}
		g := image.NewGray(image.Rect(0, 0, w, h))
		for i := 0; i < w*h; i++ {
			v := o.stream[i*n]
			if n == 3 {
				v = uint8((299*int(o.stream[i*3]) + 587*int(o.stream[i*3+1]) + 114*int(o.stream[i*3+2])) / 1000)
			}
			g.SetGray(i%w, i/w, color.Gray{Y: v})
		}
		out = append(out, g)
	}
	return out, nil
}
