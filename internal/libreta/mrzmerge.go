package libreta

// File dell'integratore: confronto tra la lettura OCR dell'MRZ (readMRZBand) e la seconda lettura del lettore a
// template (mrzCellsReader) sulla stessa immagine, per le parti dell'MRZ che i check digit non coprono.
//
// I check digit validano il numero libretto, la nascita, la scadenza e il campo opzionale. Non validano la riga 1 (tipo,
// emittente, cognome e nomi) ne', nella riga 2, la nazionalita' (celle 11-13) e il sesso (cella 21): una riga 1
// sbagliata con prefisso ISPAN e 44 caratteri passa tutti i controlli. Il lettore a template segnala nelle sue note le
// celle di cui non e' sicuro (probabilita' bassa, forma mai vista, cella cambiata dai vincoli); nel suo corpus di prova
// tutte le celle sbagliate erano segnalate (docs/agenti/12-mrz-template.md).

import (
	"fmt"
	"slices"
	"strings"

	"scanlibretto/internal/mrz"
)

// mrzLineLen e' la lunghezza di una riga MRZ (TD3).
const mrzLineLen = 44

// Avvisi interni, tradotti in codici stabili da cmd/libretto/output.go (toWarning). Non contengono valori.
const (
	warnMRZLine1       = "riga 1 MRZ incerta: OCR e lettore a template differiscono"
	warnMRZFieldPrefix = "campo MRZ incerto: " // seguito dal nome interno del campo (nazionalita_iso, sesso)
)

// mrzUncheckedZones sono le parti della riga 2 che nessun check digit copre (indici da 0, [from,to)).
var mrzUncheckedZones = []struct {
	field    string // nome interno del campo (chiavi di Result.Fields)
	note     string // nota da aggiungere alle correzioni quando si prende la lettura a template
	from, to int
}{
	{"nazionalita_iso", "riga2: nazionalita' presa dal lettore a template (OCR e lettore differivano, nessuna cella diversa era incerta)", 10, 13},
	{"sesso", "riga2: sesso preso dal lettore a template (OCR e lettore differivano, nessuna cella diversa era incerta)", 20, 21},
}

// reconcileMRZ confronta la lettura OCR o (con tutti i check digit) con quella del lettore a template c, anch'essa con
// tutti i check digit, e risolve le differenze nelle parti senza check digit:
//   - se le due letture differiscono e nessuna delle celle diverse e' segnalata come incerta dalle note del lettore a
//     template (cnotes), si prende la lettura a template e lo si dice nelle correzioni;
//   - se una cella diversa e' segnalata incerta si tiene l'OCR e si restituisce un avviso;
//   - se differiscono le parti coperte dai check digit (due letture valide ma diverse: per caso, 1 volta su 100000) non si
//     decide niente.
//
// Le parti coperte dai check digit restano quelle dell'OCR, quindi i check digit non peggiorano mai. Restituisce i
// dati (o stesso), le correzioni (onotes piu' quelle aggiunte) e gli avvisi.
func reconcileMRZ(o *mrz.Data, onotes []string, c *mrz.Data, cnotes []string) (*mrz.Data, []string, []string) {
	if o == nil || c == nil || len(o.Line1) != mrzLineLen || len(o.Line2) != mrzLineLen ||
		len(c.Line1) != mrzLineLen || len(c.Line2) != mrzLineLen {
		return o, onotes, nil
	}
	if checkedPart(o.Line2) != checkedPart(c.Line2) {
		return o, onotes, nil
	}
	flag1, flag2 := flaggedCells(cnotes, 1), flaggedCells(cnotes, 2)
	line1, line2 := o.Line1, []byte(o.Line2)
	notes := slices.Clone(onotes)
	var warns []string
	changed := false

	// riga 1: contano le lettere, non la posizione dei riempimenti '<' (la stessa riga si puo' leggere con '<' in piu' o in meno)
	if d := diffPositions(o.Line1, c.Line1); len(d) > 0 && lettersOf(o.Line1) != lettersOf(c.Line1) {
		if anyFlagged(flag1, d) {
			warns = append(warns, warnMRZLine1)
		} else {
			line1, changed = c.Line1, true
			notes = append(notes, "riga1: cognome e nomi presi dal lettore a template (OCR e lettore differivano, nessuna cella diversa era incerta)")
		}
	}
	// riga 2: nazionalita' e sesso
	for _, z := range mrzUncheckedZones {
		d := diffPositions(o.Line2[z.from:z.to], c.Line2[z.from:z.to])
		if len(d) == 0 {
			continue
		}
		for i := range d {
			d[i] += z.from
		}
		if anyFlagged(flag2, d) {
			warns = append(warns, warnMRZFieldPrefix+z.field)
			continue
		}
		copy(line2[z.from:z.to], c.Line2[z.from:z.to])
		changed = true
		notes = append(notes, z.note)
	}
	if !changed {
		return o, onotes, warns
	}
	d, err := mrz.Parse(line1, string(line2))
	if err != nil || !d.Checks.AllOK() {
		return o, onotes, warns // le parti sostituite non hanno check digit, quindi non dovrebbe succedere: se succede, resta l'OCR
	}
	return &d, notes, warns
}

// checkedPart sono i caratteri della riga 2 coperti dai check digit, check digit compresi: tutto tranne la nazionalita'
// (celle 11-13) e il sesso (cella 21).
func checkedPart(line2 string) string { return line2[0:10] + line2[13:20] + line2[21:44] }

// diffPositions elenca le posizioni (da 0) in cui due stringhe della stessa lunghezza differiscono.
func diffPositions(a, b string) []int {
	var out []int
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			out = append(out, i)
		}
	}
	return out
}

// lettersOf toglie i riempimenti '<': il contenuto di una riga 1 senza il modo in cui e' spaziato.
func lettersOf(s string) string { return strings.ReplaceAll(s, "<", "") }

// flaggedCells restituisce le celle (da 0) della riga row (1 o 2) che le note del lettore a template segnalano: celle
// incerte, cambiate dai vincoli o riconosciute da un glifo sintetico. Le note hanno la forma "riga2 pos 7: ...".
func flaggedCells(notes []string, row int) map[int]bool {
	m := map[int]bool{}
	for _, n := range notes {
		var r, p int
		if _, err := fmt.Sscanf(n, "riga%d pos %d:", &r, &p); err == nil && r == row && p >= 1 {
			m[p-1] = true
		}
	}
	return m
}

// anyFlagged dice se almeno una delle posizioni e' segnalata.
func anyFlagged(flagged map[int]bool, positions []int) bool {
	for _, p := range positions {
		if flagged[p] {
			return true
		}
	}
	return false
}
