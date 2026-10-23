package libreta

// Sigla di grado della qualifica ("MASTER II/2", "GMDSS GENERAL OPERATOR IV/2"): numerale romano, barra, cifra (i capitoli
// e le regole della convenzione STCW). La grammatica e' generica: nessun elenco di qualifiche, solo la forma
// NUMERALE/CIFRA. Il motore OCR legge la sigla in grassetto a pezzi ("11/2", "1112", "N/2" per IV/2, "TVT2"...): la si
// decodifica con una distanza di modifica pesata contro tutte le sigle della grammatica, con costi bassi per i
// caratteri che si confondono davvero (1 per I, barra letta come 1 o t, V e I fusi in N...).

import (
	"math"
	"strings"
	"sync"
)

var vizRomans = []string{"I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X"}

var (
	vizRankOnce  sync.Once
	vizRankCodes []string // tutte le sigle NUMERALE/CIFRA (cifre 1-9)
)

func vizAllRankCodes() []string {
	vizRankOnce.Do(func() {
		for _, r := range vizRomans {
			for d := '1'; d <= '9'; d++ {
				vizRankCodes = append(vizRankCodes, r+"/"+string(d))
			}
		}
	})
	return vizRankCodes
}

// vizRankSubCost e' il costo di leggere il carattere o al posto del carattere canonico c.
func vizRankSubCost(c, o rune) float64 {
	if c == o {
		return 0
	}
	has := func(set string) bool { return strings.ContainsRune(set, o) }
	switch c {
	case 'I':
		switch {
		case has("1l|!"):
			return 0.15
		case has("iıjJLtT][fF"):
			return 0.4
		}
	case 'V':
		switch {
		case has("v"):
			return 0.05
		case has("UuYyW"):
			return 0.4
		}
	case 'X':
		switch {
		case has("xK"):
			return 0.1
		case has("kY+"):
			return 0.4
		}
	case '/':
		switch {
		case has("\\"):
			return 0.05
		case has("1l|!I"):
			return 0.4
		case has("tT7,.'`’"):
			return 0.5
		}
	case '1':
		if has("Il|!") {
			return 0.15
		}
	case '2':
		if has("Zz") {
			return 0.3
		}
	case '3':
		if has("8B") {
			return 0.5
		}
	case '4':
		if has("AH") {
			return 0.5
		}
	case '5':
		if has("Ss") {
			return 0.3
		}
	case '6':
		if has("Gb") {
			return 0.4
		}
	case '7':
		if has("T") {
			return 0.5
		}
	case '8':
		if has("B3") {
			return 0.4
		}
	case '9':
		if has("gq") {
			return 0.5
		}
	}
	return 1
}

// vizRankDistance e' la distanza di modifica pesata tra la lettura obs e la sigla canonica can. Operazioni: sostituzione
// (vizRankSubCost), carattere letto in piu' (0,7; 0,3 se e' punteggiatura), carattere canonico mancante (0,5 per la barra, 1 per gli altri) e un
// carattere letto per due canonici fusi (N per IV, 0,4).
func vizRankDistance(obs, can []rune) float64 {
	n, m := len(obs), len(can)
	d := make([][]float64, n+1)
	for i := range d {
		d[i] = make([]float64, m+1)
	}
	missing := func(c rune) float64 {
		if c == '/' {
			return 0.5
		}
		return 1
	}
	for j := 1; j <= m; j++ {
		d[0][j] = d[0][j-1] + missing(can[j-1])
	}
	extra := func(o rune) float64 { // carattere letto in piu': la punteggiatura spuria costa poco
		if strings.ContainsRune(".,'`’", o) {
			return 0.3
		}
		return 0.7
	}
	for i := 1; i <= n; i++ {
		d[i][0] = d[i-1][0] + extra(obs[i-1])
		for j := 1; j <= m; j++ {
			best := d[i-1][j-1] + vizRankSubCost(can[j-1], obs[i-1])
			if v := d[i-1][j] + extra(obs[i-1]); v < best {
				best = v
			}
			if v := d[i][j-1] + missing(can[j-1]); v < best {
				best = v
			}
			// V e I fusi in una N (o M/H per II): un carattere letto per due canonici
			if j >= 2 && can[j-2] == 'I' && can[j-1] == 'V' && (obs[i-1] == 'N' || obs[i-1] == 'n') {
				if v := d[i-1][j-2] + 0.4; v < best {
					best = v
				}
			}
			d[i][j] = best
		}
	}
	return d[n][m]
}

// vizRankMaxCost: costo massimo di decodifica perche' la lettura valga come sigla (una lettura ordinaria come "STAFF"
// costa oltre 3).
const vizRankMaxCost = 1.4

// vizDecodeRankCode decodifica una lettura della sigla di grado ("11/2" -> "II/2"). cost e' la distanza dalla sigla
// scelta; ok e' falso se nessuna sigla della grammatica e' abbastanza vicina o se la lettura non ha l'aspetto di una sigla
// (2-7 caratteri). A parita' di costo vince la sigla piu' corta (piu' frequente).
func vizDecodeRankCode(tok string) (code string, cost float64, ok bool) {
	obs := []rune(strings.TrimSpace(tok))
	if len(obs) < 2 || len(obs) > 7 {
		return "", 0, false
	}
	best, bc := "", math.MaxFloat64
	for _, c := range vizAllRankCodes() {
		if d := vizRankDistance(obs, []rune(c)); d < bc-1e-9 {
			best, bc = c, d
		}
	}
	if bc > vizRankMaxCost {
		return "", bc, false
	}
	return best, bc, true
}

// vizDecodeRankTail sostituisce l'ultima parola di una qualifica con la sigla canonica se somiglia a una sigla di grado.
// Restituisce il testo e se la sostituzione e' avvenuta (o se l'ultima parola era gia' una sigla canonica).
func vizDecodeRankTail(text string) (string, bool) {
	f := strings.Fields(text)
	if len(f) == 0 {
		return text, false
	}
	last, c1, ok1 := vizDecodeRankCode(f[len(f)-1])
	// la sigla puo' essere stata spezzata in due parole ("11 /2"): si prova a unire le ultime due, se costa meno
	if len(f) >= 2 {
		if merged, c2, ok2 := vizDecodeRankCode(f[len(f)-2] + f[len(f)-1]); ok2 && (!ok1 || c2 < c1) {
			return strings.Join(append(f[:len(f)-2:len(f)-2], merged), " "), true
		}
	}
	if ok1 {
		return strings.Join(append(f[:len(f)-1:len(f)-1], last), " "), true
	}
	return text, false
}
