package pdfout

import (
	"bytes"
	"image"
	"image/color"
	"math/rand"
	"testing"
)

// immagine sintetica con rumore: pesa abbastanza da costringere la ricerca della qualita'.
func noisy(w, h int) *image.RGBA {
	r := rand.New(rand.NewSource(1))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(150 + r.Intn(60))
			if (x/40+y/40)%2 == 0 {
				v = 60 + uint8(r.Intn(40))
			}
			img.Set(x, y, color.RGBA{v, v, uint8(int(v) * 9 / 10), 255})
		}
	}
	return img
}

func TestEncodeRispettaIlLimite(t *testing.T) {
	img := noisy(1200, 800)
	big, info0, err := Encode(img, 300, 0)
	if err != nil {
		t.Fatal(err)
	}
	limit := len(big) / 2
	out, info, err := Encode(img, 300, limit)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > limit {
		t.Errorf("PDF %d byte oltre il limite %d", len(out), limit)
	}
	if info.Quality > info0.Quality || info.Bytes != len(out) {
		t.Errorf("info incoerente: %+v contro %+v", info, info0)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-1.4")) || !bytes.HasSuffix(bytes.TrimSpace(out), []byte("%%EOF")) {
		t.Error("PDF malformato")
	}
}

func TestEncodeQualitaAltaSeCiStaTutto(t *testing.T) {
	img := noisy(300, 200)
	_, info, err := Encode(img, 300, 50_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if info.Quality != maxQuality || info.Scale != 1 {
		t.Errorf("con limite largo servono qualita' %d e scala 1, avuto %+v", maxQuality, info)
	}
}

func TestEncodeLimiteImpossibile(t *testing.T) {
	if _, _, err := Encode(noisy(800, 600), 300, 500); err == nil {
		t.Error("atteso errore con limite di 500 byte")
	}
	if _, _, err := Encode(noisy(10, 10), 0, 1000); err == nil {
		t.Error("atteso errore con dpi 0")
	}
}

func TestMisuraPagina(t *testing.T) {
	pdf, _, err := Encode(noisy(300, 150), 300, 0)
	if err != nil {
		t.Fatal(err)
	}
	// 300 px a 300 dpi = 1 pollice = 72 pt; 150 px = 36 pt
	if !bytes.Contains(pdf, []byte("/MediaBox [0 0 72.00 36.00]")) {
		t.Errorf("MediaBox attesa 72x36 pt")
	}
}
