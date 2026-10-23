package wia

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	ole "github.com/go-ole/go-ole"
)

// Questi test NON toccano lo scanner: solo funzioni pure.

func TestNum(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int
	}{
		{int32(5), 5}, {int64(-3), -3}, {int(7), 7}, {int16(2), 2}, {uint32(9), 9},
		{uint8(4), 4}, {uint16(600), 600}, {float64(3.9), 3}, {nil, 0}, {"12", 0}, {true, 0},
	}
	for _, c := range cases {
		if got := num(c.in); got != c.want {
			t.Errorf("num(%#v)=%d, atteso %d", c.in, got, c.want)
		}
	}
	if isNum(nil) || isNum("1") || !isNum(int32(1)) {
		t.Error("isNum non riconosce correttamente i tipi")
	}
}

func TestStr(t *testing.T) {
	if str(nil) != "" || str(int32(5)) != "5" || str("x") != "x" {
		t.Error("str errata")
	}
}

func TestDescribeHRESULT(t *testing.T) {
	cases := map[uint32]string{
		0x80210003: "carta assente",
		0x80210002: "inceppamento",
		0x80210016: "coperchio",
		0x80210006: "occupato",
		0x8021000A: "USB",
	}
	for code, frag := range cases {
		if m := describeHRESULT(code); !strings.Contains(m, frag) {
			t.Errorf("0x%08X: %q non contiene %q", code, m, frag)
		}
	}
	// ogni codice storico 0x01..0x0F deve avere un messaggio specifico
	for c := uint32(0x80210001); c <= 0x8021000F; c++ {
		if _, ok := hresultMessages[c]; !ok {
			t.Errorf("manca il messaggio per 0x%08X", c)
		}
	}
	if m := describeHRESULT(0x80210021); !strings.Contains(m, "non classificato") {
		t.Errorf("codice WIA ignoto: %q", m)
	}
	if m := describeHRESULT(0x80001234); m != "errore COM" {
		t.Errorf("codice non WIA: %q", m)
	}
}

func TestExplainAndHelpers(t *testing.T) {
	if explain("x", nil) != nil {
		t.Fatal("explain(nil) deve dare nil")
	}
	empty := ole.NewError(0x80210003)
	err := explain("trasferimento", empty)
	if !strings.Contains(err.Error(), "trasferimento") || !strings.Contains(err.Error(), "carta assente") ||
		!strings.Contains(err.Error(), "0x80210003") {
		t.Errorf("messaggio inatteso: %v", err)
	}
	if !errors.Is(err, error(empty)) {
		t.Error("l'errore originale deve restare raggiungibile")
	}
	if !IsPaperEmpty(err) || IsPaperJam(err) {
		t.Error("IsPaperEmpty/IsPaperJam errate")
	}
	if !IsPaperJam(explain("x", ole.NewError(0x80210002))) {
		t.Error("IsPaperJam non riconosce 0x80210002")
	}
	if !isRetryable(ole.NewError(0x80210006)) || !isRetryable(ole.NewError(0x80210007)) || isRetryable(ole.NewError(0x80210003)) {
		t.Error("isRetryable errata")
	}
	// errore non COM: solo contesto
	plain := explain("ctx", errors.New("boom"))
	if plain.Error() != "ctx: boom" {
		t.Errorf("errore non COM: %v", plain)
	}
	if _, ok := hresultOf(errors.New("boom")); ok {
		t.Error("hresultOf su errore non COM")
	}
	// DISP_E_EXCEPTION senza scode: ricade sul codice del chiamante
	wrapped := fmt.Errorf("w: %w", ole.NewErrorWithSubError(0x80020009, "d", ole.EXCEPINFO{}))
	if c, ok := hresultOf(wrapped); !ok || c != 0x80020009 {
		t.Errorf("DISP_E_EXCEPTION senza scode: 0x%X %v", c, ok)
	}
}

func TestOptionsNormalized(t *testing.T) {
	if _, err := (Options{}).normalized(); err == nil {
		t.Error("OutPath vuoto deve essere rifiutato")
	}
	o, err := Options{OutPath: "a.png"}.normalized()
	if err != nil {
		t.Fatal(err)
	}
	if o.DPI != 300 || o.DeviceMatch != "fi-800R" || o.Timeout != defaultTimeout {
		t.Errorf("default errati: %+v", o)
	}
	for _, dpi := range []int{-1, 10, 5000} {
		if _, err := (Options{OutPath: "a.png", DPI: dpi}).normalized(); err == nil {
			t.Errorf("dpi %d dovrebbe essere rifiutata", dpi)
		}
	}
	if _, err := (Options{OutPath: "a.png", Timeout: -time.Second}).normalized(); err == nil {
		t.Error("timeout negativo accettato")
	}
	o, err = Options{OutPath: "a.png", DPI: 600, DeviceMatch: "x", Timeout: time.Second, Gray: true}.normalized()
	if err != nil || o.DPI != 600 || o.DeviceMatch != "x" || o.Timeout != time.Second || !o.Gray {
		t.Errorf("valori espliciti alterati: %+v %v", o, err)
	}
}

func TestCheckDPI(t *testing.T) {
	if checkDPI(300, 75, 600) != nil || checkDPI(75, 75, 600) != nil || checkDPI(600, 75, 600) != nil {
		t.Error("dpi nell'intervallo rifiutata")
	}
	if checkDPI(74, 75, 600) == nil || checkDPI(601, 75, 600) == nil {
		t.Error("dpi fuori intervallo accettata")
	}
	if checkDPI(9999, 0, 0) != nil {
		t.Error("intervallo sconosciuto: non si deve rifiutare")
	}
}

func TestCheckSimplex(t *testing.T) {
	if err := checkSimplex(1, true); err != nil {
		t.Errorf("1 (solo alimentatore) deve passare: %v", err)
	}
	for _, sel := range []int{5, 4, 7} {
		if checkSimplex(sel, true) == nil {
			t.Errorf("selezione %d (duplex) deve essere rifiutata", sel)
		}
	}
	if checkSimplex(2, true) == nil {
		t.Error("senza il bit alimentatore deve essere rifiutato")
	}
	if checkSimplex(1, false) == nil {
		t.Error("valore non leggibile deve essere rifiutato")
	}
}

func TestRunWithTimeout(t *testing.T) {
	if err := runWithTimeout(time.Second, func() error { return nil }); err != nil {
		t.Errorf("successo: %v", err)
	}
	want := errors.New("x")
	if err := runWithTimeout(time.Second, func() error { return want }); err != want {
		t.Errorf("errore non propagato: %v", err)
	}
	if err := runWithTimeout(0, func() error { return want }); err != want {
		t.Errorf("senza timeout: %v", err)
	}
	block := make(chan struct{})
	defer close(block)
	err := runWithTimeout(30*time.Millisecond, func() error { <-block; return nil })
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("atteso ErrTimeout, ottenuto %v", err)
	}
	err = runWithTimeout(time.Second, func() error { panic("boom") })
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("panic non convertito: %v", err)
	}
}

// bmp24 costruisce un BMP 24 bit sintetico (bottom-up) 2x2: righe dal basso verso l'alto.
func bmp24(w, h int, px func(x, y int) color.RGBA) []byte {
	stride := (w*24 + 31) / 32 * 4
	data := make([]byte, 54+stride*h)
	copy(data, "BM")
	binary.LittleEndian.PutUint32(data[10:], 54)
	binary.LittleEndian.PutUint32(data[14:], 40)
	binary.LittleEndian.PutUint32(data[18:], uint32(int32(w)))
	binary.LittleEndian.PutUint32(data[22:], uint32(int32(h)))
	binary.LittleEndian.PutUint16(data[26:], 1)
	binary.LittleEndian.PutUint16(data[28:], 24)
	for y := 0; y < h; y++ { // y immagine, dall'alto
		row := data[54+(h-1-y)*stride:]
		for x := 0; x < w; x++ {
			c := px(x, y)
			row[3*x], row[3*x+1], row[3*x+2] = c.B, c.G, c.R
		}
	}
	return data
}

func TestDecodeBMP24(t *testing.T) {
	px := func(x, y int) color.RGBA { return color.RGBA{uint8(10 * x), uint8(20 * y), 77, 255} }
	img, err := decodeBMP(bmp24(3, 2, px)) // 3 px di larghezza: richiede padding di riga
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 3 || b.Dy() != 2 {
		t.Fatalf("dimensioni %v", b)
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			if got, want := color.RGBAModel.Convert(img.At(x, y)), color.Color(px(x, y)); got != want {
				t.Errorf("(%d,%d)=%v atteso %v", x, y, got, want)
			}
		}
	}
}

func TestDecodeBMP8Palette(t *testing.T) {
	w, h := 2, 1
	data := make([]byte, 54+1024+4)
	copy(data, "BM")
	binary.LittleEndian.PutUint32(data[10:], 54+1024)
	binary.LittleEndian.PutUint32(data[14:], 40)
	binary.LittleEndian.PutUint32(data[18:], uint32(w))
	binary.LittleEndian.PutUint32(data[22:], uint32(h))
	binary.LittleEndian.PutUint16(data[28:], 8)
	for i := 0; i < 256; i++ {
		p := 54 + 4*i
		data[p], data[p+1], data[p+2] = byte(i), byte(i), byte(i)
	}
	data[54+1024], data[54+1024+1] = 0, 200
	img, err := decodeBMP(data)
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := img.At(1, 0).RGBA(); r>>8 != 200 {
		t.Errorf("pixel 1 = %d, atteso 200", r>>8)
	}
}

func TestDecodeBMPErrors(t *testing.T) {
	good := bmp24(2, 2, func(x, y int) color.RGBA { return color.RGBA{A: 255} })
	if _, err := decodeBMP(good[:40]); err == nil {
		t.Error("BMP troncato accettato")
	}
	if _, err := decodeBMP(good[:len(good)-4]); err == nil {
		t.Error("dati pixel incompleti accettati")
	}
	bad := append([]byte(nil), good...)
	bad[0] = 'X'
	if _, err := decodeBMP(bad); err == nil {
		t.Error("firma errata accettata")
	}
	bad = append([]byte(nil), good...)
	binary.LittleEndian.PutUint32(bad[30:], 1) // RLE8
	if _, err := decodeBMP(bad); err == nil {
		t.Error("compressione non supportata accettata")
	}
}
