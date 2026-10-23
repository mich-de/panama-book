package twain

import (
	"math"
	"testing"
)

func TestUnitsToMMAndScale(t *testing.T) {
	// il campo fisico e' dato nell'unita' corrente: 8,5 pollici = 21,59 cm = 2550 px a 300 dpi = 215,9 mm
	for _, tc := range []struct {
		v     float64
		unit  uint16
		dpi   float64
		wantW float64
	}{{8.5, unitInch, 300, 215.9}, {21.59, unitCm, 300, 215.9}, {2550, unitPixels, 300, 215.9}, {5100, unitPixels, 600, 215.9}} {
		if got := unitsToMM(tc.v, unitScale(tc.unit, tc.dpi)); math.Abs(got-tc.wantW) > 0.01 {
			t.Errorf("unita' %d: %.3f mm, atteso %.1f", tc.unit, got, tc.wantW)
		}
	}
	if frameTolerance(unitInch) >= frameTolerance(unitPixels) {
		t.Error("tolleranze in ordine inatteso")
	}
}

func TestRecipesOrderAndCustomSizes(t *testing.T) {
	r := recipes([]uint16{32769, 32770})
	if r[0].frameCap != icapFrames || len(r[0].pre) != 0 || r[0].unit != unitInch {
		t.Errorf("la prima ricetta deve essere quella senza modifiche al formato: %+v", r[0])
	}
	n := 0
	for _, x := range r {
		if x.layoutSet {
			n++
		}
	}
	if n != 2 || len(r) < 18 {
		t.Errorf("ricette: %d, con DG_IMAGE %d", len(r), n)
	}
	// le ricette con DAT_IMAGELAYOUT (DG_IMAGE) non sono mai usate da Scan
	for _, x := range r {
		if x.layoutSet && x.name == "" {
			t.Error("ricetta senza nome")
		}
	}
}
