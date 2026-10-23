package twain

import (
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"testing"
)

func errorsIs(err, target error) bool { return errors.Is(err, target) }

// dibHeader costruisce un BITMAPINFOHEADER da 40 byte.
func dibHeader(w, h, bpp, used int) []byte {
	b := make([]byte, 40)
	le := binary.LittleEndian
	le.PutUint32(b[0:], 40)
	le.PutUint32(b[4:], uint32(int32(w)))
	le.PutUint32(b[8:], uint32(int32(h)))
	le.PutUint16(b[12:], 1)
	le.PutUint16(b[14:], uint16(bpp))
	le.PutUint32(b[32:], uint32(used))
	return b
}

func TestDecodeDIB24BottomUp(t *testing.T) {
	// 2x2: righe dal basso. Riga 0 (in basso): blu, verde. Riga 1 (in alto): rosso, bianco. BGR con padding a 4 byte.
	pix := []byte{
		255, 0, 0, 0, 255, 0, 0, 0, // basso: blu, verde, +2 padding
		0, 0, 255, 255, 255, 255, 0, 0, // alto: rosso, bianco, +2 padding
	}
	img, err := decodeDIB(append(dibHeader(2, 2, 24, 0), pix...))
	if err != nil {
		t.Fatal(err)
	}
	want := map[[2]int]color.RGBA{{0, 0}: {255, 0, 0, 255}, {1, 0}: {255, 255, 255, 255}, {0, 1}: {0, 0, 255, 255}, {1, 1}: {0, 255, 0, 255}}
	for p, c := range want {
		if got := color.RGBAModel.Convert(img.At(p[0], p[1])); got != color.Color(c) {
			t.Errorf("pixel %v = %v, atteso %v", p, got, c)
		}
	}
}

func TestDecodeDIB8GrayAndTopDown(t *testing.T) {
	pal := make([]byte, 4*256)
	for i := 0; i < 256; i++ {
		pal[4*i], pal[4*i+1], pal[4*i+2] = byte(i), byte(i), byte(i)
	}
	// altezza negativa = righe dall'alto; 3x2, stride 4
	pix := []byte{10, 20, 30, 0, 40, 50, 60, 0}
	data := append(append(dibHeader(3, -2, 8, 0), pal...), pix...)
	img, err := decodeDIB(data)
	if err != nil {
		t.Fatal(err)
	}
	g, ok := img.(*image.Gray)
	if !ok {
		t.Fatalf("attesa image.Gray, avuta %T", img)
	}
	if g.GrayAt(0, 0).Y != 10 || g.GrayAt(2, 0).Y != 30 || g.GrayAt(0, 1).Y != 40 || g.GrayAt(2, 1).Y != 60 {
		t.Errorf("pixel grigi: %v", g.Pix)
	}
}

func TestDecodeDIB1Bit(t *testing.T) {
	// tavolozza 0=nero 1=bianco, 8x1: 10101010
	pal := []byte{0, 0, 0, 0, 255, 255, 255, 0}
	data := append(append(dibHeader(8, 1, 1, 2), pal...), 0xAA, 0, 0, 0)
	img, err := decodeDIB(data)
	if err != nil {
		t.Fatal(err)
	}
	g := img.(*image.Gray)
	for x := 0; x < 8; x++ {
		want := uint8(0)
		if x%2 == 0 {
			want = 255
		}
		if g.GrayAt(x, 0).Y != want {
			t.Errorf("x=%d: %d, atteso %d", x, g.GrayAt(x, 0).Y, want)
		}
	}
}

func TestDecodeDIBErrors(t *testing.T) {
	if _, err := decodeDIB(nil); err == nil {
		t.Error("vuoto accettato")
	}
	if _, err := decodeDIB(dibHeader(2, 2, 24, 0)); err == nil {
		t.Error("pixel mancanti accettati")
	}
	h := dibHeader(2, 2, 24, 0)
	binary.LittleEndian.PutUint32(h[16:], 1) // compressione
	if _, err := decodeDIB(append(h, make([]byte, 16)...)); err == nil {
		t.Error("compressione accettata")
	}
	if _, err := decodeDIB(append(dibHeader(100000, 2, 24, 0), make([]byte, 16)...)); err == nil {
		t.Error("dimensioni enormi accettate")
	}
	if _, err := decodeDIB(append(dibHeader(2, 2, 16, 0), make([]byte, 16)...)); err == nil {
		t.Error("16 bit accettati")
	}
}
