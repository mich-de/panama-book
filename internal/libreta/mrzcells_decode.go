package libreta

// Decodifica delle righe MRZ a partire dai punteggi dei simboli di ogni cella (vedi mrzcells.go e mrzcells_glyph.go).
//
// Le due righe hanno una struttura nota (Libreta de embarque di Panama, ICAO TD3). Invece di prendere il simbolo
// migliore di ogni cella si cerca la stringa piu' probabile che rispetta la struttura:
//   - riga 1: "IS" + "PAN" + COGNOME<<NOMI<<<<...: solo lettere e '<', con la grammatica del campo nomi;
//   - riga 2: campi con tipo fisso per posizione (lettere, cifre, sesso, date valide, nazionalita' ISO) e, soprattutto,
//     i cinque check digit ICAO come vincolo esatto. Un check a una cifra torna per caso 1 volta su 10, ma qui i
//     vincoli valgono tutti insieme (la probabilita' che una lettura sbagliata li rispetti per caso e' ~1e-5) e la
//     soluzione cercata e' la piu' probabile secondo il classificatore, non una qualunque.
//
// La ricerca e' una programmazione dinamica (Viterbi) sullo stato (somma del check composito, somma del check del
// campo): per la riga 2 sono 200 stati e ~30 passi. La stessa ricorsione con la somma al posto del massimo da' la
// probabilita' totale delle stringhe valide: il rapporto e' la probabilita' a posteriori della lettura scelta, che si
// usa come misura di affidabilita'.

import (
	"math"
	"strings"
)

// tplAlphabet: simboli riconosciuti nelle celle, nell'ordine usato dalle matrici di punteggi.
const tplAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789<"

const (
	tplNSym  = len(tplAlphabet) // 37
	tplFill  = tplNSym - 1      // indice di '<'
	tplDig0  = 26               // indice di '0'
	tplNegIn = -1e9             // "impossibile"
)

// tplLogp: logaritmo della probabilita' di ogni simbolo in ogni cella di una riga.
type tplLogp [tplCells][tplNSym]float64

func tplSym(c byte) int { return strings.IndexByte(tplAlphabet, c) }

func tplIsLetter(s int) bool { return s >= 0 && s < tplDig0 }
func tplIsDigit(s int) bool  { return s >= tplDig0 && s < tplFill }

// tplVal e' il valore ICAO del simbolo per i check digit: cifre 0-9, lettere 10-35, '<' 0.
func tplVal(s int) int {
	switch {
	case tplIsLetter(s):
		return 10 + s
	case tplIsDigit(s):
		return s - tplDig0
	}
	return 0
}

var tplWeights = [3]int{7, 3, 1}

// tplLogAdd restituisce log(exp(a)+exp(b)).
func tplLogAdd(a, b float64) float64 {
	if a < b {
		a, b = b, a
	}
	if b <= tplNegIn/2 {
		return a
	}
	return a + math.Log1p(math.Exp(b-a))
}

// tplDecoded e' l'esito della decodifica di una riga.
type tplDecoded struct {
	text string  // riga a 44 caratteri
	logp float64 // log della probabilita' (non normalizzata) della stringa scelta
	post float64 // probabilita' a posteriori della stringa scelta tra tutte quelle valide (0..1)
	ok   bool    // false se nessuna stringa rispetta la struttura (non dovrebbe capitare)
}

// ---------------------------------------------------------------------------------------------------------------
// Riga 1

// tplLine1Head: simboli attesi nelle prime 5 celle della riga 1 ("IS" + "PAN"). Il prior e' morbido: un simbolo
// diverso costa tplMismatchCost nel logaritmo, quindi lo vince solo un'evidenza molto forte.
const tplLine1Head = "ISPAN"

// tplMismatchCost e' la penalita' (log) per un simbolo diverso da quello atteso nelle prime 5 celle della riga 1.
const tplMismatchCost = 7.0

// tplDecodeLine1 trova la stringa di 44 caratteri piu' probabile con la grammatica della riga 1: "ISPAN", poi
// COGNOME (lettere, al piu' separate da un '<'), "<<" (o piu' '<'), NOMI (lettere separate da un '<') e '<' fino alla fine.
func tplDecodeLine1(lp *tplLogp) tplDecoded {
	// stati: 0 prime parole (lettere), 1 un '<' dopo di esse, 2 separatore "<<" o piu' lungo, 3 seconde parole
	// (lettere), 4 un '<' dopo di esse, 5 riempimento finale, 6 partenza (prima della prima lettera). Nei libretti di
	// Panama le parole prima del separatore sono i nomi o il cognome indifferentemente: la grammatica non lo distingue.
	const nState, start = 7, 6
	next := func(st, sym int) int { // -1 se la transizione non e' ammessa
		letter, fill := tplIsLetter(sym), sym == tplFill
		switch {
		case st == start && letter:
			return 0
		case (st == 0 || st == 1) && letter:
			return 0
		case st == 0 && fill:
			return 1
		case st == 1 && fill:
			return 2
		case (st == 2 || st == 3 || st == 4) && letter:
			return 3
		case st == 2 && fill:
			return 2 // separatore di tre o piu' '<' (visto in 6 libretti su 8) o riempimento senza nomi
		case st == 3 && fill:
			return 4
		case (st == 4 || st == 5) && fill:
			return 5
		}
		return -1
	}
	emit := func(k, sym int) float64 {
		c := lp[k][sym]
		if k < len(tplLine1Head) && sym != tplSym(tplLine1Head[k]) {
			c -= tplMismatchCost
		}
		return c
	}
	run := func(sum bool) (float64, string) {
		var cur, nxt [nState]float64
		var bp [tplCells][nState]int8  // stato precedente
		var bs [tplCells][nState]uint8 // simbolo scelto
		for i := range cur {
			cur[i] = tplNegIn
		}
		cur[start] = 0
		for k := 0; k < tplCells; k++ {
			for i := range nxt {
				nxt[i] = tplNegIn
			}
			for st := 0; st < nState; st++ {
				if cur[st] <= tplNegIn/2 {
					continue
				}
				for sym := 0; sym < tplNSym; sym++ {
					ns := next(st, sym)
					if ns < 0 {
						continue
					}
					v := cur[st] + emit(k, sym)
					if sum {
						nxt[ns] = tplLogAdd(nxt[ns], v)
					} else if v > nxt[ns] {
						nxt[ns], bp[k][ns], bs[k][ns] = v, int8(st), uint8(sym)
					}
				}
			}
			cur = nxt
		}
		bestSt, bestV := -1, tplNegIn
		for st := 0; st < start; st++ {
			if sum {
				bestV = tplLogAdd(bestV, cur[st])
			} else if cur[st] > bestV {
				bestSt, bestV = st, cur[st]
			}
		}
		if sum || bestSt < 0 {
			return bestV, ""
		}
		out := make([]byte, tplCells)
		st := bestSt
		for k := tplCells - 1; k >= 0; k-- {
			out[k] = tplAlphabet[bs[k][st]]
			st = int(bp[k][st])
		}
		return bestV, string(out)
	}
	best, text := run(false)
	if text == "" {
		return tplDecoded{}
	}
	z, _ := run(true)
	return tplDecoded{text: text, logp: best, post: math.Exp(best - z), ok: true}
}

// ---------------------------------------------------------------------------------------------------------------
// Riga 2

// tplItem e' un passo della ricerca sulla riga 2: una o piu' celle consecutive con le alternative ammesse.
type tplItem struct {
	pos   int      // prima cella
	n     int      // numero di celle (1, 2, 3)
	check int      // 0 = normale; 1 = cella di check di un campo (valore forzato dalla somma del campo); 2 = check composito
	opts  []tplOpt // alternative (vuoto per le celle di check)
}

// tplOpt e' una alternativa per un item: i simboli e il costo (log) nei due modi (massimo / somma).
type tplOpt struct {
	syms [3]int8
	cost float64 // log della verosimiglianza per il massimo (Viterbi)
	sum  float64 // log della verosimiglianza per la somma (probabilita' totale); di solito uguale a cost
	fill bool    // la cella e' '<'
}

// Posizioni dei campi della riga 2 (TD3).
const (
	tplDocEnd   = 9  // numero documento 0..8, check in 9
	tplNatPos   = 10 // nazionalita' 10..12
	tplBirthPos = 13 // nascita 13..18, check in 19
	tplSexPos   = 20
	tplExpPos   = 21 // scadenza 21..26, check in 27
	tplOptPos   = 28 // opzionale 28..41, check in 42
	tplCompPos  = 43
)

// tplFieldStart: prima cella del campo di cui fa parte la cella p (per i pesi 7-3-1 del check del campo), -1 se non
// fa parte di un campo con check.
func tplFieldStart(p int) int {
	switch {
	case p < tplDocEnd:
		return 0
	case p >= tplBirthPos && p < tplBirthPos+6:
		return tplBirthPos
	case p >= tplExpPos && p < tplExpPos+6:
		return tplExpPos
	case p >= tplOptPos && p < tplOptPos+14:
		return tplOptPos
	}
	return -1
}

// tplCompIndex: indice della cella p nella stringa del check composito (celle 0-9, 13-19, 21-42), -1 se non ne fa parte.
func tplCompIndex(p int) int {
	switch {
	case p < 10:
		return p
	case p >= 13 && p < 20:
		return 10 + p - 13
	case p >= 21 && p < 43:
		return 17 + p - 21
	}
	return -1
}

// tplDocPatterns: forma ammessa del numero libretto (celle 0-8): 'L' = lettera, 'D' = cifra, altro = quel carattere.
// Visti: "PA" + 7 cifre. Il prior e' morbido (tplPatternCost per ogni cella che non rispetta nessun modello).
var tplDocPatterns = []string{"PADDDDDDD", "PDDDDDDD<", "PDDDDDDDD", "LLDDDDDDD"}

// tplPatMatch dice se il simbolo s e' ammesso dal carattere di modello c: 'D' cifra, 'L' lettera, altro = esattamente
// quel carattere (anche '<').
func tplPatMatch(c byte, s int) bool {
	switch c {
	case 'D':
		return tplIsDigit(s)
	case 'L':
		return tplIsLetter(s)
	}
	return s == tplSym(c)
}

// tplPatternCost: penalita' (log) per ogni cella del numero libretto che non rispetta nessun modello.
const tplPatternCost = 4.0

// tplISO3 elenca i codici nazione ICAO a 3 lettere (ISO 3166-1 alpha-3 piu' i codici speciali ICAO): prior morbido
// sulla nazionalita' (tplNatCost se il codice non e' in lista).
var tplISO3 = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range strings.Fields(`ABW AFG AGO AIA ALA ALB AND ARE ARG ARM ASM ATA ATF ATG AUS AUT AZE BDI BEL BEN BES BFA
BGD BGR BHR BHS BIH BLM BLR BLZ BMU BOL BRA BRB BRN BTN BVT BWA CAF CAN CCK CHE CHL CHN CIV CMR COD COG COK COL COM CPV
CRI CUB CUW CXR CYM CYP CZE DEU DJI DMA DNK DOM DZA ECU EGY ERI ESH ESP EST ETH FIN FJI FLK FRA FRO FSM GAB GBR GEO GGY
GHA GIB GIN GLP GMB GNB GNQ GRC GRD GRL GTM GUF GUM GUY HKG HMD HND HRV HTI HUN IDN IMN IND IOT IRL IRN IRQ ISL ISR ITA
JAM JEY JOR JPN KAZ KEN KGZ KHM KIR KNA KOR KWT LAO LBN LBR LBY LCA LIE LKA LSO LTU LUX LVA MAC MAF MAR MCO MDA MDG MDV
MEX MHL MKD MLI MLT MMR MNE MNG MNP MOZ MRT MSR MTQ MUS MWI MYS MYT NAM NCL NER NFK NGA NIC NIU NLD NOR NPL NRU NZL OMN
PAK PAN PCN PER PHL PLW PNG POL PRI PRK PRT PRY PSE PYF QAT REU ROU RUS RWA SAU SDN SEN SGP SGS SHN SJM SLB SLE SLV SMR
SOM SPM SRB SSD STP SUR SVK SVN SWE SWZ SXM SYC SYR TCA TCD TGO THA TJK TKL TKM TLS TON TTO TUN TUR TUV TWN TZA UGA UKR
UMI URY USA UZB VAT VCT VEN VGB VIR VNM VUT WLF WSM YEM ZAF ZMB ZWE
GBD GBN GBO GBP GBS UNO UNA UNK XXA XXB XXC XXX EUE RKS ANT SCG YUG SUN DDR TMP ZAR`) {
		m[c] = true
	}
	return m
}()

// tplNatCost e' la penalita' (log) per una nazionalita' che non e' un codice ICAO noto.
const tplNatCost = 4.0

// tplDigitInOptCost e' la penalita' (log) per una cifra nel campo opzionale (il nome dell'ufficio regionale e' fatto
// di lettere).
const tplDigitInOptCost = 1.5

// tplBuildItems prepara i passi della riga 2 dai punteggi lp.
func tplBuildItems(lp *tplLogp) []tplItem {
	var items []tplItem
	single := func(p int, allow func(s int) (float64, bool)) {
		it := tplItem{pos: p, n: 1}
		for s := 0; s < tplNSym; s++ {
			if pen, ok := allow(s); ok {
				c := lp[p][s] + pen
				it.opts = append(it.opts, tplOpt{syms: [3]int8{int8(s)}, cost: c, sum: c, fill: s == tplFill})
			}
		}
		items = append(items, it)
	}
	digit := func(s int) (float64, bool) { return 0, tplIsDigit(s) }

	// numero libretto: ogni cella ha il prior del modello (tra tplDocPatterns) che costa meno; si approssima cella per
	// cella con il minimo tra i modelli (basta per dare un'inclinazione, il check decide)
	for p := 0; p < tplDocEnd; p++ {
		single(p, func(s int) (float64, bool) {
			for _, pat := range tplDocPatterns {
				if tplPatMatch(pat[p], s) {
					return 0, true
				}
			}
			return -tplPatternCost, true
		})
	}
	items = append(items, tplItem{pos: tplDocEnd, n: 1, check: 1})

	// nazionalita': tre lettere, con il prior della lista dei codici
	nat := tplItem{pos: tplNatPos, n: 3}
	natBest, natSum := tplNegIn, tplNegIn
	var natSyms [3]int8
	for a := 0; a < 26; a++ {
		for b := 0; b < 26; b++ {
			for c := 0; c < 26; c++ {
				v := lp[tplNatPos][a] + lp[tplNatPos+1][b] + lp[tplNatPos+2][c]
				if !tplISO3[string([]byte{tplAlphabet[a], tplAlphabet[b], tplAlphabet[c]})] {
					v -= tplNatCost
				}
				natSum = tplLogAdd(natSum, v)
				if v > natBest {
					natBest, natSyms = v, [3]int8{int8(a), int8(b), int8(c)}
				}
			}
		}
	}
	nat.opts = []tplOpt{{syms: natSyms, cost: natBest, sum: natSum}}
	items = append(items, nat)

	// date: anno (due cifre qualunque), mese 01-12 e giorno 01-31 come coppie
	date := func(start int) {
		single(start, digit)
		single(start+1, digit)
		for _, rng := range [][3]int{{start + 2, 1, 12}, {start + 4, 1, 31}} {
			it := tplItem{pos: rng[0], n: 2}
			for v := rng[1]; v <= rng[2]; v++ {
				a, b := tplDig0+v/10, tplDig0+v%10
				c := lp[rng[0]][a] + lp[rng[0]+1][b]
				it.opts = append(it.opts, tplOpt{syms: [3]int8{int8(a), int8(b)}, cost: c, sum: c})
			}
			items = append(items, it)
		}
	}
	date(tplBirthPos)
	items = append(items, tplItem{pos: tplBirthPos + 6, n: 1, check: 1})
	single(tplSexPos, func(s int) (float64, bool) {
		switch s {
		case tplSym('M'), tplSym('F'):
			return 0, true
		case tplFill:
			return -3, true
		}
		return 0, false
	})
	date(tplExpPos)
	items = append(items, tplItem{pos: tplExpPos + 6, n: 1, check: 1})
	for p := tplOptPos; p < tplOptPos+14; p++ {
		single(p, func(s int) (float64, bool) {
			if tplIsDigit(s) {
				return -tplDigitInOptCost, true
			}
			return 0, true
		})
	}
	items = append(items, tplItem{pos: tplOptPos + 14, n: 1, check: 1})
	items = append(items, tplItem{pos: tplCompPos, n: 1, check: 2})
	return items
}

// tplDecodeLine2 trova la stringa di 44 caratteri piu' probabile con la struttura della riga 2 e tutti i check digit validi.
func tplDecodeLine2(lp *tplLogp) tplDecoded {
	items := tplBuildItems(lp)
	// stato: (somma composito mod 10, somma campo mod 10, campo opzionale finora tutto '<'): 200 stati
	const nState = 200
	enc := func(comp, fld int, fill bool) int {
		s := comp*10 + fld
		if fill {
			s += 100
		}
		return s
	}
	run := func(sum bool) (float64, string) {
		cur := make([]float64, nState)
		nxt := make([]float64, nState)
		for i := range cur {
			cur[i] = tplNegIn
		}
		cur[enc(0, 0, true)] = 0
		bp := make([][]int16, len(items)) // stato precedente
		bo := make([][]int16, len(items)) // alternativa scelta (o simbolo del check)
		for i := range items {
			bp[i] = make([]int16, nState)
			bo[i] = make([]int16, nState)
		}
		for idx, it := range items {
			for i := range nxt {
				nxt[i] = tplNegIn
			}
			put := func(ns int, v float64, from, o int) {
				if sum {
					nxt[ns] = tplLogAdd(nxt[ns], v)
				} else if v > nxt[ns] {
					nxt[ns], bp[idx][ns], bo[idx][ns] = v, int16(from), int16(o)
				}
			}
			for st := 0; st < nState; st++ {
				if cur[st] <= tplNegIn/2 {
					continue
				}
				fill := st >= 100
				comp, fld := (st%100)/10, st%10
				switch it.check {
				case 0:
					for oi, o := range it.opts {
						dfld, dcomp := 0, 0
						for j := 0; j < it.n; j++ {
							p, v := it.pos+j, tplVal(int(o.syms[j]))
							if fs := tplFieldStart(p); fs >= 0 {
								dfld += tplWeights[(p-fs)%3] * v
							}
							if ci := tplCompIndex(p); ci >= 0 {
								dcomp += tplWeights[ci%3] * v
							}
						}
						nfill := fill
						if it.pos >= tplOptPos && it.pos < tplOptPos+14 {
							nfill = fill && o.fill
						}
						c := o.cost
						if sum {
							c = o.sum
						}
						put(enc((comp+dcomp)%10, (fld+dfld)%10, nfill), cur[st]+c, st, oi)
					}
				case 1: // check di un campo: il simbolo e' la cifra della somma del campo
					p := it.pos
					syms := []int{tplDig0 + fld}
					if p == tplOptPos+14 && fill && fld == 0 {
						syms = append(syms, tplFill) // campo opzionale tutto '<': il check puo' essere '<' (vale 0)
					}
					for _, s := range syms {
						dcomp := tplWeights[tplCompIndex(p)%3] * tplVal(s)
						// il campo successivo riparte da zero; il flag "tutto '<'" riparte vero
						put(enc((comp+dcomp)%10, 0, true), cur[st]+lp[p][s], st, s)
					}
				case 2: // check composito
					s := tplDig0 + comp
					put(enc(0, 0, true), cur[st]+lp[it.pos][s], st, s)
				}
			}
			cur, nxt = nxt, cur
		}
		bestSt, bestV := -1, tplNegIn
		for st := 0; st < nState; st++ {
			if sum {
				bestV = tplLogAdd(bestV, cur[st])
			} else if cur[st] > bestV {
				bestSt, bestV = st, cur[st]
			}
		}
		if sum || bestSt < 0 {
			return bestV, ""
		}
		out := make([]byte, tplCells)
		st := bestSt
		for idx := len(items) - 1; idx >= 0; idx-- {
			it := items[idx]
			o := int(bo[idx][st])
			if it.check != 0 {
				out[it.pos] = tplAlphabet[o]
			} else {
				for j := 0; j < it.n; j++ {
					out[it.pos+j] = tplAlphabet[it.opts[o].syms[j]]
				}
			}
			st = int(bp[idx][st])
		}
		return bestV, string(out)
	}
	best, text := run(false)
	if text == "" {
		return tplDecoded{}
	}
	z, _ := run(true)
	return tplDecoded{text: text, logp: best, post: math.Exp(best - z), ok: true}
}

// tplArgmax restituisce la stringa dei simboli piu' probabili cella per cella, senza vincoli.
func tplArgmax(lp *tplLogp) string {
	out := make([]byte, tplCells)
	for k := range lp {
		best := 0
		for s := 1; s < tplNSym; s++ {
			if lp[k][s] > lp[k][best] {
				best = s
			}
		}
		out[k] = tplAlphabet[best]
	}
	return string(out)
}
