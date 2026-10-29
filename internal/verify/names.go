package verify

import (
	"strings"
	"unicode"
)

// mrzNameLen e' la lunghezza del campo nome della riga 1 dell'MRZ (posizioni 6-44).
const mrzNameLen = 39

// nameWords divide un testo di nomi in parole normalizzate: maiuscole, accenti tolti, tutto cio' che non e' una lettera
// (spazio, apostrofo, trattino, `<`) separa le parole.
func nameWords(s string) []string {
	return strings.Fields(foldToLetters(s))
}

// foldToLetters normalizza come Norm ma tiene i separatori come spazio, cosi' che "D'ANGELO" diventi due parole.
func foldToLetters(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if f, ok := accentFold[r]; ok {
			b.WriteString(f)
			continue
		}
		if r == 'ß' {
			b.WriteString("SS")
			continue
		}
		if unicode.IsLetter(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// NameMatchesMRZ dice se il cognome e i nomi letti dalla riga 1 dell'MRZ corrispondono alla riga del nome stampata
// (Truth.NameRow). Il confronto e' per parole, senza ordine, con accenti e punteggiatura ignorati. L'MRZ ha 39 caratteri per il
// nome e li tronca: una parola dell'MRZ che e' un prefisso di una parola stampata conta come uguale, e le parole
// stampate che mancano in fondo contano solo se non sarebbero entrate nei 39 caratteri. La riga 1 non ha check digit:
// questo confronto e' l'unico modo di sapere se e' stata letta giusta.
func NameMatchesMRZ(surname, givenNames, nameRow string) bool {
	sw, gw := nameWords(surname), nameWords(givenNames)
	mrz := append(append([]string{}, sw...), gw...)
	printed := nameWords(nameRow)
	if len(mrz) == 0 || len(printed) == 0 {
		return false
	}
	// parole stampate non ancora abbinate; ogni parola dell'MRZ ne consuma al piu' una
	free := append([]string{}, printed...)
	var unmatched []string
	for _, m := range mrz {
		if i := indexOf(free, m); i >= 0 {
			free = append(free[:i], free[i+1:]...)
		} else {
			unmatched = append(unmatched, m)
		}
	}
	truncated := false
	switch len(unmatched) {
	case 0:
	case 1: // una sola parola puo' essere stata troncata dall'MRZ: deve essere il prefisso di una parola stampata
		m := unmatched[0]
		i := prefixOf(free, m)
		if i < 0 {
			return false
		}
		free = append(free[:i], free[i+1:]...)
		truncated = true
	default:
		return false
	}
	// lunghezza del campo nome come l'ha letto l'MRZ: cognome, `<<`, nomi
	core := coreLen(sw) + 2 + coreLen(gw)
	if len(gw) == 0 {
		core = coreLen(sw)
	}
	if truncated {
		// una parola tagliata e' credibile solo se riempie il campo di 39 caratteri (2 di tolleranza per l'OCR); e le
		// parole stampate che seguono non ci stavano piu'
		return core >= mrzNameLen-2
	}
	if len(free) == 0 {
		return true
	}
	// restano parole stampate che nell'MRZ non ci sono: va bene solo se nel campo di 39 caratteri non c'era piu' posto
	shortest := len(free[0])
	for _, w := range free {
		shortest = min(shortest, len(w))
	}
	return core+1+shortest > mrzNameLen
}

// coreLen e' la lunghezza di un gruppo di parole separate da un `<` ciascuna.
func coreLen(ws []string) int {
	n := 0
	for _, w := range ws {
		n += len(w)
	}
	if len(ws) > 1 {
		n += len(ws) - 1
	}
	return n
}

func indexOf(ws []string, w string) int {
	for i, x := range ws {
		if x == w {
			return i
		}
	}
	return -1
}

// prefixOf cerca una parola di ws di cui p sia un prefisso proprio (p piu' corta della parola).
func prefixOf(ws []string, p string) int {
	for i, x := range ws {
		if len(p) < len(x) && strings.HasPrefix(x, p) {
			return i
		}
	}
	return -1
}

// NameStatus e' l'esito del confronto tra il cognome e i nomi dell'MRZ e la riga del nome stampata.
type NameStatus int

const (
	// NameNA: la verita' non ha la riga del nome (`_riga_nome`), niente da confrontare.
	NameNA NameStatus = iota
	// NameOK: cognome e nomi dell'MRZ corrispondono alla riga stampata (vedi NameMatchesMRZ).
	NameOK
	// NameWrong: l'MRZ ha un cognome e dei nomi, ma non corrispondono alla riga stampata.
	NameWrong
	// NameMissing: l'uscita non ha cognome e nomi (MRZ non trovato).
	NameMissing
)

// String e' l'etichetta stampata nei report.
func (s NameStatus) String() string {
	switch s {
	case NameOK:
		return "OK"
	case NameWrong:
		return "SBAGLIATO"
	case NameMissing:
		return "MANCANTE"
	}
	return "NON_VERIFICABILE"
}

// nameStatus confronta i nomi dell'MRZ nell'uscita con la riga del nome della verita'.
func nameStatus(fields map[string]string, truth Truth) NameStatus {
	if truth.NameRow == "" {
		return NameNA
	}
	surname, given := fields["surname"], fields["given_names"]
	if surname == "" && given == "" {
		return NameMissing
	}
	if NameMatchesMRZ(surname, given, truth.NameRow) {
		return NameOK
	}
	return NameWrong
}
