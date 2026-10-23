package libreta

import (
	"context"
	"math"
	"testing"

	"scanlibretto/internal/ocr"
)

// vizNoOCR sostituisce la lettura OCR dei ritagli con una che non trova testo (i test non avviano PowerShell).
func vizNoOCR(t *testing.T) {
	t.Helper()
	old := vizOCRMany
	vizOCRMany = func(_ context.Context, paths []string) ([]*ocr.Result, error) {
		rs := make([]*ocr.Result, len(paths))
		for i := range rs {
			rs[i] = &ocr.Result{}
		}
		return rs, nil
	}
	t.Cleanup(func() { vizOCRMany = old })
}

// vizShifted colloca una parola nella fascia (campo, variante) del foglio spostata in verticale di off (in W).
func vizShifted(key string, variant int, text string, off float64) vizWordSpec {
	s := vizSheetWord(key, variant, text)
	s.y += off * vizVariants[variant].unit
	return s
}

func TestVIZCalibrateMeasuresColumnOffsets(t *testing.T) {
	// colonna sinistra piu' in basso di 0,02 W (due date e il livello), destra piu' in alto di 0,005 W (nascita)
	var specs []vizWordSpec
	for v := 0; v < 3; v++ {
		specs = append(specs, vizShifted("data_emissione", v, "09-03-2026", 0.02))
		specs = append(specs, vizShifted("data_scadenza", v, "04-03-2031", 0.021))
		specs = append(specs, vizShifted("livello", v, "SUPPORT", 0.019))
		specs = append(specs, vizShifted("data_nascita", v, "01-01-2000", -0.005))
	}
	slots := vizSheetSlots(vizWords(vizSheetResult(specs)))
	l, r, nl, nr := vizCalibrate(slots)
	if nl != 9 || nr != 3 {
		t.Errorf("campioni %d sinistra e %d destra, attesi 9 e 3", nl, nr)
	}
	if math.Abs(l-0.02) > 0.003 || math.Abs(r+0.005) > 0.003 {
		t.Errorf("scostamenti misurati %.4f e %.4f, attesi 0,020 e -0,005", l, r)
	}
}

func TestVIZCalibrateIgnoresNeighbourLines(t *testing.T) {
	// una data trovata a un campo di distanza (0,045 W) e' la riga del campo vicino: non e' una misura
	specs := []vizWordSpec{vizShifted("data_emissione", 0, "09-03-2026", 0.045), vizShifted("data_scadenza", 0, "04-03-2031", 0.002)}
	l, _, nl, _ := vizCalibrate(vizSheetSlots(vizWords(vizSheetResult(specs))))
	if nl != 1 || math.Abs(l-0.002) > 0.003 {
		t.Errorf("campioni %d, scostamento %.4f: la riga lontana va scartata", nl, l)
	}
	// senza righe riconoscibili nessuna calibrazione
	geo := vizApplyCalibration(vizGeom{w: 1000}, vizSheetSlots(vizWords(vizSheetResult(nil))))
	if geo.calL != 0 || geo.calR != 0 {
		t.Errorf("senza campioni la geometria non cambia: %+v", geo)
	}
}

func TestVIZApplyCalibrationOneColumnOnly(t *testing.T) {
	vizNoCalib = false // la calibrazione e' spenta di default (vedi vizNoCalib)
	defer func() { vizNoCalib = true }()
	// solo la colonna destra misurata: lo stesso spostamento vale per tutta la pagina
	specs := []vizWordSpec{vizShifted("data_nascita", 0, "01-01-2000", 0.012), vizShifted("numero_libretto", 0, "PA0123401", 0.011)}
	geo := vizApplyCalibration(vizGeom{w: 1000}, vizSheetSlots(vizWords(vizSheetResult(specs))))
	if math.Abs(geo.calR-0.0115) > 0.003 || geo.calL != geo.calR {
		t.Errorf("una sola colonna misurata: sinistra %.4f, destra %.4f", geo.calL, geo.calR)
	}
}

func TestVIZCalOffsetInterpolates(t *testing.T) {
	geo := vizGeom{w: 1000, calL: 0.02, calR: -0.01}
	left := vizBand{xa: 0.262, xb: 0.52}
	right := vizBand{xa: 0.885, xb: 1.12}
	mid := vizBand{xa: 0.37, xb: 0.76}
	if got := vizCalOffset(geo, left); math.Abs(got-0.02) > 0.0005 {
		t.Errorf("colonna sinistra %.4f", got)
	}
	if got := vizCalOffset(geo, right); math.Abs(got+0.01) > 0.0005 {
		t.Errorf("colonna destra %.4f", got)
	}
	if got := vizCalOffset(geo, mid); got >= 0.02 || got <= -0.01 {
		t.Errorf("la fascia centrale va tra le due colonne: %.4f", got)
	}
	if vizCalOffset(vizGeom{w: 1000}, left) != 0 {
		t.Error("senza calibrazione nessuno spostamento")
	}
	// il rettangolo del ritaglio segue lo spostamento
	b := vizBands[vizBandIndex("data_scadenza")]
	r0 := vizBandRect(vizGeom{x0: 100, y0: 1000, w: 1000}, b, vizVariants[0], 0, 0)
	r1 := vizBandRect(vizGeom{x0: 100, y0: 1000, w: 1000, calL: 0.02, calR: 0.02}, b, vizVariants[0], 0, 0)
	if d := r1.Min.Y - r0.Min.Y; d < 19 || d > 21 {
		t.Errorf("spostamento del ritaglio %d px, atteso 20", d)
	}
}

func TestVIZExtractVIZCalibratesBeforeBuildingCrops(t *testing.T) {
	vizNoCalib = false // la calibrazione e' spenta di default (vedi vizNoCalib)
	defer func() { vizNoCalib = true }()
	// dopo prepareVIZImage la lettura del foglio con righe spostate calibra la geometria prima che extractVIZ costruisca i ritagli
	vizNoOCR(t)
	g := vizDrawMRZ(1000, 1400, 120, 1250, 16, 0)
	_ = prepareVIZImage(g)
	var gotL, gotR float64
	vizTraceCalib = func(l, r float64, nl, nr int) { gotL, gotR = l, r }
	defer func() { vizTraceCalib = nil }()
	var specs []vizWordSpec
	for v := 0; v < 3; v++ {
		specs = append(specs, vizShifted("data_emissione", v, "09-03-2026", 0.015))
		specs = append(specs, vizShifted("data_nascita", v, "01-01-2000", -0.008))
	}
	res := &Result{Fields: map[string]string{}}
	extractVIZ(vizSheetResult(specs), res)
	if math.Abs(gotL-0.015) > 0.003 || math.Abs(gotR+0.008) > 0.003 {
		t.Errorf("calibrazione in extractVIZ: %.4f e %.4f, attesi 0,015 e -0,008", gotL, gotR)
	}
}
