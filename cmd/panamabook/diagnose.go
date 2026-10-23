package main

import (
	"fmt"
	"image"
	"math"

	"scanlibretto/internal/imgproc"
)

// Diagnosi automatica della scansione GREZZA (quella che esce dal driver, prima del ritaglio), senza OCR: dice se il libretto e'
// stato inserito decentrato, e' entrato troppo poco o e' inclinato. Serve a capire PERCHE' una scansione e' venuta male. E' solo un
// suggerimento: non blocca nulla. Usa internal/imgproc (AutoCropInfo) in sola lettura.
//
// Le direzioni sono quelle dell'ANTEPRIMA (immagine ruotata di 180 gradi, cioe' come si vede il libretto sullo schermo): la grezza
// e' capovolta, quindi destra/sinistra si scambiano.
const (
	bookWidthMM      = 125.0 // larghezza della pagina dati
	offCenterMM      = 20.0  // oltre questo scostamento del centro dal centro del campo si suggerisce di spostare il libretto
	bookLengthMM     = 176.0 // lunghezza del libretto aperto (due pagine)
	shortFrac        = 0.6   // un documento alto meno di questa frazione del previsto (il minore tra lunghezza letta e libretto aperto) e' entrato troppo poco
	skewHintDeg      = 1.5   // inclinazione oltre la quale si suggerisce di raddrizzare
	touchMarginMM    = 1.5   // il documento a meno di cosi' dal bordo laterale della grezza e' tagliato dal campo di lettura
	minTouchMarginPx = 8
)

// Diagnosis e' l'esito della diagnosi della grezza.
type Diagnosis struct {
	Done        bool     `json:"done"`  // la diagnosi e' stata fatta (in prova/demo no)
	Blank       bool     `json:"blank"` // nella grezza non si vede nessun documento
	OK          bool     `json:"ok"`    // nessun problema trovato
	RawWidthMM  float64  `json:"raw_width_mm"`
	RawHeightMM float64  `json:"raw_height_mm"`
	DocWidthMM  float64  `json:"doc_width_mm"`
	DocHeightMM float64  `json:"doc_height_mm"`
	OffsetMM    float64  `json:"offset_mm"` // centro del libretto rispetto al centro del campo: positivo = verso destra nell'anteprima
	SkewDeg     float64  `json:"skew_deg"`
	Hints       []string `json:"hints"`
	Summary     string   `json:"summary"`
}

// diagnoseRaw legge la grezza e la analizza. dpi e' quello della scansione (da cui i millimetri); lenMM la lunghezza letta.
// flip dice se la grezza e' capovolta rispetto all'anteprima (TWAIN: si ruota di 180 gradi; WIA: no).
func diagnoseRaw(path string, dpi int, lenMM float64, flip bool) (Diagnosis, error) {
	img, err := imgproc.Load(path)
	if err != nil {
		return Diagnosis{}, err
	}
	_, info := imgproc.AutoCropInfo(img)
	b := img.Bounds()
	box := info.Box
	if !flip { // analyzeBox assume la grezza capovolta: senza capovolgimento si specchia il riquadro
		box = image.Rect(b.Dx()-box.Max.X, box.Min.Y, b.Dx()-box.Min.X, box.Max.Y)
	}
	return analyzeBox(b.Dx(), b.Dy(), box, info.SkewDeg, info.Blank, dpi, lenMM), nil
}

// analyzeBox e' la parte pura: dimensioni della grezza (px), riquadro del documento e inclinazione trovati da AutoCropInfo.
func analyzeBox(w, h int, box image.Rectangle, skew float64, blank bool, dpi int, lenMM float64) Diagnosis {
	if dpi <= 0 {
		dpi = 300
	}
	ppm := float64(dpi) / 25.4 // pixel per millimetro
	d := Diagnosis{Done: true, RawWidthMM: round1(float64(w) / ppm), RawHeightMM: round1(float64(h) / ppm)}
	if blank || box.Empty() {
		d.Blank = true
		d.Hints = []string{"Nella scansione grezza non si vede nessun documento: il libretto non è entrato nello scanner (o non è stato rilevato). Controlla l'inserimento e riscansiona."}
		d.Summary = "Nessun documento rilevato nella scansione grezza."
		return d
	}
	d.DocWidthMM, d.DocHeightMM, d.SkewDeg = round1(float64(box.Dx())/ppm), round1(float64(box.Dy())/ppm), round1(skew)

	// coordinate dell'anteprima: la grezza e' capovolta (rotazione di 180 gradi), x' = w - x
	minP, maxP := float64(w-box.Max.X), float64(w-box.Min.X)
	margin := math.Max(touchMarginMM*ppm, minTouchMarginPx)
	touchL, touchR := minP <= margin, maxP >= float64(w)-margin
	docW := float64(box.Dx()) / ppm
	var center float64 // centro stimato del libretto, in mm dal bordo sinistro dell'anteprima
	switch {
	case touchL && touchR: // occupa tutto il campo: niente da dire sul centro
		center = float64(w) / ppm / 2
	case touchR: // tagliato a destra: il lato sinistro e' intero
		center = minP/ppm + bookWidthMM/2
	case touchL:
		center = maxP/ppm - bookWidthMM/2
	default:
		center = (minP + maxP) / 2 / ppm
	}
	offset := center - float64(w)/ppm/2
	d.OffsetMM = round1(offset)

	side := func(off float64) (string, string) {
		if off > 0 {
			return "DESTRA", "SINISTRA"
		}
		return "SINISTRA", "DESTRA"
	}
	if (touchL != touchR) || math.Abs(offset) > offCenterMM {
		from, to := side(offset)
		shift := math.Abs(offset)
		msg := fmt.Sprintf("Il libretto è decentrato verso %s (come nell'anteprima): spostalo di circa %.1f cm verso %s.", from, shift/10, to)
		if touchL != touchR {
			msg = fmt.Sprintf("Il libretto è decentrato verso %s e il bordo %s esce dal campo di lettura (tagliato): spostalo di circa %.1f cm verso %s.",
				from, map[bool]string{true: "destro", false: "sinistro"}[touchR], math.Max(shift, bookWidthMM-docW)/10, to)
		}
		d.Hints = append(d.Hints, msg)
	}
	if expected := math.Min(lenMM, bookLengthMM); d.DocHeightMM < shortFrac*expected {
		d.Hints = append(d.Hints, fmt.Sprintf("Il libretto è entrato troppo poco: il documento rilevato è alto solo %.0f mm invece di circa %.0f. Spingilo fino in fondo, con il bordo MRZ per primo.", d.DocHeightMM, expected))
	}
	// con il documento tagliato dal bordo la stima dell'inclinazione non e' affidabile: si tace
	if math.Abs(skew) >= skewHintDeg && !touchL && !touchR {
		d.Hints = append(d.Hints, fmt.Sprintf("Il bordo del libretto è inclinato di circa %.1f gradi: raddrizzalo e spingilo contro la guida.", math.Abs(skew)))
	}
	d.OK = len(d.Hints) == 0
	d.Summary = fmt.Sprintf("Documento rilevato nella grezza: %.0f x %.0f mm; centro spostato di %.0f mm %s; inclinazione %.1f gradi.",
		d.DocWidthMM, d.DocHeightMM, math.Abs(d.OffsetMM), map[bool]string{true: "a destra", false: "a sinistra"}[offset >= 0], math.Abs(skew))
	return d
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
