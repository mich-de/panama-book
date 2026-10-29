package libreta

// Voto tra le varianti dei ritagli (modo foglio): ogni campo e' letto in molte varianti (scala, soglia) e ogni
// variante sbaglia in punti diversi. Qui si decide il valore finale.
//
//   - campi a formato fisso (cifre dell'id marittimo, del refrendo, del numero libretto, date, identificazione):
//     voto per POSIZIONE tra le letture della stessa forma, invece che sulla stringa intera: due letture con un errore
//     in punti diversi si correggono a vicenda (con il voto sulla stringa intera nessuna delle due vince);
//   - campi di testo: voto sulla stringa con sostegno delle letture parziali (inizio o fine della stessa frase) e
//     snap dei vocabolari chiusi (nazionalita', tipo, paese, suffisso dell'ufficio regionale);
//   - peso delle varianti: all'inizio uguale; finalizeVIZ, che conosce l'MRZ (verita' per le due date e il numero
//     libretto grazie ai check digit), assegna a ogni variante il peso (hit+1)/(prove+2) misurato su QUESTA scansione
//     e rivota: la qualita' di una variante dipende dalla stampa (a parita' di soglia una ne legge bene i campi
//     e l'altra no), quindi non e' una costante;
//   - i campi numerici con letture in contrasto (il secondo valore pesa almeno la meta' del primo in una posizione)
//     finiscono nell'elenco degli incerti, che finalizeVIZ riporta in Result.Warnings senza valori.

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"scanlibretto/internal/mrz"
)

// vizCand e' la lettura di un campo in una variante; id identifica la variante (vedi vizVariantID).
type vizCand struct {
	id int
	f  map[string]string
}

// vizReadings: letture per chiave di fascia, in ordine di priorita'.
type vizReadings map[string][]vizCand

// vizVariantID numera le varianti: ritaglio e foglio di ogni variante di base, poi le varianti aggiuntive.
func vizVariantID(vi int, crop bool) int {
	if crop {
		return 2 * vi
	}
	return 2*vi + 1
}

func vizExtraID(k int) int { return 2*len(vizVariants) + k }

// Letture dell'ultima extractVIZ, per il nuovo voto di finalizeVIZ (che conosce l'MRZ). Associate al Result per
// puntatore: un Result diverso (o gia' finalizzato) non le usa.
var (
	vizRdMu    sync.Mutex
	vizRdOwner *Result
	vizRd      vizReadings
)

func vizSetReadings(owner *Result, rd vizReadings) {
	vizRdMu.Lock()
	vizRdOwner, vizRd = owner, rd
	vizRdMu.Unlock()
}

// vizTakeReadings restituisce (e svuota) le letture, se appartengono a r.
func vizTakeReadings(r *Result) vizReadings {
	vizRdMu.Lock()
	defer vizRdMu.Unlock()
	if vizRdOwner != r {
		return nil
	}
	rd := vizRd
	vizRdOwner, vizRd = nil, nil
	return rd
}

// ---- voto per posizione ------------------------------------------------------------------------------------

// vizThinSupport: frazione minima delle varianti (a peso) che devono aver letto il campo perche' il valore non sia incerto.
const vizThinSupport = 0.25

// vizContested: il secondo valore di una posizione pesa almeno questa frazione del primo.
const vizContested = 0.5

// vizPosVote vota carattere per carattere tra letture della stessa lunghezza. Tra le lunghezze presenti vince
// quella con il peso totale maggiore (a parita' la prima). A parita' di peso in una posizione vince il carattere
// che compare per primo (le letture sono in ordine di priorita'). contested e' vero se in qualche posizione il
// secondo carattere pesa almeno vizContested volte il primo.
func vizPosVote(vals []string, ws []float64) (best string, contested bool) {
	return vizPosVoteX(vals, ws, false)
}

// vizSpaceWeight: peso di uno spazio letto dove un'altra lettura ha una lettera (nei testi): e' quasi sempre un glifo
// stretto perso dall'OCR ("MAURIT US" per "MAURITIUS"), non uno spazio vero; uno spazio vero e' in tutte le letture.
const vizSpaceWeight = 0.15

// vizPosVoteX come vizPosVote; con softSpace gli spazi pesano vizSpaceWeight nelle posizioni in cui compare anche una lettera.
func vizPosVoteX(vals []string, ws []float64, softSpace bool) (best string, contested bool) {
	best, contested, _ = vizPosVoteStats(vals, ws, softSpace)
	return best, contested
}

// vizPosVoteStats come vizPosVoteX; agree e' l'accordo medio per posizione (peso del carattere scelto sul peso totale
// della posizione, medio sulle posizioni non spazio): 1 = tutte le letture uguali.
func vizPosVoteStats(vals []string, ws []float64, softSpace bool) (best string, contested bool, agree float64) {
	byLen := map[int]float64{}
	var lens []int
	for i, v := range vals {
		n := len([]rune(v))
		if byLen[n] == 0 {
			lens = append(lens, n)
		}
		byLen[n] += ws[i]
	}
	if len(lens) == 0 {
		return "", false, 0
	}
	bl := lens[0]
	for _, n := range lens[1:] {
		if byLen[n] > byLen[bl]+1e-9 {
			bl = n
		}
	}
	var group [][]rune
	var gw []float64
	for i, v := range vals {
		if r := []rune(v); len(r) == bl {
			group = append(group, r)
			gw = append(gw, ws[i])
		}
	}
	out := make([]rune, bl)
	agreeSum, agreeN := 0.0, 0
	for p := 0; p < bl; p++ {
		tot := map[rune]float64{}
		var order []rune
		hasLetter := false
		for _, r := range group {
			if r[p] != ' ' {
				hasLetter = true
			}
		}
		for i, r := range group {
			if tot[r[p]] == 0 {
				order = append(order, r[p])
			}
			w := gw[i]
			if softSpace && r[p] == ' ' && hasLetter {
				w *= vizSpaceWeight
			}
			tot[r[p]] += w
		}
		win := order[0]
		for _, c := range order[1:] {
			if tot[c] > tot[win]+1e-9 {
				win = c
			}
		}
		out[p] = win
		sum := 0.0
		for _, c := range order {
			sum += tot[c]
			if c != win && tot[c] >= vizContested*tot[win]-1e-9 {
				contested = true
			}
		}
		if win != ' ' && sum > 0 {
			agreeSum += tot[win] / sum
			agreeN++
		}
	}
	if agreeN > 0 {
		agree = agreeSum / float64(agreeN)
	}
	return string(out), contested, agree
}

// ---- peso delle varianti -----------------------------------------------------------------------------------

// vizDigitsMatch conta le cifre uguali (stessa posizione, dalla fine) tra una lettura e il valore noto, e le cifre
// confrontate (quelle del valore noto).
func vizDigitsMatch(read, truth string) (hit, n int) {
	a, b := []rune(vizDigitsOnly(read)), []rune(vizDigitsOnly(truth))
	for k := 1; k <= len(b); k++ {
		n++
		if k <= len(a) && a[len(a)-k] == b[len(b)-k] {
			hit++
		}
	}
	return hit, n
}

func vizDigitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// vizMRZWeights calibra le varianti su questa scansione con i campi di cui l'MRZ da' la verita' (check digit
// validi): data di nascita, data di scadenza, numero libretto. Per ogni variante si contano le CIFRE lette giuste
// (accuratezza a, lisciata (hit+1)/(n+2)) e il peso e' il logaritmo del rapporto di verosimiglianza di una cifra
// giusta contro una delle altre nove, ln(9a/(1-a)): il peso ottimo del voto di maggioranza pesato per letture con
// errori indipendenti. Normalizzato a media 1 sulle varianti che hanno almeno una lettura. Senza MRZ restituisce
// nil (pesi uguali).
func vizMRZWeights(rd vizReadings, m *mrz.Data) map[int]float64 {
	if m == nil {
		return nil
	}
	hit, tot := map[int]int{}, map[int]int{}
	seen := map[int]bool{}
	for _, cs := range rd {
		for _, c := range cs {
			seen[c.id] = true
		}
	}
	date := func(key, mrzISO string, ok bool) {
		if !ok || len(mrzISO) < 8 {
			return
		}
		for _, c := range rd[key] {
			if iso := c.f[key+"_iso"]; len(iso) >= 8 {
				h, n := vizDigitsMatch(iso[len(iso)-8:], mrzISO[len(mrzISO)-8:])
				hit[c.id] += h
				tot[c.id] += n
			}
		}
	}
	date("data_nascita", m.BirthDate, m.Checks.Birth)
	date("data_scadenza", m.ExpiryDate, m.Checks.Expiry)
	if doc := strings.TrimRight(m.DocNumber, "<"); m.Checks.DocNumber && len(doc) >= 8 {
		last7 := doc[len(doc)-7:]
		for _, key := range []string{"numero_libretto", "libdig"} {
			for _, c := range rd[key] {
				if v := c.f[key]; v != "" {
					h, n := vizDigitsMatch(v, last7)
					hit[c.id] += h
					tot[c.id] += n
				}
			}
		}
	}
	w := map[int]float64{}
	sum := 0.0
	for id := range seen {
		if tot[id] == 0 {
			continue // nessuna lettura di un campo noto: peso neutro (1), non quello di una variante al 50%
		}
		a := float64(hit[id]+1) / float64(tot[id]+2)
		w[id] = max(0.05, math.Log(9*a/(1-a)))
		sum += w[id]
	}
	if sum == 0 {
		return nil
	}
	mean := sum / float64(len(w))
	for id := range w {
		w[id] /= mean
	}
	return w
}

// ---- voto sui campi ----------------------------------------------------------------------------------------

// vizVoteAll vota tutti i campi delle fasce a partire dalle letture, con i pesi per variante (nil = uguali), e
// scrive i valori in out.Fields. Restituisce le chiavi dei campi numerici con letture in contrasto.
func vizVoteAll(rd vizReadings, weights map[int]float64, out *Result) (uncertain []string) {
	w := func(id int) float64 {
		m := 1.0
		if id >= vizWordIDBase+vizWordIDAgree { // parola per parola concorde con un'altra elaborazione: due letture indipendenti
			m = vizWordWeight
		}
		if weights == nil {
			return m
		}
		if x, ok := weights[id]; ok {
			return x * m
		}
		return m
	}
	for _, b := range vizBands {
		cs := rd[b.key]
		switch b.key {
		case "libdig":
			continue
		case "tipo":
			vizVote(b, cs, w, out)
			vizVote(vizBand{key: "paese", kind: "short"}, cs, w, out)
			continue
		case "numero_libretto":
			if vizVoteLibretto(rd["numero_libretto"], rd["libdig"], w, out) {
				uncertain = append(uncertain, b.key)
			}
			continue
		}
		if vizVote(b, cs, w, out) {
			uncertain = append(uncertain, b.key)
		}
	}
	return uncertain
}

// vizVoteLibretto unisce le letture del numero libretto: il campo intero (se ha il formato giusto) e prefisso
// piu' sole cifre (le cifre in grassetto vanno lette senza prefisso). Voto per posizione tra letture di pari lunghezza.
func vizVoteLibretto(full, digits []vizCand, w func(int) float64, out *Result) (contested bool) {
	pre := map[string]float64{}
	for _, c := range full {
		if p := c.f["libpre"]; p != "" {
			pre[p] += w(c.id)
		}
	}
	prefix := ""
	for p, n := range pre {
		if n > pre[prefix] || (n == pre[prefix] && p > prefix) {
			prefix = p
		}
	}
	var vals []string
	var ws []float64
	if prefix != "" {
		for _, d := range digits {
			if v := d.f["libdig"]; v != "" {
				vals, ws = append(vals, prefix+v), append(ws, w(d.id))
			}
		}
	}
	for _, c := range full {
		if v := c.f["numero_libretto"]; v != "" {
			vals, ws = append(vals, v), append(ws, w(c.id))
		}
	}
	if len(vals) == 0 {
		return false
	}
	best, contested := vizPosVote(vals, ws)
	out.Fields["numero_libretto"] = best
	return contested
}

// vizCanonIdent e' vero per le forme valide dell'identificazione: tre gruppi di cifre separati da trattini
// (cedula panamense), 1-2 lettere + 7-8 cifre o un codice alfanumerico di 6-12 caratteri con almeno una cifra.
func vizCanonIdent(s string) bool {
	if m := reIdent.FindStringSubmatch(s); m != nil {
		return true
	}
	if a, ok := vizIdentLoose(s); ok && !strings.Contains(s, "-") && (len(a) >= 7 || strings.IndexFunc(a, unicode.IsLetter) >= 0) {
		return true // i soli 6 numeri senza trattini sono di solito una cedula storpiata
	}
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return false
	}
	for i, p := range parts {
		if p == "" || len(p) > []int{2, 4, 6}[i] {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// vizVote sceglie il valore di un campo tra le letture delle varianti; restituisce true se il campo e' numerico e
// le letture sono in contrasto (valore incerto).
func vizVote(b vizBand, cands []vizCand, w func(int) float64, out *Result) (contested bool) {
	type item struct {
		v   string
		c   map[string]string
		w   float64
		idx int
		h   float64 // altezza della riga letta (unita' di W), 0 se ignota
	}
	var items []item
	for i, c := range cands {
		v := c.f[b.key]
		if v == "" {
			continue
		}
		switch b.key {
		case "nacionalidad":
			// l'aggettivo di nazionalita' ha solo lettere: l'apostrofo e la barra letti al posto della I sono errori OCR
			v = vizSnapNationality(vizNatClean(v))
		case "tipo":
			// vocabolario chiuso: una lettura che non somiglia a "SI" e' rumore (il tipo sta su un ologramma)
			if v = vizSnapTo(v, "SI", 0.5, true); v != "SI" {
				continue
			}
		case "paese":
			// vocabolario chiuso (la libreta e' panamense): una lettura che non somiglia a "PAN" e' rumore
			if v = vizSnapTo(v, "PAN", 0.6, true); v != "PAN" {
				continue
			}
		case "regionale":
			v = vizSnapRegional(v)
		case "cargo":
			v = vizSnapRank(v)
		}
		hw, _ := strconv.ParseFloat(c.f["_hw"], 64)
		wt := w(c.id)
		if b.key != "cargo" && c.id >= vizWordIDBase+vizWordIDAgree {
			wt /= vizWordWeight // il peso doppio delle parole concordi vale solo per la qualifica
		}
		items = append(items, item{v, c.f, wt, i, hw})
	}
	if len(items) == 0 {
		return false
	}
	vals := make([]string, len(items))
	ws := make([]float64, len(items))
	hs := make([]float64, len(items))
	support, all := 0.0, 0.0
	for i, it := range items {
		vals[i], ws[i], hs[i] = it.v, it.w, it.h
		support += it.w
	}
	for _, c := range cands {
		all += w(c.id)
	}
	// letto da meno di un quarto delle varianti: valore poco sostenuto, quindi incerto anche senza letture in contrasto
	thin := support < vizThinSupport*all
	if thin {
		defer func() { contested = true }()
	}
	best := ""
	switch b.kind {
	case "num9":
		best, contested = vizPosVote(vals, ws)
	case "date":
		best, contested = vizPosVote(vals, ws)
		if d, ok := vizPickDate(vizDates(best), b.key); ok {
			vizSetDate(out, b.key, d)
			return contested
		}
		best, contested = vizWholeVote(b, vals, ws), false
	case "ident":
		var cv []string
		var cw []float64
		for i, v := range vals {
			if vizCanonIdent(v) {
				cv, cw = append(cv, v), append(cw, ws[i])
			}
		}
		if len(cv) > 0 {
			// stessa forma (stessi trattini) soltanto: il voto per posizione presuppone lo stesso allineamento
			best, contested = vizPosVote(cv, cw)
		} else {
			best = vizWholeVote(b, vals, ws)
		}
	case "text":
		var ok bool
		var conf float64
		if best, contested, ok, conf = vizTextVoteFull(vals, ws, hs, b.key == "cargo"); !ok {
			best, contested = vizWholeVote(b, vals, ws), false
			conf = 0.3 // nessun accordo su una lunghezza: lettura debole
		}
		out.Fields["_conf_"+b.key] = strconv.FormatFloat(conf, 'f', 3, 64)
	default:
		best = vizWholeVote(b, vals, ws)
	}
	out.Fields[b.key] = best
	if b.kind == "date" {
		if d, ok := vizPickDate(vizDates(best), b.key); ok {
			vizSetDate(out, b.key, d)
		}
	}
	return contested
}

// vizWholeVote: la stringa con il peso totale maggiore; a parita' la prima in ordine di priorita'. Per i campi di
// testo vedi vizPickLongest; senza accordo vince la lettura piu' vicina a tutte le altre.
func vizWholeVote(b vizBand, vals []string, ws []float64) string {
	tot := map[string]float64{}
	var order []string
	for i, v := range vals {
		if _, ok := tot[v]; !ok {
			order = append(order, v)
		}
		tot[v] += ws[i]
	}
	sort.SliceStable(order, func(i, j int) bool { return tot[order[i]] > tot[order[j]]+1e-9 })
	best := order[0]
	if b.kind == "text" {
		best = vizPickLongest(order, tot)
	}
	if b.kind == "text" && tot[best] < 1.5 && len(order) > 2 {
		// nessun accordo: la lettura piu' vicina a tutte le altre
		bestSum := -1.0
		for _, v := range order {
			sum := 0.0
			for _, o := range order {
				if o != v {
					sum += vizSim(norm(v), norm(o))
				}
			}
			if sum > bestSum+1e-9 {
				best, bestSum = v, sum
			}
		}
	}
	return best
}

// vizSnapRegional: l'ufficio regionale e' un nome di luogo seguito dal suffisso "DR" (direzione regionale); in grassetto
// il motore OCR legge quasi sempre la D come O ("OR") o come zero. Un ultimo gruppo di due caratteri che sia
// "OR"/"0R"/"DR" dopo un altro gruppo diventa "DR".
func vizSnapRegional(v string) string {
	f := strings.Fields(v)
	if len(f) < 2 {
		return v
	}
	switch strings.ToUpper(f[len(f)-1]) {
	case "OR", "0R", "DR", "DA", "OA":
		f[len(f)-1] = "DR"
	}
	return strings.Join(f, " ")
}

// vizTextVote vota i testi liberi (luogo di nascita, cargo...) carattere per carattere. Dopo aver tenuto il gruppo
// delle letture simili (vizTextCluster) si sceglie la lunghezza (in caratteri, spazi compresi) piu' lunga che abbia un
// sostegno reale (peso totale almeno 2 e almeno un quarto del peso di tutte le letture): una lettura piu' corta e'
// incompleta (l'OCR perde spesso la prima o l'ultima parola o un glifo), una piu' lunga e' quasi sempre rumore (la riga
// vicina). Tra le letture di quella lunghezza si sceglie, per ogni posizione, il carattere piu' pesato (Q/O, D/O, U/V...
// sbagliati solo in una parte delle varianti si correggono); l'apostrofo dentro una parola e' una I persa e uno spazio
// dove un'altra lettura ha una lettera pesa poco (vizSpaceWeight). contested: in qualche posizione il secondo carattere
// pesa almeno meta' del primo. ok=false se nessuna lunghezza ha sostegno (si torna al voto sulla stringa intera).
func vizTextVote(vals []string, ws []float64) (best string, contested, ok bool) {
	return vizTextVoteH(vals, ws, nil)
}

// vizTextVoteH come vizTextVote con l'altezza (in unita' di W) della riga di ogni lettura; hs puo' essere nil.
func vizTextVoteH(vals []string, ws, hs []float64) (best string, contested, ok bool) {
	best, contested, ok, _ = vizTextVoteFull(vals, ws, hs, true)
	return best, contested, ok
}

// vizTextVoteFull come vizTextVoteH e restituisce anche la confidenza in [0,1]: la frazione del peso di tutte le
// letture che sostiene il testo scelto (stessa lunghezza, gruppo simile) per l'accordo medio per carattere.
func vizTextVoteFull(vals []string, ws, hs []float64, cons bool) (best string, contested, ok bool, conf float64) {
	allW := 0.0
	for _, w := range ws {
		allW += w
	}
	vals, ws = vizTextCluster(vals, ws, hs, cons)
	norms := make([]string, len(vals))
	groupW := 0.0
	for i, v := range vals {
		norms[i] = vizInnerApostrophe(v)
		groupW += ws[i]
	}
	if cons {
		best, contested, conf = vizConsensus(norms, ws)
		if best == "" {
			return "", false, false, 0
		}
		if allW > 0 {
			conf *= groupW / allW
		}
		return best, contested, true, conf
	}
	// voto per lunghezza (campi diversi dalla qualifica): tra le letture della lunghezza con sostegno si vota per posizione
	total := 0.0
	byLen := map[int]float64{}
	for i := range norms {
		total += ws[i]
		byLen[len([]rune(norms[i]))] += ws[i]
	}
	n := 0
	for k, w := range byLen {
		if k > n && w >= 2 && w >= total/4 {
			n = k
		}
	}
	if n == 0 {
		return "", false, false, 0
	}
	var gv []string
	var gw []float64
	gsum := 0.0
	for i, v := range norms {
		if len([]rune(v)) == n {
			gv, gw = append(gv, v), append(gw, ws[i])
			gsum += ws[i]
		}
	}
	best, contested, agree := vizPosVoteStats(gv, gw, true)
	if allW > 0 {
		conf = gsum / allW * agree
	}
	return best, contested, true, conf
}

// vizInnerApostrophe sostituisce con I l'apostrofo dentro una parola (dal terzo carattere in poi: "D'" e "L'" iniziali
// sono apostrofi veri): il motore OCR legge la I stretta in grassetto come apostrofo.
func vizInnerApostrophe(v string) string {
	r := []rune(v)
	idx := 0
	for i, c := range r {
		switch {
		case c == ' ':
			idx = 0
			continue
		case c == '\'' && idx >= 2 && i+1 < len(r) && r[i+1] != ' ':
			r[i] = 'I'
		}
		idx++
	}
	return string(r)
}

// vizClusterSim: somiglianza minima (vizSim sul testo normalizzato) perche' due letture siano della stessa parola.
const vizClusterSim = 0.45

// vizRelation misura quanto due letture sono varianti dello stesso testo, in (0,1] (0 = non correlate): la somiglianza
// (vizSim sul testo normalizzato) se e' almeno vizClusterSim; 0,7 se una e' l'inizio o la fine dell'altra (lettura
// troncata: almeno 3 caratteri, oppure parole intere).
func vizRelation(a, b string) float64 {
	na, nb := norm(a), norm(b)
	if s := vizSim(na, nb); s >= vizClusterSim {
		return s
	}
	short, long := na, nb
	if len(short) > len(long) {
		short, long = long, short
	}
	if len(short) >= 3 && len(short) < len(long) && (strings.HasPrefix(long, short) || strings.HasSuffix(long, short)) {
		return 0.7
	}
	x, y := strings.Fields(a), strings.Fields(b)
	if len(x) > len(y) {
		x, y = y, x
	}
	if len(x) == 0 || len(x) == len(y) {
		return 0
	}
	pre, suf := true, true
	for i := range x {
		if x[i] != y[i] {
			pre = false
		}
		if x[i] != y[len(y)-len(x)+i] {
			suf = false
		}
	}
	if pre || suf {
		return 0.7
	}
	return 0
}

// vizRelated: le due letture sono varianti dello stesso testo (vedi vizRelation).
func vizRelated(a, b string) bool { return vizRelation(a, b) > 0 }

// vizTextCluster tiene solo le letture del gruppo di letture simili che pesa di piu': le letture di rumore (resti
// dell'etichetta, riga vicina) sono tutte diverse tra loro e non formano un gruppo, mentre le letture buone del
// valore, anche con un carattere storpiato, si sostengono a vicenda. Eccezione: il valore e' in grassetto e molto piu'
// alto dei resti dell'etichetta (che di solito producono piu' letture, ma basse); se un altro gruppo ha righe alte
// almeno vizTallRatio volte quelle del gruppo piu' pesante e un peso di almeno la meta', vince quello. Con altezze
// ignote (hs nil) conta solo il peso.
func vizTextCluster(vals []string, ws, hs []float64, soft bool) ([]string, []float64) {
	if len(vals) < 3 {
		return vals, ws
	}
	type group struct {
		center int
		sup, h float64
	}
	var gs []group
	for i, v := range vals {
		g := group{center: i}
		hn := 0
		for j, o := range vals {
			rel := 1.0
			if i != j {
				rel = vizRelation(v, o)
				if !soft && rel > 0 {
					rel = 1 // senza consenso conta solo se sono correlate
				}
			}
			if rel > 0 {
				g.sup += ws[j] * rel // sostegno pesato dalla somiglianza: un gruppo coerente batte uno di letture a stento simili
				if hs != nil && hs[j] > 0 {
					g.h += hs[j]
					hn++
				}
			}
		}
		if hn > 0 {
			g.h /= float64(hn)
		}
		gs = append(gs, g)
	}
	best := gs[0]
	for _, g := range gs[1:] {
		if g.sup > best.sup+1e-9 {
			best = g
		}
	}
	if best.h > 0 {
		alt := best
		for _, g := range gs {
			if g.h >= vizTallRatio*best.h && g.sup >= 0.5*best.sup && (alt == best || g.sup > alt.sup+1e-9) {
				alt = g
			}
		}
		best = alt
	}
	var cv []string
	var cw []float64
	for j, o := range vals {
		if j == best.center || vizRelated(vals[best.center], o) {
			cv, cw = append(cv, o), append(cw, ws[j])
		}
	}
	return cv, cw
}

// vizTallRatio: rapporto minimo tra le altezze di righe di due gruppi di letture perche' il piu' alto si preferisca.
const vizTallRatio = 1.25

// vizRanks: qualifiche di bordo (capacity) piu' comuni, in inglese come stampate. L'elenco NON e' chiuso (esistono
// qualifiche di albergo, ristorazione, intrattenimento che cambiano da compagnia a compagnia): serve solo a correggere
// confusioni di lettere (D/O...) che il voto tra le varianti non risolve quando la maggioranza sbaglia allo stesso modo.
// Una lettura vale per una qualifica solo se e' quasi identica (>= 0,9); un testo non riconosciuto resta com'e'.
var vizRanks = []string{
	"MASTER", "CHIEF MATE", "CHIEF OFFICER", "FIRST MATE", "SECOND OFFICER", "SECOND MATE", "THIRD OFFICER", "THIRD MATE",
	"DECK CADET", "DECK OFFICER", "OFFICER IN CHARGE OF A NAVIGATIONAL WATCH", "ABLE SEAMAN", "ORDINARY SEAMAN", "ABLE SEAFARER DECK",
	"BOSUN", "BOATSWAIN", "DECK FITTER", "RATING FORMING PART OF A NAVIGATIONAL WATCH", "CHIEF ENGINEER", "SECOND ENGINEER",
	"THIRD ENGINEER", "FOURTH ENGINEER", "ENGINE CADET", "ENGINEER OFFICER", "ELECTRO-TECHNICAL OFFICER", "ELECTRICIAN",
	"MOTORMAN", "OILER", "WIPER", "FITTER", "PUMPMAN", "RADIO OFFICER", "COOK", "CHIEF COOK", "STEWARD", "MESSMAN",
}

// vizSnapRank riporta una qualifica storpiata dall'OCR a quella nota quasi identica; altrimenti la lascia com'e'.
func vizSnapRank(v string) string {
	n := norm(v)
	if n == "" {
		return v
	}
	best, bestS := "", 0.0
	for _, r := range vizRanks {
		if s := vizSim(n, norm(r)); s > bestS {
			best, bestS = r, s
		}
	}
	if bestS >= 0.9 {
		return best
	}
	return v
}
