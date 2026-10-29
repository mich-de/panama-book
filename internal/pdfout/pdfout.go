// Package pdfout scrive un PDF di una sola pagina con l'immagine scansionata, entro un peso massimo.
// L'immagine e' incorporata come JPEG (DCTDecode) senza altre ricodifiche: il PDF pesa quanto il JPEG piu' circa 1 KB.
package pdfout

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"

	"scanlibretto/internal/imgproc"
)

// Info descrive il risultato: qualita' JPEG scelta, riduzione applicata (1 = nessuna), misure in pixel e peso del PDF.
type Info struct {
	Quality int
	Scale   float64
	W, H    int
	Bytes   int
}

// Qualita' JPEG provate: la piu' alta che sta nel limite. Sotto minQuality il testo piccolo degrada: prima si riduce l'immagine.
const (
	maxQuality = 95
	minQuality = 80
	scaleStep  = 0.93
	minScale   = 0.5
)

// Encode produce il PDF con la qualita' JPEG piu' alta possibile che lo tiene entro maxBytes (0 = nessun limite,
// qualita' 92). dpi e' la risoluzione dell'immagine e decide la misura fisica della pagina. Se anche a minQuality
// non basta, riduce l'immagine a passi del 7% (mai sotto la meta') e riprova.
func Encode(img image.Image, dpi float64, maxBytes int) ([]byte, Info, error) {
	if dpi <= 0 {
		return nil, Info{}, errors.New("dpi non valido")
	}
	rgb := toRGBA(img)
	if maxBytes <= 0 {
		return build(rgb, dpi, 92, 1)
	}
	for scale := 1.0; scale >= minScale-1e-9; scale *= scaleStep {
		cur := image.Image(rgb)
		if scale < 1 {
			cur = toRGBA(imgproc.Scale(rgb, scale))
		}
		// ricerca binaria sulla qualita': il peso cresce con la qualita'
		lo, hi := minQuality, maxQuality
		var best []byte
		var bestInfo Info
		for lo <= hi {
			mid := (lo + hi) / 2
			out, info, err := build(cur, dpi/scale, mid, scale)
			if err != nil {
				return nil, Info{}, err
			}
			if len(out) <= maxBytes {
				best, bestInfo = out, info
				lo = mid + 1
			} else {
				hi = mid - 1
			}
		}
		if best != nil {
			return best, bestInfo, nil
		}
	}
	return nil, Info{}, fmt.Errorf("impossibile stare entro %d byte senza ridurre l'immagine a meno della meta'", maxBytes)
}

func toRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

// build codifica il JPEG e lo incorpora nel PDF. effDPI e' la risoluzione effettiva dell'immagine ridotta.
func build(img image.Image, effDPI float64, quality int, scale float64) ([]byte, Info, error) {
	var jb bytes.Buffer
	if err := jpeg.Encode(&jb, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, Info{}, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	pdf := wrap(jb.Bytes(), w, h, float64(w)/effDPI*72, float64(h)/effDPI*72)
	return pdf, Info{Quality: quality, Scale: scale, W: w, H: h, Bytes: len(pdf)}, nil
}

// wrap scrive un PDF 1.4 minimo: catalogo, pagina, immagine JPEG e il contenuto che la disegna a pagina intera.
func wrap(jpg []byte, w, h int, pw, ph float64) []byte {
	var out bytes.Buffer
	var offs [6]int
	obj := func(n int, body string) {
		offs[n] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", n, body)
	}
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	obj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	obj(3, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources << /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>", pw, ph))
	offs[4] = out.Len()
	fmt.Fprintf(&out, "4 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", w, h, len(jpg))
	out.Write(jpg)
	out.WriteString("\nendstream\nendobj\n")
	content := fmt.Sprintf("q %.2f 0 0 %.2f 0 0 cm /Im0 Do Q", pw, ph)
	obj(5, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	xref := out.Len()
	out.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", offs[i])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return out.Bytes()
}
