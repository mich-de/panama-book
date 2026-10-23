package verify

import (
	"fmt"
	"strings"

	"scanlibretto/internal/outsum"
)

// Status e' l'esito del confronto di un campo con la verita'.
type Status int

const (
	// StatusOK: letto e uguale alla verita'.
	StatusOK Status = iota
	// StatusWrong: letto ma diverso dalla verita'. E' il caso che "campi 14/14" non vede.
	StatusWrong
	// StatusMissing: la verita' c'e', il programma non ha letto nulla.
	StatusMissing
	// StatusNotPrinted: la verita' e' vuota (campo non stampato, fuori dall'immagine o illeggibile) e il programma
	// non ha letto nulla: niente da confrontare.
	StatusNotPrinted
	// StatusExtra: la verita' e' vuota ma il programma ha letto un valore. Non e' un errore sicuro (puo' aver letto
	// bene un campo che l'occhio non distingue), e' un dato da guardare, specie dove il campo non c'e' davvero.
	StatusExtra
)

// String e' l'etichetta stampata nei report.
func (s Status) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusWrong:
		return "SBAGLIATO"
	case StatusMissing:
		return "MANCANTE"
	case StatusNotPrinted:
		return "NON_STAMPATO"
	case StatusExtra:
		return "IN_PIU"
	}
	return "?"
}

// MRZFix dice se l'MRZ avrebbe dato il valore giusto per un campo sbagliato o mancante.
type MRZFix int

const (
	// MRZNA: non applicabile. Il campo non e' nell'MRZ (luogo di nascita, data di emissione, livello, cargo, numeri
	// d'identificazione).
	MRZNA MRZFix = iota
	// MRZFixes: il campo e' nell'MRZ, l'MRZ c'e' ed e' valido per quel campo, e il suo valore coincide con la verita'.
	MRZFixes
	// MRZNoFix: l'MRZ c'e' ma da solo non darebbe il valore giusto (e' solo un prefisso, o e' diverso).
	MRZNoFix
	// MRZAbsent: il campo e' nell'MRZ ma l'MRZ non c'e' nell'uscita (o il check digit che copre il campo non torna):
	// non si sa cosa avrebbe dato; con un MRZ letto bene il campo sarebbe recuperabile.
	MRZAbsent
)

// FieldResult e' l'esito di un campo. Non contiene valori.
type FieldResult struct {
	Field  string
	Status Status
	Class  ErrorClass // solo per StatusWrong
	MRZ    MRZFix     // solo per StatusWrong e StatusMissing
}

// Compare confronta i campi letti da libretto con la verita'. Restituisce 14 esiti, nell'ordine di outsum.FieldKeys.
// sum e' il riassunto dell'uscita JSON (outsum.Parse): i campi letti, l'MRZ e i suoi check digit.
func Compare(sum outsum.Summary, truth Truth) []FieldResult {
	ctx := newDiffContext(sum.Fields, truth)
	out := make([]FieldResult, 0, outsum.NFields)
	for _, f := range outsum.FieldKeys {
		// lookup accetta il vecchio nome `endorsement` di `certificate`, sia nell'uscita sia nella verita'
		got, want := lookup(sum.Fields, f), lookup(truth.Fields, f)
		r := FieldResult{Field: f}
		switch {
		case want == "" && got == "":
			r.Status = StatusNotPrinted
		case want == "":
			r.Status = StatusExtra
		case got == "":
			r.Status = StatusMissing
			r.MRZ = mrzFix(sum, f, want)
		case Equal(f, got, want):
			r.Status = StatusOK
		default:
			r.Status = StatusWrong
			r.Class = classify(f, got, want, ctx)
			r.MRZ = mrzFix(sum, f, want)
		}
		out = append(out, r)
	}
	return out
}

// Counts riassume gli esiti di un confronto.
type Counts struct {
	OK, Wrong, Missing, NotPrinted, Extra int
}

// WithTruth e' il numero di campi che la verita' dice stampati e leggibili: il denominatore dell'accuratezza.
func (c Counts) WithTruth() int { return c.OK + c.Wrong + c.Missing }

// Count conta gli esiti.
func Count(rs []FieldResult) Counts {
	var c Counts
	for _, r := range rs {
		switch r.Status {
		case StatusOK:
			c.OK++
		case StatusWrong:
			c.Wrong++
		case StatusMissing:
			c.Missing++
		case StatusNotPrinted:
			c.NotPrinted++
		case StatusExtra:
			c.Extra++
		}
	}
	return c
}

// ---- classi di errore (senza valori) ----

// ErrKind e' la famiglia dell'errore di un campo sbagliato. L'insieme e' chiuso e non rivela nessun valore.
type ErrKind string

const (
	ErrDate       ErrKind = "data diversa"
	ErrSubst      ErrKind = "caratteri sostituiti"
	ErrSwap       ErrKind = "due caratteri scambiati"
	ErrTrunc      ErrKind = "troncato"
	ErrTail       ErrKind = "coda in piu'"
	ErrLength     ErrKind = "lunghezza diversa"
	ErrUnrelated  ErrKind = "testo non correlato"
	ErrNameRow    ErrKind = "testo della riga del nome"
	ErrOtherField ErrKind = "valore di un altro campo"
)

// ErrorClass descrive un campo sbagliato senza dire quali valori: la famiglia e un dettaglio fatto di conteggi
// ("3 caratteri sostituiti", "1 parola invece di 2").
type ErrorClass struct {
	Kind   ErrKind
	Detail string
}

// String e' "famiglia" oppure "famiglia: dettaglio".
func (e ErrorClass) String() string {
	if e.Detail == "" {
		return string(e.Kind)
	}
	return string(e.Kind) + ": " + e.Detail
}

// diffContext porta a classify cio' che serve per riconoscere un valore preso dal posto sbagliato.
type diffContext struct {
	nameTokens []string          // parole della riga del nome (dall'MRZ), normalizzate, di almeno 4 lettere
	otherTruth map[string]string // verita' degli altri campi, per riconoscere il valore di un altro campo
}

// newDiffContext raccoglie le parole della riga del nome da due fonti: l'MRZ letto dal programma (surname e
// given_names) e la riga stampata dichiarata nella verita' (NameRow), che c'e' anche quando l'MRZ non si e' letto.
func newDiffContext(fields map[string]string, truth Truth) *diffContext {
	ctx := &diffContext{otherTruth: truth.Fields}
	for _, text := range []string{fields["surname"], fields["given_names"], truth.NameRow} {
		for _, w := range strings.Fields(text) {
			if n := Norm(w); len(n) >= 4 {
				ctx.nameTokens = append(ctx.nameTokens, n)
			}
		}
	}
	return ctx
}

// hasNameWord dice se il testo contiene una parola della riga del nome, anche storpiata dall'OCR (al piu' un errore
// ogni quattro lettere) o attaccata ad altre. tokens sono parole normalizzate di almeno 4 lettere.
func hasNameWord(text string, tokens []string) bool {
	norm := Norm(text)
	for _, t := range tokens {
		if strings.Contains(norm, t) {
			return true
		}
	}
	for _, w := range strings.Fields(text) {
		nw := Norm(w)
		if len(nw) < 4 {
			continue
		}
		for _, t := range tokens {
			sub, ins, del := editOps([]rune(nw), []rune(t))
			if d := sub + ins + del; d <= len(t)/4 && abs(len(nw)-len(t)) <= 1 {
				return true
			}
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Diff classifica la differenza tra il valore letto e la verita' senza mostrarli. got e want devono essere
// diversi secondo Equal (altrimenti Kind e' vuoto).
func Diff(field, got, want string) ErrorClass { return classify(field, got, want, nil) }

func classify(field, got, want string, ctx *diffContext) ErrorClass {
	if Equal(field, got, want) {
		return ErrorClass{}
	}
	if isDateField(field) {
		return diffDate(got, want)
	}
	g, w := Norm(got), Norm(want)
	if ctx != nil {
		if hasNameWord(got, ctx.nameTokens) && !hasNameWord(want, ctx.nameTokens) {
			return ErrorClass{Kind: ErrNameRow}
		}
		for other, v := range ctx.otherTruth {
			if Canonical(other) != Canonical(field) && len(Norm(v)) >= 3 && Equal(other, got, v) {
				return ErrorClass{Kind: ErrOtherField, Detail: Canonical(other)}
			}
		}
	}
	if g == "" {
		return ErrorClass{Kind: ErrUnrelated, Detail: "nessuna lettera o cifra"}
	}
	ga, wa := []rune(g), []rune(w)
	sub, ins, del := editOps(ga, wa)
	dist := sub + ins + del
	words := wordsDetail(got, want)
	switch {
	case len(ga) < len(wa) && strings.HasPrefix(w, g):
		return ErrorClass{Kind: ErrTrunc, Detail: join(fmt.Sprintf("%d caratteri invece di %d", len(ga), len(wa)), words)}
	case len(ga) > len(wa) && strings.HasPrefix(g, w):
		return ErrorClass{Kind: ErrTail, Detail: join(plural(len(ga)-len(wa), "carattere in piu'", "caratteri in piu'"), words)}
	case len(ga) == len(wa) && isSwap(ga, wa):
		return ErrorClass{Kind: ErrSwap}
	case dist*10 > max(len(ga), len(wa))*6: // piu' del 60% da cambiare: non e' una lettura storpiata, e' un altro testo
		return ErrorClass{Kind: ErrUnrelated, Detail: join(fmt.Sprintf("%d caratteri invece di %d", len(ga), len(wa)), words)}
	case len(ga) == len(wa):
		return ErrorClass{Kind: ErrSubst, Detail: join(plural(sub, "carattere sostituito", "caratteri sostituiti"), words)}
	}
	return ErrorClass{Kind: ErrLength, Detail: join(fmt.Sprintf("%d caratteri invece di %d, %d differenze", len(ga), len(wa), dist), words)}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// join mette insieme i dettagli non vuoti.
func join(parts ...string) string {
	var keep []string
	for _, p := range parts {
		if p != "" {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, ", ")
}

// wordsDetail dice quante parole ha il valore letto e quante la verita', se sono diverse.
func wordsDetail(got, want string) string {
	g, w := len(strings.Fields(got)), len(strings.Fields(want))
	if g == w {
		return ""
	}
	return fmt.Sprintf("%s invece di %d", plural(g, "parola", "parole"), w)
}

// isSwap e' vero se a e b (stessa lunghezza) differiscono solo per due caratteri adiacenti scambiati.
func isSwap(a, b []rune) bool {
	var diff []int
	for i := range a {
		if a[i] != b[i] {
			diff = append(diff, i)
			if len(diff) > 2 {
				return false
			}
		}
	}
	return len(diff) == 2 && diff[1] == diff[0]+1 && a[diff[0]] == b[diff[1]] && a[diff[1]] == b[diff[0]]
}

// editOps conta le operazioni minime (distanza di Levenshtein) per trasformare a in b: sostituzioni, inserimenti (a
// ha caratteri in meno) e cancellazioni (a ha caratteri in piu').
func editOps(a, b []rune) (sub, ins, del int) {
	n, m := len(a), len(b)
	d := make([][]int, n+1)
	for i := range d {
		d[i] = make([]int, m+1)
		d[i][0] = i
	}
	for j := 1; j <= m; j++ {
		d[0][j] = j
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j-1]+cost, d[i-1][j]+1, d[i][j-1]+1)
		}
	}
	i, j := n, m
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1] && d[i][j] == d[i-1][j-1]:
			i, j = i-1, j-1
		case i > 0 && j > 0 && d[i][j] == d[i-1][j-1]+1:
			sub++
			i, j = i-1, j-1
		case i > 0 && d[i][j] == d[i-1][j]+1:
			del++
			i--
		default:
			ins++
			j--
		}
	}
	return sub, ins, del
}

// diffDate dice quali parti di una data sono diverse (giorno, mese, anno) senza mostrarle.
func diffDate(got, want string) ErrorClass {
	g, gok := parseDate(got)
	w, wok := parseDate(want)
	if !gok {
		return ErrorClass{Kind: ErrDate, Detail: "non e' una data valida"}
	}
	if !wok {
		return ErrorClass{Kind: ErrDate, Detail: "verita' non interpretabile"}
	}
	var parts []string
	if g.d != w.d {
		parts = append(parts, "giorno")
	}
	if g.m != w.m {
		parts = append(parts, "mese")
	}
	if g.y != w.y {
		parts = append(parts, "anno")
	}
	if g.d == w.m && g.m == w.d && len(parts) > 0 && g.y == w.y {
		return ErrorClass{Kind: ErrDate, Detail: "giorno e mese scambiati"}
	}
	return ErrorClass{Kind: ErrDate, Detail: "diversi: " + strings.Join(parts, ", ")}
}

// ---- correzione con l'MRZ ----

// mrzFix dice se il valore che l'MRZ da' per il campo sarebbe quello giusto. L'MRZ contiene la nascita, la scadenza,
// il numero libretto, la nazionalita' (in codice ISO) e un campo opzionale che comincia come il regionale; la
// riga 1 (IS PAN) da' il tipo (SI) e il paese (PAN), che sono costanti per i libretti di Panama.
func mrzFix(sum outsum.Summary, field, want string) MRZFix {
	var cands []string
	var ok bool // l'MRZ c'e' e il check digit che copre il campo e' valido
	switch field {
	case "type":
		cands, ok = []string{"SI"}, sum.HasMRZ
	case "country_code":
		cands, ok = []string{"PAN"}, sum.HasMRZ
	case "date_of_birth":
		cands, ok = one(sum.Fields["date_of_birth_mrz"]), sum.HasMRZ && sum.Checks[1]
	case "date_of_expiry":
		cands, ok = one(sum.Fields["date_of_expiry_mrz"]), sum.HasMRZ && sum.Checks[2]
	case "seamans_book":
		cands, ok = one(sum.Fields["seamans_book_mrz"]), sum.HasMRZ && sum.Checks[0]
	case "nationality":
		// il codice nazionalita' e' coperto solo dal check digit finale
		cands, ok = nationalityAdjectives[strings.ToUpper(sum.Fields["nationality_iso"])], sum.HasMRZ && sum.Checks[4]
	case "regional":
		cands, ok = one(sum.Fields["regional_mrz"]), sum.HasMRZ && sum.Checks[3]
	default:
		return MRZNA
	}
	if !ok || len(cands) == 0 {
		return MRZAbsent
	}
	for _, c := range cands {
		if Equal(field, c, want) {
			return MRZFixes
		}
	}
	return MRZNoFix
}
func one(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}
