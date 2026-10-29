// Package filename costruisce il nome dei file di un libretto ("NOME - COGNOME - NUMERO LIBRETTO") in modo sicuro per
// Windows: solo lettere, cifre e spazi nei nomi, senza accenti ne' caratteri che il sistema non accetta, cosi' il file
// si puo' sempre rinominare, spostare e allegare.
package filename

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Parts sono i dati da cui nasce il nome: nomi e cognome (dall'MRZ) e numero libretto (con check digit validi).
type Parts struct {
	Given, Surname, Book string
}

// maxPartLen limita ogni pezzo: nome file totale ben sotto i 260 caratteri di un percorso Windows.
const maxPartLen = 45

// translit porta le lettere latine accentate o speciali alla loro forma ASCII.
var translit = map[rune]string{
	'À': "A", 'Á': "A", 'Â': "A", 'Ã': "A", 'Ä': "A", 'Å': "A", 'Ā': "A", 'Ă': "A", 'Ą': "A",
	'Ç': "C", 'Ć': "C", 'Č': "C", 'Ď': "D", 'Đ': "D",
	'È': "E", 'É': "E", 'Ê': "E", 'Ë': "E", 'Ē': "E", 'Ę': "E", 'Ě': "E",
	'Ì': "I", 'Í': "I", 'Î': "I", 'Ï': "I", 'Ī': "I", 'İ': "I",
	'Ł': "L", 'Ĺ': "L", 'Ľ': "L", 'Ñ': "N", 'Ń': "N", 'Ň': "N",
	'Ò': "O", 'Ó': "O", 'Ô': "O", 'Õ': "O", 'Ö': "O", 'Ø': "O", 'Ō': "O", 'Ő': "O",
	'Ŕ': "R", 'Ř': "R", 'Ś': "S", 'Š': "S", 'Ş': "S", 'Ţ': "T", 'Ť': "T",
	'Ù': "U", 'Ú': "U", 'Û': "U", 'Ü': "U", 'Ū': "U", 'Ů': "U", 'Ű': "U",
	'Ý': "Y", 'Ÿ': "Y", 'Ź': "Z", 'Ž': "Z", 'Ż': "Z",
	'Æ': "AE", 'Œ': "OE", 'ß': "SS", 'Þ': "TH",
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'ā': "a", 'ă': "a", 'ą': "a",
	'ç': "c", 'ć': "c", 'č': "c", 'ď': "d", 'đ': "d",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ę': "e", 'ě': "e",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ī': "i", 'ı': "i",
	'ł': "l", 'ĺ': "l", 'ľ': "l", 'ñ': "n", 'ń': "n", 'ň': "n",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'ō': "o", 'ő': "o",
	'ŕ': "r", 'ř': "r", 'ś': "s", 'š': "s", 'ş': "s", 'ţ': "t", 'ť': "t",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ū': "u", 'ů': "u", 'ű': "u",
	'ý': "y", 'ÿ': "y", 'ź': "z", 'ž': "z", 'ż': "z",
	'æ': "ae", 'œ': "oe", 'þ': "th",
}

// Clean rende un pezzo di nome sicuro: accenti tolti, tutto cio' che non e' lettera o cifra ASCII (compresi '<', '/',
// ':', apostrofi, trattini, punti) diventa uno spazio, gli spazi doppi si riducono, niente spazi ai bordi.
func Clean(s string) string {
	var b strings.Builder
	for _, r := range s {
		if t, ok := translit[r]; ok {
			b.WriteString(t)
			continue
		}
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
			continue
		}
		b.WriteByte(' ')
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if len(out) > maxPartLen {
		out = strings.TrimSpace(out[:maxPartLen])
	}
	return out
}

// Base restituisce "NOME - COGNOME - NUMERO LIBRETTO". ok e' falso se manca uno dei tre pezzi (dopo la pulizia): in quel
// caso non si deve rinominare niente.
func Base(p Parts) (string, bool) {
	given, surname, book := Clean(p.Given), Clean(p.Surname), Clean(p.Book)
	if given == "" || surname == "" || book == "" {
		return "", false
	}
	return given + " - " + surname + " - " + book, true
}

// Unique restituisce base, oppure "base (2)", "base (3)", ... se in dir esiste gia' un file base+ext per una delle
// estensioni indicate (cosi' immagine, JSON e PDF dello stesso libretto restano insieme e nessuno sovrascrive un altro).
func Unique(dir, base string, exts ...string) string {
	for n := 1; ; n++ {
		cand := base
		if n > 1 {
			cand = fmt.Sprintf("%s (%d)", base, n)
		}
		free := true
		for _, e := range exts {
			if _, err := os.Lstat(filepath.Join(dir, cand+e)); err == nil {
				free = false
				break
			}
		}
		if free {
			return cand
		}
	}
}
