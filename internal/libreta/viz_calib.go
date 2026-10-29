package libreta

// Calibrazione verticale per scansione. Le posizioni dei campi (vizBands) sono in unita' di W dal bordo superiore della
// prima riga MRZ, ma la pagina dati non e' un corpo rigido con la MRZ: sui libretti veri la pagina si incurva verso il
// fondo (la MRZ sta in basso, vicino alla piega) e le righe dei dati risultano spostate di 0,01-0,03 W rispetto alla MRZ,
// diversamente nella colonna sinistra e in quella destra (misurato con la verita' su 62 immagini: da -0,01 a +0,03 W).
// Le finestre strette (0,53 volte la standard) allora si centrano sull'etichetta invece che sul valore e leggono
// rumore. Lo scostamento si misura sul foglio gia' letto dall'OCR: per i campi con un formato riconoscibile (date, numero
// libretto, id marittimo di 9 cifre, livello) si trova la riga che lo contiene e si confronta la sua posizione con
// quella attesa; la mediana per colonna da' lo scostamento della colonna, che si applica a tutti i ritagli (con
// interpolazione lineare in orizzontale: la pendenza dei dati e' diversa da quella della MRZ).

import (
	"sort"
	"strings"
)

// Ascisse (in W) dei punti di misura delle due colonne: il centro orizzontale delle fasce di ancoraggio.
const (
	vizCalXL = 0.391 // colonna sinistra: fasce 0,262-0,52
	vizCalXR = 1.0025
	// vizCalMax: scostamento massimo accettato (in W); oltre, la riga trovata e' quella di un campo vicino.
	vizCalMax = 0.034
)

// vizAnchors: fasce con formato riconoscibile e la colonna a cui appartengono.
var vizAnchors = []struct {
	key   string
	right bool
}{
	{"data_emissione", false}, {"data_scadenza", false}, {"livello", false}, {"id_marino", false},
	{"data_nascita", true}, {"numero_libretto", true},
}

// vizSheetSlots divide le parole del foglio nelle fasce (una per variante e campo, piu' le due righe MRZ).
func vizSheetSlots(words []word) [][]word {
	slots := make([][]word, len(vizVariants)*len(vizBands)+2)
	for _, w := range words {
		i := int(w.cy() / vizBandH)
		if i >= 0 && i < len(slots) {
			slots[i] = append(slots[i], w)
		}
	}
	return slots
}

// vizAnchorLineY cerca nelle parole di una fascia la riga che ha il formato del campo e ne restituisce il centro verticale.
func vizAnchorLineY(key string, ws []word) (cy float64, ok bool) {
	for _, l := range vizGroupLines(ws) {
		for _, t := range []string{l.text, vizFirstSegment(l)} {
			switch key {
			case "data_emissione", "data_scadenza", "data_nascita":
				if _, ok := vizPickDate(vizDates(t), key); ok {
					return l.cy(), true
				}
			case "numero_libretto":
				if _, ok := vizLibretto(t); ok {
					return l.cy(), true
				}
			case "id_marino":
				if _, ok := vizNum9(t); ok && !strings.ContainsAny(t, "/") {
					return l.cy(), true
				}
			case "livello":
				if _, ok := vizLevel(t); ok && !vizIsLabelish(t) {
					return l.cy(), true
				}
			}
		}
	}
	return 0, false
}

// vizCalibrate misura lo scostamento verticale (in W) delle due colonne dalla lettura del foglio. n* sono i campioni usati.
// Lo scostamento e' positivo se la riga sta piu' in basso del previsto.
func vizCalibrate(slots [][]word) (calL, calR float64, nL, nR int) {
	nb := len(vizBands)
	var left, right []float64
	for _, a := range vizAnchors {
		bi := -1
		for i, b := range vizBands {
			if b.key == a.key {
				bi = i
			}
		}
		if bi < 0 {
			continue
		}
		b := vizBands[bi]
		for vi, v := range vizVariants {
			ws := slots[vi*nb+bi]
			cy, ok := vizAnchorLineY(a.key, ws)
			if !ok {
				continue
			}
			up, _ := vizWindow(b, v)
			exp := float64((vi*nb+bi)*vizBandH+vizBandPad) + up*v.unit
			off := (cy - exp) / v.unit
			if off > vizCalMax || off < -vizCalMax {
				continue
			}
			if a.right {
				right = append(right, off)
			} else {
				left = append(left, off)
			}
		}
	}
	return vizMedian(left), vizMedian(right), len(left), len(right)
}

func vizMedian(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// vizApplyCalibration imposta in geo lo scostamento misurato: se una colonna non ha campioni prende quello dell'altra
// (spostamento rigido, con la differenza media tra le colonne gia' inclusa nelle costanti di vizBands).
func vizApplyCalibration(geo vizGeom, slots [][]word) vizGeom {
	if vizNoCalib {
		return geo
	}
	l, r, nl, nr := vizCalibrate(slots)
	switch {
	case nl == 0 && nr == 0:
		return geo
	case nl == 0:
		l = r
	case nr == 0:
		r = l
	}
	geo.calL, geo.calR = l, r
	if vizTraceCalib != nil {
		vizTraceCalib(l, r, nl, nr)
	}
	return geo
}

// vizNoCalib disattiva la calibrazione verticale. Misurata sul corpus: con la calibrazione le finestre dei ritagli
// si spostano di qualche pixel e in alcune scansioni le letture di riga intera sono vuote (regressioni su qualifica e
// luogo di nascita), quindi e' spenta; il codice resta per riprovarla con LAB_CALIB nel laboratorio.
var vizNoCalib = true

// vizTraceCalib (solo diagnostica dei test) riceve gli scostamenti misurati e il numero di campioni per colonna.
var vizTraceCalib func(calL, calR float64, nL, nR int)

// vizCalOffset e' lo scostamento verticale (in W) da applicare al ritaglio di b: interpolazione lineare tra le due colonne
// al centro orizzontale della fascia.
func vizCalOffset(geo vizGeom, b vizBand) float64 {
	if geo.calL == 0 && geo.calR == 0 {
		return 0
	}
	x := (b.xa + b.xb) / 2
	return geo.calL + (geo.calR-geo.calL)*(x-vizCalXL)/(vizCalXR-vizCalXL)
}
