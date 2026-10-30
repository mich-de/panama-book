package certif

import (
	"sort"
	"strings"

	"scanlibretto/internal/ocr"
)

// RowsFromOCR ricostruisce le righe fisiche del foglio dalle parole dell'OCR. Il motore di Windows raggruppa il testo a modo suo
// (di solito per colonna: prima tutte le etichette, poi tutti i valori); qui si guarda solo la posizione, cosi' «con identificacion
// A12345678 , de SUDAFRICANA» torna una riga sola, come sul foglio.
func RowsFromOCR(r *ocr.Result) []string {
	var ws []ocr.Word
	for _, l := range r.Lines {
		ws = append(ws, l.Words...)
	}
	sort.SliceStable(ws, func(i, j int) bool { return ws[i].Y+ws[i].H/2 < ws[j].Y+ws[j].H/2 })
	type row struct {
		yc, h float64
		ws    []ocr.Word
	}
	var rows []*row
	for _, w := range ws {
		yc := w.Y + w.H/2
		if n := len(rows); n > 0 {
			last := rows[n-1]
			// stessa riga se il centro e' dentro mezza altezza di riga (la riga e' alta quanto la sua parola piu' bassa)
			if d := yc - last.yc; d < 0.5*min(last.h, w.H) {
				last.ws = append(last.ws, w)
				last.yc += (yc - last.yc) / float64(len(last.ws))
				last.h = max(last.h, w.H)
				continue
			}
		}
		rows = append(rows, &row{yc: yc, h: w.H, ws: []ocr.Word{w}})
	}
	out := make([]string, 0, len(rows))
	for _, rw := range rows {
		sort.SliceStable(rw.ws, func(i, j int) bool { return rw.ws[i].X < rw.ws[j].X })
		parts := make([]string, len(rw.ws))
		for i, w := range rw.ws {
			parts[i] = w.Text
		}
		out = append(out, strings.Join(parts, " "))
	}
	return out
}
