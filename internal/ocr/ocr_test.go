package ocr

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// font 5x7 minimo, solo i caratteri che servono alle immagini sintetiche dei test
var font5x7 = map[rune][7]string{
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'I': {"01110", "00100", "00100", "00100", "00100", "00100", "01110"},
	'L': {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N': {"10001", "11001", "10101", "10011", "10001", "10001", "10001"},
	'O': {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "11110", "00001", "00001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
}

// textImage disegna le righe di testo (nero su bianco, ogni punto del font diventa un quadrato scale x scale).
func textImage(lines []string, scale int) *image.Gray {
	margin := 6 * scale
	maxw := 0
	for _, l := range lines {
		if n := len([]rune(l)); n > maxw {
			maxw = n
		}
	}
	w := 2*margin + maxw*6*scale
	h := 2*margin + len(lines)*10*scale
	img := image.NewGray(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.Gray{255}), image.Point{}, draw.Src)
	for li, l := range lines {
		for ci, r := range []rune(l) {
			g, ok := font5x7[r]
			if !ok {
				continue
			}
			for y := 0; y < 7; y++ {
				for x := 0; x < 5; x++ {
					if g[y][x] != '1' {
						continue
					}
					x0, y0 := margin+(ci*6+x)*scale, margin+(li*10+y)*scale
					draw.Draw(img, image.Rect(x0, y0, x0+scale, y0+scale), image.NewUniform(color.Gray{0}), image.Point{}, draw.Src)
				}
			}
		}
	}
	return blur(img, scale/2)
}

// blur e' una sfocatura a scatola separabile (due passate): addolcisce i bordi a gradino del font a punti, che il motore
// legge molto peggio del testo con i bordi antialias.
func blur(src *image.Gray, r int) *image.Gray {
	if r < 1 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	cur := src
	for pass := 0; pass < 2; pass++ {
		for _, horiz := range []bool{true, false} {
			dst := image.NewGray(b)
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					sum, n := 0, 0
					for k := -r; k <= r; k++ {
						xx, yy := x, y
						if horiz {
							xx += k
						} else {
							yy += k
						}
						if xx < 0 || yy < 0 || xx >= w || yy >= h {
							continue
						}
						sum += int(cur.Pix[yy*cur.Stride+xx])
						n++
					}
					dst.Pix[y*dst.Stride+x] = uint8(sum / n)
				}
			}
			cur = dst
		}
	}
	return cur
}

func writePNG(t *testing.T, img image.Image) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.png")
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// BGRA straight come quello di BitmapDecoder (confrontato byte per byte con le uscite di PowerShell su grigio 8 bit,
// RGB e RGBA: vedi docs/agenti/16-ocr-nativo.md).
func TestFillBGRA(t *testing.T) {
	g := image.NewGray(image.Rect(0, 0, 2, 1))
	g.Pix[0], g.Pix[1] = 10, 200
	got := make([]byte, 8)
	fillBGRA(got, g)
	if want := []byte{10, 10, 10, 255, 200, 200, 200, 255}; !bytes.Equal(got, want) {
		t.Errorf("Gray: %v, atteso %v", got, want)
	}

	n := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	n.SetNRGBA(0, 0, color.NRGBA{R: 1, G: 2, B: 3, A: 100})
	got = make([]byte, 4)
	fillBGRA(got, n)
	if want := []byte{3, 2, 1, 100}; !bytes.Equal(got, want) {
		t.Errorf("NRGBA: %v, atteso %v", got, want)
	}

	r := image.NewRGBA(image.Rect(0, 0, 1, 1))
	r.SetRGBA(0, 0, color.RGBA{R: 5, G: 6, B: 7, A: 255})
	fillBGRA(got, r)
	if want := []byte{7, 6, 5, 255}; !bytes.Equal(got, want) {
		t.Errorf("RGBA opaco: %v, atteso %v", got, want)
	}
	r.SetRGBA(0, 0, color.RGBA{R: 50, G: 25, B: 0, A: 100}) // premoltiplicato: va riportato a Straight
	fillBGRA(got, r)
	c := color.NRGBAModel.Convert(color.RGBA{R: 50, G: 25, B: 0, A: 100}).(color.NRGBA)
	if want := []byte{c.B, c.G, c.R, 100}; !bytes.Equal(got, want) {
		t.Errorf("RGBA premoltiplicato: %v, atteso %v", got, want)
	}

	// sotto-immagine con origine diversa da zero, e percorso generico (palette)
	big := image.NewGray(image.Rect(0, 0, 4, 4))
	big.Pix[1*4+2] = 99
	sub := big.SubImage(image.Rect(2, 1, 4, 3)).(*image.Gray)
	got = make([]byte, 2*2*4)
	fillBGRA(got, sub)
	if got[0] != 99 || got[3] != 255 || got[4] != 0 {
		t.Errorf("SubImage: %v", got)
	}
	p := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.RGBA{R: 9, G: 8, B: 7, A: 255}})
	got = make([]byte, 4)
	fillBGRA(got, p)
	if want := []byte{7, 8, 9, 255}; !bytes.Equal(got, want) {
		t.Errorf("Paletted: %v, atteso %v", got, want)
	}
}

func TestFloatShortest(t *testing.T) {
	if v := floatShortest(float32(12.3)); v != 12.3 {
		t.Errorf("floatShortest(12.3) = %v", v)
	}
	if v := floatShortest(81); v != 81 {
		t.Errorf("floatShortest(81) = %v", v)
	}
}

func TestRequested(t *testing.T) {
	for in, want := range map[string]string{"": "", "native": "native", " PowerShell ": "powershell", "boh": ""} {
		t.Setenv(envBackend, in)
		if got := requested(); got != want {
			t.Errorf("LIBRETTO_OCR=%q: %q, atteso %q", in, got, want)
		}
	}
}

func TestBitmapSize(t *testing.T) {
	p := writePNG(t, image.NewGray(image.Rect(0, 0, 30, 20)))
	w, h, err := bitmapSize(p)
	if err != nil || w != 30 || h != 20 {
		t.Errorf("bitmapSize = %d,%d,%v", w, h, err)
	}
	if _, _, err := bitmapSize(filepath.Join(t.TempDir(), "manca.png")); err == nil {
		t.Error("file mancante: atteso errore")
	}
}

func TestRecognizeManyVuoto(t *testing.T) {
	rs, err := RecognizeMany(context.Background(), nil)
	if rs != nil || err != nil {
		t.Errorf("RecognizeMany(nil) = %v, %v", rs, err)
	}
}

// squash toglie gli spazi e rende maiuscolo, per confrontare testi senza dipendere da come il motore divide le parole.
func squash(s string) string { return strings.ToUpper(strings.Join(strings.Fields(s), "")) }
