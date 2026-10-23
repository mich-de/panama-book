package mrz

import (
	"strings"
)

// Correzioni elenca le modifiche fatte da Repair, per poterle mostrare all'operatore.
type Correzioni []string

var lookalikeFiller = strings.NewReplacer("«", "<", "‹", "<", "〈", "<", "＜", "<", "≪", "<")

// confusi per cifre: lettere che l'OCR scambia con una cifra.
var toDigit = map[byte]byte{'O': '0', 'Q': '0', 'D': '0', 'I': '1', 'L': '1', 'Z': '2', 'S': '5', 'B': '8', 'G': '6', 'T': '7'}

// confusi per lettere: cifre che l'OCR scambia con una lettera.
var toAlpha = map[byte]byte{'0': 'O', '1': 'I', '2': 'Z', '5': 'S', '8': 'B', '6': 'G'}

type class int

const (
	alpha class = iota
	digit
	alnum
	sexc
	fillOrDigit
)

// classi per posizione, riga 2 TD3.
func classLine2(i int) class {
	switch {
	case i < 9:
		return alnum
	case i == 9:
		return digit
	case i < 13:
		return alpha
	case i < 20:
		return digit
	case i == 20:
		return sexc
	case i < 28:
		return digit
	case i < 42:
		return alnum
	case i == 42:
		return fillOrDigit
	default:
		return digit
	}
}

// Normalize porta le due righe OCR a 44 caratteri nell'alfabeto MRZ, correggendo gli scambi
// piu' comuni in base alla classe della posizione (cifra o lettera).
// Ritorna anche le correzioni applicate. Se una riga non ha 44 caratteri: la riga 1 viene
// riempita o accorciata in coda (sono solo '<'), la riga 2 no perche' le posizioni si sposterebbero.
func Normalize(line1, line2 string) (string, string, Correzioni) {
	var notes Correzioni
	l1 := cleanOCR(line1)
	l2 := cleanOCR(line2)

	if len(l1) < lineLen {
		l1 += strings.Repeat("<", lineLen-len(l1))
	} else if len(l1) > lineLen {
		l1 = l1[:lineLen]
	}

	b1 := []byte(l1)
	for i := 0; i < 5 && i < len(b1); i++ { // codice documento + stato: lettere
		if v, ok := toAlpha[b1[i]]; ok {
			notes = append(notes, note(1, i, b1[i], v))
			b1[i] = v
		}
	}
	for i := 5; i < len(b1); i++ { // nomi: solo lettere e '<'
		if v, ok := toAlpha[b1[i]]; ok {
			notes = append(notes, note(1, i, b1[i], v))
			b1[i] = v
		}
	}

	if len(l2) != lineLen {
		return string(b1), l2, notes
	}
	b2 := []byte(l2)
	for i := range b2 {
		switch classLine2(i) {
		case digit:
			if v, ok := toDigit[b2[i]]; ok {
				notes = append(notes, note(2, i, b2[i], v))
				b2[i] = v
			}
		case alpha:
			if v, ok := toAlpha[b2[i]]; ok {
				notes = append(notes, note(2, i, b2[i], v))
				b2[i] = v
			}
		case sexc:
			if b2[i] != 'M' && b2[i] != 'F' && b2[i] != '<' {
				if b2[i] == 'H' || b2[i] == 'N' { // M letta male
					notes = append(notes, note(2, i, b2[i], 'M'))
					b2[i] = 'M'
				}
			}
		case fillOrDigit:
			if v, ok := toDigit[b2[i]]; ok {
				notes = append(notes, note(2, i, b2[i], v))
				b2[i] = v
			}
		}
	}
	return string(b1), string(b2), notes
}

func note(line, pos int, from, to byte) string {
	return "riga" + string(rune('0'+line)) + " pos " + itoa(pos+1) + ": " + string(from) + "→" + string(to)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func cleanOCR(s string) string {
	s = lookalikeFiller.Replace(s)
	s = strings.ToUpper(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '<':
			b.WriteRune(r)
		}
		// tutto il resto (spazi, punteggiatura) viene scartato
	}
	return b.String()
}

// Repair normalizza le righe, le interpreta e, se un check digit non torna, prova a sostituire
// un solo carattere confondibile nel campo interessato. Le sostituzioni sono riportate.
func Repair(line1, line2 string) (Data, Correzioni, error) {
	l1, l2, notes := Normalize(line1, line2)
	d, err := Parse(l1, l2)
	if err != nil {
		return d, notes, err
	}
	if d.Checks.AllOK() {
		return d, notes, nil
	}

	b2 := []byte(l2)
	// Per ogni campo con check digit non valido si cercano le sostituzioni di un solo carattere
	// confondibile che lo fanno tornare. Un check digit a una cifra torna per caso 1 volta su 10,
	// quindi si accetta la correzione solo se e' l'unica che fa tornare anche il controllo composito.
	tryField := func(start, end, cdPos int) {
		if CheckDigit(string(b2[start:end])) == b2[cdPos] {
			return
		}
		type fix struct {
			pos        int
			from, cand byte
		}
		var ok []fix
		for i := start; i < end; i++ {
			orig := b2[i]
			for _, cand := range candidates(orig) {
				b2[i] = cand
				field := string(b2[start:end])
				composite := CheckDigit(string(b2[0:10])+string(b2[13:20])+string(b2[21:43])) == b2[43]
				if CheckDigit(field) == b2[cdPos] && composite {
					ok = append(ok, fix{i, orig, cand})
				}
			}
			b2[i] = orig
		}
		if len(ok) == 1 {
			b2[ok[0].pos] = ok[0].cand
			notes = append(notes, note(2, ok[0].pos, ok[0].from, ok[0].cand)+" (check digit)")
		}
	}
	tryField(0, 9, 9)
	tryField(13, 19, 19)
	tryField(21, 27, 27)

	d2, err := Parse(l1, string(b2))
	if err != nil {
		return d, notes, err
	}
	return d2, notes, nil
}

func candidates(c byte) []byte {
	var out []byte
	if v, ok := toDigit[c]; ok {
		out = append(out, v)
	}
	if v, ok := toAlpha[c]; ok {
		out = append(out, v)
	}
	// scambi tra cifre simili
	switch c {
	case '1':
		out = append(out, '7')
	case '7':
		out = append(out, '1')
	case '3':
		out = append(out, '8')
	case '8':
		out = append(out, '3', '6')
	case '5':
		out = append(out, '6')
	case '6':
		out = append(out, '5', '8')
	case '0':
		out = append(out, '8')
	}
	return out
}
