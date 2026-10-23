package twain

import (
	"math"
	"testing"
	"time"
)

func TestFrameForAreaFull(t *testing.T) {
	f, err := frameForArea(0, 0, 0, defaultFieldWmm, defaultFieldHmm)
	if err != nil {
		t.Fatal(err)
	}
	if f.Left != 0 || f.Top != 0 || math.Abs(f.Right-8.5) > 1e-9 || math.Abs(f.Bottom-14) > 1e-9 {
		t.Errorf("area massima: %+v", f)
	}
}

func TestFrameForAreaStartAndHeight(t *testing.T) {
	// pagina dati: da 91 mm, 87 mm di lunghezza, tutta la larghezza
	f, err := frameForArea(91, 0, 87, 215.9, 355.6)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(f.Top*mmPerInch-91) > 1e-6 || math.Abs(f.Bottom*mmPerInch-178) > 1e-6 {
		t.Errorf("alto/basso in mm: %.3f %.3f", f.Top*mmPerInch, f.Bottom*mmPerInch)
	}
	if f.Left != 0 || math.Abs(f.Right*mmPerInch-215.9) > 1e-6 {
		t.Errorf("larghezza: %+v", f)
	}
	// HeightMM=0 con offset: fino al fondo del campo
	f, err = frameForArea(91, 0, 0, 215.9, 355.6)
	if err != nil || math.Abs(f.Bottom*mmPerInch-355.6) > 1e-6 || math.Abs(f.Top*mmPerInch-91) > 1e-6 {
		t.Errorf("fino al fondo: %+v %v", f, err)
	}
}

func TestFrameForAreaCentered(t *testing.T) {
	f, err := frameForArea(0, 100, 50, 215.9, 355.6)
	if err != nil {
		t.Fatal(err)
	}
	left, right := f.Left*mmPerInch, f.Right*mmPerInch
	if math.Abs((right-left)-100) > 1e-6 || math.Abs(left-(215.9-100)/2) > 1e-6 {
		t.Errorf("centratura: sinistra %.3f destra %.3f", left, right)
	}
	// simmetria rispetto al centro del campo
	if math.Abs((left+right)/2-215.9/2) > 1e-6 {
		t.Errorf("non centrata: centro %.3f", (left+right)/2)
	}
}

func TestFrameForAreaClampsRounding(t *testing.T) {
	// 216x356 mm superano di poco il campo reale 215,9x355,6: entro la tolleranza si limita al massimo
	f, err := frameForArea(0, 216, 356, 215.9, 355.6)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(f.Right*mmPerInch-215.9) > 1e-6 || math.Abs(f.Bottom*mmPerInch-355.6) > 1e-6 || f.Left < 0 {
		t.Errorf("limitazione: %+v", f)
	}
}

func TestFrameForAreaErrors(t *testing.T) {
	W, H := 215.9, 355.6
	bad := []struct {
		nome    string
		s, w, h float64
	}{
		{"negativo", -1, 0, 10}, {"larghezza negativa", 0, -5, 0}, {"altezza negativa", 0, 0, -0.1},
		{"NaN", math.NaN(), 0, 10}, {"infinito", 0, 0, math.Inf(1)},
		{"larghezza oltre il campo", 0, 220, 0}, {"altezza oltre il campo", 0, 0, 360},
		{"offset oltre il campo", 360, 0, 0}, {"somma oltre il campo", 200, 0, 157},
	}
	for _, c := range bad {
		if _, err := frameForArea(c.s, c.w, c.h, W, H); err == nil {
			t.Errorf("%s: errore atteso", c.nome)
		}
	}
	if _, err := frameForArea(0, 0, 0, 0, 100); err == nil {
		t.Error("campo di larghezza 0 accettato")
	}
}

func TestOptionsNormalized(t *testing.T) {
	o, err := Options{}.normalized()
	if err != nil || o.DPI != 300 || o.DeviceMatch != "fi-800R" || o.Timeout != 3*time.Minute {
		t.Errorf("predefiniti: %+v %v", o, err)
	}
	if _, err := (Options{DPI: 5}).normalized(); err == nil {
		t.Error("dpi 5 accettato")
	}
	if _, err := (Options{StartMM: -1}).normalized(); err == nil {
		t.Error("offset negativo accettato")
	}
	if _, err := (Options{StartMM: 300, HeightMM: 100}).normalized(); err == nil {
		t.Error("offset+altezza oltre il campo accettati")
	}
	if _, err := (Options{Timeout: -1}).normalized(); err == nil {
		t.Error("timeout negativo accettato")
	}
	if (Options{}).areaRequested() || !(Options{StartMM: 1}).areaRequested() {
		t.Error("areaRequested errato")
	}
}

func TestRunWithTimeout(t *testing.T) {
	if err := runWithTimeout(time.Second, func() error { return nil }); err != nil {
		t.Error(err)
	}
	err := runWithTimeout(20*time.Millisecond, func() error { time.Sleep(time.Second); return nil })
	if err == nil || !errorsIs(err, ErrTimeout) {
		t.Errorf("timeout atteso, avuto %v", err)
	}
	if err := runWithTimeout(time.Second, func() error { panic("x") }); err == nil {
		t.Error("panic non convertito in errore")
	}
}
