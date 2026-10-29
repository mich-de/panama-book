package libreta

// Varianti "jitter" dei testi liberi. L'OCR dei testi in grassetto su sfondo di ologramma e' fragile: spostando il
// ritaglio di 3-5 pixel lo stesso campo si legge giusto o sbagliato a caso (misurato su un libretto vero: il cargo
// e il luogo di nascita risultavano esatti in 3 spostamenti su 7), mentre i numeri sono stabili. Gli errori dei
// ritagli spostati sono quasi indipendenti, quindi il voto per carattere li corregge. Si aggiungono solo sulle
// scansioni a tratto spesso (le reali) e solo ai campi di testo, per limitare il tempo (circa 24 ritagli).

const (
	vizJitterBase = 5000 // primo slot dei ritagli jitter
	vizJitterStep = 0.003
)

// vizJitterBands: campi di testo libero senza controllo sull'MRZ (la nazionalita' si riallinea al codice MRZ).
var vizJitterBands = map[string]bool{"luogo_nascita": true, "cargo": true, "regionale": true}

// vizJitter e' una variante spostata: v e' la variante di base, ox/oy lo spostamento in unita' di W.
type vizJitter struct {
	v      vizVariant
	ox, oy float64
}

// vizJitterID: identificativo di variante (per i pesi) dopo le varianti di base e aggiuntive.
func vizJitterID(nExtra, k int) int { return 2*len(vizVariants) + nExtra + k }

// vizJitterJobs elenca le varianti spostate: tre varianti di base (rampa a soglia thr in finestra stretta e grigi a scala 1650) per quattro spostamenti (su, giu', sinistra, destra).
func vizJitterJobs(thr int) []vizJitter {
	bases := []vizVariant{
		{1950, 0.53, 0.53, false, thr},
		{1650, 0.85, 1.0, false, 0},
	}
	offs := [][2]float64{{0, -1.3 * vizJitterStep}, {0, 1.3 * vizJitterStep}, {-vizJitterStep, 0}, {vizJitterStep, 0}}
	var out []vizJitter
	for _, o := range offs {
		for _, v := range bases {
			out = append(out, vizJitter{v, o[0], o[1]})
		}
	}
	return out
}
