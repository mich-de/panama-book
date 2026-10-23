// Package verify controlla i JSON prodotti da `libretto extract` in due modi: contro la verita' letta a occhio
// sull'immagine (campo per campo: OK, SBAGLIATO, MANCANTE, NON_STAMPATO) e con regole di coerenza che non hanno
// bisogno della verita' (formati del libretto di Panama, ordine delle date, nazionalita' contro il codice ISO,
// zona visiva contro MRZ, caratteri anomali).
//
// Regola di privacy: nessuna funzione di questo pacchetto che produce testo per l'utente (report, esiti, messaggi
// dei controlli) mette dentro un valore letto o una verita': solo nomi di campo, conteggi e classi di errore.
// I valori restano in memoria, per i confronti.
package verify

import (
	"strings"
	"time"
	"unicode"
)

// accentFold associa le lettere accentate (gia' maiuscole) alla lettera di base.
var accentFold = func() map[rune]string {
	groups := map[string]string{
		"A":  "ÀÁÂÃÄÅĀĂĄ",
		"C":  "ÇĆĈĊČ",
		"D":  "ĎĐ",
		"E":  "ÈÉÊËĒĔĖĘĚ",
		"G":  "ĜĞĠĢ",
		"H":  "ĤĦ",
		"I":  "ÌÍÎÏĨĪĬĮİ",
		"J":  "Ĵ",
		"K":  "Ķ",
		"L":  "ĹĻĽĿŁ",
		"N":  "ÑŃŅŇ",
		"O":  "ÒÓÔÕÖØŌŎŐ",
		"R":  "ŔŖŘ",
		"S":  "ŚŜŞŠ",
		"T":  "ŢŤŦ",
		"U":  "ÙÚÛÜŨŪŬŮŰŲ",
		"W":  "Ŵ",
		"Y":  "ÝŶŸ",
		"Z":  "ŹŻŽ",
		"AE": "Æ",
		"OE": "Œ",
		"SS": "ẞ",
	}
	m := map[rune]string{}
	for base, letters := range groups {
		for _, r := range letters {
			m[r] = base
		}
	}
	return m
}()

// Norm normalizza un valore per il confronto: maiuscolo, senza accenti, solo lettere e cifre. Spazi, trattini,
// barre, punti e ogni altra punteggiatura spariscono: "4-123-4567" e "41234567" sono lo stesso valore.
func Norm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if f, ok := accentFold[r]; ok {
			b.WriteString(f)
			continue
		}
		if r == 'ß' { // ß: ToUpper la lascia com'e'
			b.WriteString("SS")
			continue
		}
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || (r > unicode.MaxASCII && unicode.IsLetter(r)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ymd e' una data di calendario (anno, mese, giorno). Non si stampa mai: e' un dato personale.
type ymd struct{ y, m, d int }

// time restituisce la data come time.Time (mezzanotte UTC).
func (d ymd) time() time.Time { return time.Date(d.y, time.Month(d.m), d.d, 0, 0, 0, 0, time.UTC) }

// before dice se d e' strettamente prima di o.
func (d ymd) before(o ymd) bool { return d.time().Before(o.time()) }

// parseDate legge una data nelle forme GG-MM-AAAA, GG/MM/AAAA, GG.MM.AAAA e AAAA-MM-GG (e con "/" o "."). Controlla
// che esista sul calendario. Cifre di troppo o di meno nell'anno (due cifre) non sono ammesse.
func parseDate(s string) (ymd, bool) {
	s = strings.TrimSpace(s)
	f := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '/' || r == '.' || r == ' ' })
	if len(f) != 3 {
		return ymd{}, false
	}
	n := make([]int, 3)
	for i, p := range f {
		v, ok := atoi(p)
		if !ok {
			return ymd{}, false
		}
		n[i] = v
	}
	var d ymd
	switch {
	case len(f[0]) == 4 && len(f[1]) <= 2 && len(f[2]) <= 2:
		d = ymd{n[0], n[1], n[2]}
	case len(f[2]) == 4 && len(f[0]) <= 2 && len(f[1]) <= 2:
		d = ymd{n[2], n[1], n[0]}
	default:
		return ymd{}, false
	}
	if d.y < 1 || d.m < 1 || d.m > 12 || d.d < 1 {
		return ymd{}, false
	}
	t := d.time()
	if t.Year() != d.y || int(t.Month()) != d.m || t.Day() != d.d {
		return ymd{}, false
	}
	return d, true
}

func atoi(s string) (int, bool) {
	if s == "" || len(s) > 4 {
		return 0, false
	}
	v := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		v = v*10 + int(r-'0')
	}
	return v, true
}

// isDateField dice se il campo e' una data (confronto per giorno, mese e anno e non per testo).
func isDateField(field string) bool {
	switch field {
	case "date_of_issue", "date_of_expiry", "date_of_birth":
		return true
	}
	return false
}

// Equal dice se il valore letto got coincide con la verita' want per il campo field. Il confronto e' normalizzato:
// maiuscole, spazi, trattini, accenti e punteggiatura non contano. Le date si confrontano come date: "09-03-2026" e
// "2026-03-09" sono uguali. Due valori vuoti sono uguali.
func Equal(field, got, want string) bool {
	if isDateField(field) {
		g, gok := parseDate(got)
		w, wok := parseDate(want)
		if gok && wok {
			return g == w
		}
	}
	return Norm(got) == Norm(want)
}
