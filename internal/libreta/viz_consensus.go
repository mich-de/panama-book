package libreta

// Consenso tra letture di testo di lunghezza diversa. Le varianti dello stesso campo sbagliano in punti diversi e spesso
// perdono o aggiungono un carattere o interi pezzi (una lettura troncata "MAS", una con la sigla storpiata "MASTER ITN",
// una con la prima lettera sbagliata "RASTER II/2"): il voto per carattere a lunghezza fissa non le mette insieme. Qui si
// sceglie un pivot (la lettura meglio sostenuta dalle altre), si allinea ogni lettura al pivot (allineamento semiglobale:
// i pezzi del pivot che una lettura troncata non copre non contano contro di lei) e per ogni posizione del pivot si
// vota il carattere (o l'assenza di carattere) tra tutte le letture che la coprono: come la tecnica ROVER del
// riconoscimento vocale.

import (
	"strings"
)

// vizAlignMinIdentity: identita' minima (caratteri uguali sulla regione coperta) perche' una lettura conti nel consenso.
const vizAlignMinIdentity = 0.55

// vizAlignment e' l'allineamento di una lettura al pivot: per ogni posizione del pivot l'indice del carattere della
// lettura (>= 0), vizGapIn (assente dentro la regione coperta) oppure vizOutside (fuori dalla regione coperta: nessuna informazione).
type vizAlignment struct {
	at       []int
	identity float64 // caratteri uguali / posizioni coperte
	covered  int     // posizioni del pivot coperte (dentro la regione)
}

const (
	vizGapIn   = -1
	vizOutside = -2
)

// vizAlign allinea c al pivot p massimizzando il punteggio (carattere uguale +1, diverso -1, inserzione/cancellazione
// -1); gli estremi sono liberi (il pivot puo' avere pezzi non coperti da c, e c puo' avere caratteri oltre il pivot).
func vizAlign(p, c []rune) vizAlignment {
	n, m := len(p), len(c)
	s := make([][]int, n+1)
	tb := make([][]byte, n+1)
	for i := range s {
		s[i] = make([]int, m+1)
		tb[i] = make([]byte, m+1)
	}
	const (
		stop byte = iota
		diag
		up   // carattere del pivot assente nella lettura
		left // carattere in piu' nella lettura
	)
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			sub := -1
			if p[i-1] == c[j-1] {
				sub = 1
			}
			best, bs := s[i-1][j-1]+sub, diag
			if v := s[i-1][j] - 1; v > best {
				best, bs = v, up
			}
			if v := s[i][j-1] - 1; v > best {
				best, bs = v, left
			}
			s[i][j], tb[i][j] = best, bs
		}
	}
	// fine libera: il punteggio massimo sull'ultima riga o sull'ultima colonna (a parita' la regione piu' lunga)
	bi, bj, bv := 0, 0, 0
	for i := 1; i <= n; i++ {
		if s[i][m] >= bv && s[i][m] > 0 {
			bi, bj, bv = i, m, s[i][m]
		}
	}
	for j := 1; j <= m; j++ {
		if s[n][j] > bv || (s[n][j] == bv && bv > 0 && j > bj) {
			bi, bj, bv = n, j, s[n][j]
		}
	}
	al := vizAlignment{at: make([]int, n)}
	for i := range al.at {
		al.at[i] = vizOutside
	}
	if bv <= 0 {
		return al
	}
	i, j := bi, bj
	same, cov := 0, 0
	for i > 0 && j > 0 {
		switch tb[i][j] {
		case diag:
			al.at[i-1] = j - 1
			if p[i-1] == c[j-1] {
				same++
			}
			cov++
			i--
			j--
		case up:
			al.at[i-1] = vizGapIn
			cov++
			i--
		case left:
			j--
		default:
			i, j = 0, 0
		}
	}
	al.covered = cov
	if cov > 0 {
		al.identity = float64(same) / float64(cov)
	}
	return al
}

// vizConsensus costruisce il testo di consenso. vals sono le letture (gia' normalizzate), ws i pesi. Restituisce il testo,
// se almeno una posizione e' contesa (il secondo carattere pesa almeno meta' del primo) e la confidenza in [0,1]: media,
// sulle posizioni non spazio, della quota di peso che sostiene il carattere scelto, per la quota di peso totale delle
// letture che hanno contribuito.
func vizConsensus(vals []string, ws []float64) (best string, contested bool, conf float64) {
	if len(vals) == 0 {
		return "", false, 0
	}
	runes := make([][]rune, len(vals))
	allW := 0.0
	for i, v := range vals {
		runes[i] = []rune(v)
		allW += ws[i]
	}
	// pivot: tra le letture ben sostenute dalle altre (almeno il 60% del sostegno massimo) la piu' lunga: porta piu'
	// informazione, e le letture troncate sono coerenti con lei. Il sostegno di una lettura e' il suo peso piu' quello
	// delle altre che concordano con lei nella regione che coprono.
	supports := make([]float64, len(vals))
	maxSupport := 0.0
	for i := range vals {
		sc := ws[i]
		for j := range vals {
			if i == j {
				continue
			}
			al := vizAlign(runes[i], runes[j])
			if al.covered >= vizMinCovered(len(runes[i]), len(runes[j])) && al.identity >= vizAlignMinIdentity {
				sc += ws[j] * al.identity
			}
		}
		supports[i] = sc
		if sc > maxSupport {
			maxSupport = sc
		}
	}
	pivot := -1
	for i := range vals {
		if supports[i] < 0.6*maxSupport {
			continue
		}
		if pivot < 0 || len(runes[i]) > len(runes[pivot]) || (len(runes[i]) == len(runes[pivot]) && supports[i] > supports[pivot]) {
			pivot = i
		}
	}
	p := runes[pivot]
	type votes struct {
		ch  map[rune]float64
		gap float64
	}
	vs := make([]votes, len(p))
	for i := range vs {
		vs[i].ch = map[rune]float64{}
	}
	used := 0.0
	for j := range vals {
		w := ws[j]
		if j == pivot {
			for i, c := range p {
				vs[i].ch[c] += w
			}
			used += w
			continue
		}
		al := vizAlign(p, runes[j])
		if al.covered < vizMinCovered(len(p), len(runes[j])) || al.identity < vizAlignMinIdentity {
			continue
		}
		used += w
		for i, a := range al.at {
			switch {
			case a >= 0:
				vs[i].ch[runes[j][a]] += w
			case a == vizGapIn:
				vs[i].gap += w
			}
		}
	}
	// le posizioni agli estremi coperte da poco peso (una sola lettura isolata) non contano: sono rumore in coda o in testa
	cover := make([]float64, len(vs))
	for i, v := range vs {
		cover[i] = v.gap
		for _, w := range v.ch {
			cover[i] += w
		}
	}
	minCover := max(vizMinCoverWeight, vizMinCoverFrac*used)
	first, last := 0, len(vs)-1
	for first < len(vs) && cover[first] < minCover {
		first++
	}
	for last >= first && cover[last] < minCover {
		last--
	}
	if first > last {
		return "", false, 0
	}
	vs = vs[first : last+1]
	var out []rune
	agreeSum, agreeN := 0.0, 0
	for _, v := range vs {
		var win rune
		ww := -1.0
		sum := v.gap
		for c, w := range v.ch {
			sum += w
			if w > ww+1e-9 || (w > ww-1e-9 && c < win) {
				win, ww = c, w
			}
		}
		// uno spazio letto dove un'altra lettura ha una lettera pesa poco (e' un glifo stretto perso, non uno spazio vero)
		if _, has := v.ch[' ']; has && len(v.ch) > 1 {
			adj := map[rune]float64{}
			for c, w := range v.ch {
				if c == ' ' {
					w *= vizSpaceWeight
				}
				adj[c] = w
			}
			win, ww = 0, -1
			for c, w := range adj {
				if w > ww+1e-9 || (w > ww-1e-9 && c < win) {
					win, ww = c, w
				}
			}
		}
		if v.gap > ww+1e-9 {
			contested = contested || v.gap >= vizContested*ww
			continue // piu' letture dicono che qui non c'e' nessun carattere
		}
		out = append(out, win)
		if win != ' ' && sum > 0 {
			agreeSum += ww / sum
			agreeN++
		}
		for c, w := range v.ch {
			if c != win && w >= vizContested*ww-1e-9 {
				contested = true
			}
		}
		if v.gap >= vizContested*ww-1e-9 && v.gap > 0 {
			contested = true
		}
	}
	if agreeN > 0 && allW > 0 {
		conf = agreeSum / float64(agreeN) * used / allW
	}
	return strings.Join(strings.Fields(string(out)), " "), contested, conf
}

// vizMinCoverWeight: peso minimo (circa due varianti) che deve coprire una posizione agli estremi del consenso.
const vizMinCoverWeight = 1.5

// vizMinCoverFrac: frazione minima del peso delle letture sovrapposte che deve coprire una posizione agli estremi.
const vizMinCoverFrac = 0.2

// vizMinCovered: posizioni minime del pivot che una lettura deve coprire per contare (3, e almeno un quinto della piu'
// corta tra pivot e lettura: una lettura di due caratteri allineata ovunque non prova nulla).
func vizMinCovered(pivotLen, readLen int) int {
	return max(3, min(pivotLen, readLen)*4/5)
}
