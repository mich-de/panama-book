// Package certif legge i certificati dell'Autorita' Marittima di Panama che accompagnano il libretto:
//
//	SAT  Security Awareness Training            (certificato di endoso di corso, STCW A-VI/6-1)
//	SD   Security Duties (special duty)         (certificato di endoso di corso, STCW A-VI/6-2)
//	SSO  Ship Security Officer                  (certificato di endoso di corso, STCW A-VI/5)
//	CT   Certificacion de Tramite de Titulo     (ricevuta della domanda: serve per ottenere il Panama Book (libretto) definitivo;
//	                                             nel frattempo vale per lavorare a bordo per al massimo 3 mesi)
//
// Si parte dal testo in righe (una riga = una riga fisica del foglio, vedi RowsFromOCR) e si ancora ogni campo alla sua etichetta
// bilingue ES/EN, come fa internal/libreta per la pagina dati del libretto. Le date escono nella forma GG-MM-AAAA (come il resto del
// programma), i numeri del documento nella forma E-NNNNNNNNNA. Nessun campo viene inventato: se non si trova resta vuoto e finisce
// in Result.Missing, cosi' l'operatore lo vede e lo completa a mano.
package certif

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"scanlibretto/internal/verify"
)

// Tipi di certificato.
const (
	SAT = "SAT"
	SD  = "SD"
	SSO = "SSO"
	CT  = "CT"
)

// Chiavi dei campi (le stesse del libretto dove esistono).
const (
	FDocNumber     = "document_number"
	FName          = "name"
	FBirth         = "date_of_birth"
	FNationality   = "nationality"
	FNationalityES = "nationality_es"
	FIssue         = "date_of_issue"
	FExpiry        = "date_of_expiry"
	FMarineID      = "marine_id"
	FCTNumber      = "ct_number"
	FIdentity      = "identification"
	FCapacity      = "capacity"
	FCapacityES    = "capacity_es"
	FConsulate     = "consulate"
	FReceipt       = "receipt_number"
	FCourse        = "course"
)

// Required sono i campi che servono per ogni tipo (l'elenco chiesto dall'utente); gli altri si leggono comunque se ci sono.
var Required = map[string][]string{
	SAT: {FDocNumber, FExpiry, FIssue, FBirth},
	CT:  {FCTNumber, FDocNumber, FName, FIdentity, FCapacity, FIssue, FExpiry, FConsulate, FMarineID},
	SSO: {FDocNumber, FName, FBirth, FNationality, FIssue, FExpiry},
	SD:  {FDocNumber, FExpiry, FName, FBirth, FNationality, FIssue, FMarineID},
}

// Result e' l'esito della lettura di un certificato.
type Result struct {
	Type     string            `json:"type"`
	Fields   map[string]string `json:"fields"`
	Missing  []string          `json:"missing,omitempty"`  // campi richiesti dal tipo ma non letti
	Warnings []string          `json:"warnings,omitempty"` // incoerenze (date, formati)
}

// fold porta il testo in MAIUSCOLO ASCII, una lettera per lettera (gli indici restano gli stessi): serve a cercare le etichette
// senza dipendere da accenti e da come l'OCR rende le lettere accentate.
func fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case r < 128:
			b.WriteRune(r)
		case strings.ContainsRune("ÁÀÂÄÃ", r):
			b.WriteByte('A')
		case strings.ContainsRune("ÉÈÊË", r):
			b.WriteByte('E')
		case strings.ContainsRune("ÍÌÎÏ", r):
			b.WriteByte('I')
		case strings.ContainsRune("ÓÒÔÖÕ", r):
			b.WriteByte('O')
		case strings.ContainsRune("ÚÙÛÜ", r):
			b.WriteByte('U')
		case r == 'Ñ':
			b.WriteByte('N')
		case r == 'Ç':
			b.WriteByte('C')
		case unicode.IsSpace(r):
			b.WriteByte(' ')
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

// doc e' il foglio gia' pronto per la ricerca: le righe originali e quelle "piegate".
type doc struct {
	rows []string // testo com'e'
	f    []string // fold(rows[i])
	all  string   // tutte le righe piegate, unite con "\n"
}

func newDoc(rows []string) *doc {
	d := &doc{}
	for _, r := range rows {
		r = strings.Join(strings.Fields(r), " ")
		if r == "" {
			continue
		}
		d.rows = append(d.rows, r)
		d.f = append(d.f, fold(r))
	}
	d.all = strings.Join(d.f, "\n")
	return d
}

// after restituisce il testo originale della riga i dopo il match m (indici su d.f[i]), senza separatori iniziali.
func (d *doc) after(i, end int) string {
	r := []rune(d.rows[i])
	if end > len(r) {
		end = len(r)
	}
	return strings.TrimLeft(string(r[end:]), " :/-.|")
}

// Parse legge le righe di un certificato. Se il tipo non e' riconosciuto restituisce errore (non e' un certificato di questi quattro).
func Parse(rows []string) (*Result, error) {
	d := newDoc(rows)
	t := detectType(d.all)
	if t == "" {
		return nil, fmt.Errorf("certificato non riconosciuto: non e' un SAT, SD, SSO o CT di Panama")
	}
	res := &Result{Type: t, Fields: map[string]string{}}
	set := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			res.Fields[k] = v
		}
	}
	set(FDocNumber, docNumber(d))
	set(FMarineID, marineID(d))
	if t == CT {
		parseCT(d, set)
	} else {
		parseCourse(d, set)
	}
	check(res)
	return res, nil
}

var (
	reCT      = regexp.MustCompile(`PROCESSING CERTIFICATE|TRAMITE DE TITULO|SOLICITUD DE TITULO`)
	reSSO     = regexp.MustCompile(`SHIP SECURITY OFFICER|OFICIAL DE PROTECCION DEL BUQUE`)
	reSD      = regexp.MustCompile(`DESIGNATED SECURITY DUT|TAREAS DE PROTECCION ASIGNADAS|TAREAS DE PROTECCION`)
	reSAT     = regexp.MustCompile(`SECURITY AWARENESS|SENSIBILIZACION SOBRE PROTECCION`)
	reCourse  = regexp.MustCompile(`ENDOSO DE CURSOS|COURSES ENDORSEMENT`)
	reTableSD = regexp.MustCompile(`A-V[IL1]/6-2`)
	reTableSA = regexp.MustCompile(`A-V[IL1]/6-1`)
	reTableSO = regexp.MustCompile(`A-V[IL1]/5\b`)
)

// detectType distingue i quattro tipi dal titolo e dal tema del corso (parole, non solo i codici STCW: l'OCR confonde I e l).
func detectType(all string) string {
	switch {
	case reCT.MatchString(all):
		return CT
	case reSSO.MatchString(all):
		return SSO
	case reSD.MatchString(all):
		return SD
	case reSAT.MatchString(all):
		return SAT
	case reCourse.MatchString(all):
		switch {
		case reTableSO.MatchString(all):
			return SSO
		case reTableSD.MatchString(all):
			return SD
		case reTableSA.MatchString(all):
			return SAT
		}
		return SD // un altro corso con «Special Duty» (antincendio, sopravvivenza...): stesso foglio, stessi campi
	case strings.Contains(all, "SPECIAL DUTY") || strings.Contains(all, "ESPECIALIDAD"):
		return SD
	}
	return ""
}

// ---- numeri ----

var reDocNum = regexp.MustCompile(`E-?([0-9OIL]{9})([A-Z])`)

// digits corregge le lettere che l'OCR scambia con le cifre.
func digits(s string) string {
	return strings.NewReplacer("O", "0", "I", "1", "L", "1").Replace(s)
}

// docNumber cerca il numero del documento E-NNNNNNNNNA (in alto a destra; l'OCR spesso mette uno spazio o confonde O e 0).
func docNumber(d *doc) string {
	for _, f := range d.f {
		s := strings.ReplaceAll(f, " ", "")
		if m := reDocNum.FindStringSubmatch(s); m != nil {
			return "E-" + digits(m[1]) + m[2]
		}
	}
	return ""
}

var reNine = regexp.MustCompile(`^[0-9OIL]{9}$`)

// marineID e' il numero di 9 cifre sotto «Marine ID / ID del Marino».
func marineID(d *doc) string {
	for i, f := range d.f {
		if !strings.Contains(f, "MARINE ID") {
			continue
		}
		for j := i; j < len(d.f) && j <= i+3; j++ {
			for _, w := range strings.Fields(strings.ReplaceAll(d.f[j], "MARINE ID / ID DEL MARINO", " ")) {
				if reNine.MatchString(w) {
					return digits(w)
				}
			}
		}
	}
	for _, f := range d.f { // ripiego: una riga che e' solo il numero
		if s := strings.ReplaceAll(f, " ", ""); reNine.MatchString(s) && s[0] >= '1' && s[0] <= '9' {
			return digits(s)
		}
	}
	return ""
}

// ---- date ----

var (
	reDateNum = regexp.MustCompile(`([0-9OIL]{1,2})\s*[/.\-]\s*([0-9OIL]{1,2})\s*[/.\-]\s*([0-9OIL]{4})`)
	months    = map[string]int{"JANUARY": 1, "FEBRUARY": 2, "MARCH": 3, "APRIL": 4, "MAY": 5, "JUNE": 6, "JULY": 7,
		"AUGUST": 8, "SEPTEMBER": 9, "OCTOBER": 10, "NOVEMBER": 11, "DECEMBER": 12}
	reDateEN = regexp.MustCompile(`(JANUARY|FEBRUARY|MARCH|APRIL|MAY|JUNE|JULY|AUGUST|SEPTEMBER|OCTOBER|NOVEMBER|DECEMBER)\s+([0-9OIL]{1,2})\s*,?\s*([0-9OIL]{4})`)
)

func mkDate(dd, mm, yy string) (time.Time, bool) {
	var d, m, y int
	if _, err := fmt.Sscanf(digits(dd)+" "+digits(mm)+" "+digits(yy), "%d %d %d", &d, &m, &y); err != nil {
		return time.Time{}, false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if y < 1900 || y > 2200 || t.Day() != d || int(t.Month()) != m {
		return time.Time{}, false
	}
	return t, true
}

func fmtDate(t time.Time) string { return t.Format("02-01-2006") }

// datesIn restituisce le date di una riga piegata, nell'ordine in cui compaiono (GG/MM/AAAA e «October 29, 2026»).
func datesIn(f string) []time.Time {
	var out []time.Time
	for _, m := range reDateNum.FindAllStringSubmatch(f, -1) {
		if t, ok := mkDate(m[1], m[2], m[3]); ok {
			out = append(out, t)
		}
	}
	for _, m := range reDateEN.FindAllStringSubmatch(f, -1) {
		if t, ok := mkDate(m[2], fmt.Sprint(months[m[1]]), m[3]); ok {
			out = append(out, t)
		}
	}
	return out
}

// dateOnRow cerca la data nella prima riga che contiene una delle etichette.
func dateOnRow(d *doc, labels ...string) (time.Time, bool) {
	for _, f := range d.f {
		for _, l := range labels {
			if strings.Contains(f, l) {
				if ds := datesIn(f); len(ds) > 0 {
					return ds[0], true
				}
			}
		}
	}
	return time.Time{}, false
}

// ---- SAT, SD, SSO ----

var (
	reName = regexp.MustCompile(`NOMBRE\s*[/:]*\s*(?:NAME\b\s*[:.]?)?\s*`)
	reNat  = regexp.MustCompile(`NACIONALIDAD\s*(?:/|:)?\s*(?:NATIONALITY\s*[:.]?)?\s*`)
)

func parseCourse(d *doc, set func(k, v string)) {
	// nome del corso: l'ultima riga (la versione inglese) tra «ESPECIALIDAD - SPECIAL DUTY» e la data di scadenza
	for i, f := range d.f {
		if !strings.Contains(f, "ESPECIALIDAD") && !strings.Contains(f, "SPECIAL DUTY") {
			continue
		}
		for j := i + 1; j < len(d.f) && j <= i+6 && !strings.Contains(d.f[j], "EXPIR") && !strings.HasPrefix(d.f[j], "NOMBRE"); j++ {
			if len([]rune(d.rows[j])) >= 15 { // le righe corte («-1», «(IMO 3.26)») sono frammenti, non il titolo
				set(FCourse, truncate(d.rows[j], 300))
			}
		}
		break
	}
	if t, ok := dateOnRow(d, "EXPIRY", "EXPIRACION"); ok {
		set(FExpiry, fmtDate(t))
	}
	if t, ok := dateOnRow(d, "ISSUE", "EXPEDICION"); ok {
		set(FIssue, fmtDate(t))
	}
	if t, ok := dateOnRow(d, "BIRTH", "NACIMIENTO"); ok {
		set(FBirth, fmtDate(t))
	}
	for i, f := range d.f {
		if m := reName.FindStringIndex(f); m != nil && m[0] == 0 {
			set(FName, cleanName(d.after(i, m[1])))
			break
		}
	}
	for i, f := range d.f {
		if m := reNat.FindStringIndex(f); m != nil && m[0] == 0 {
			es, en := splitSlash(d.after(i, m[1]))
			set(FNationalityES, cleanName(es))
			set(FNationality, nationality(en))
			break
		}
	}
}

// cleanName toglie i segni che l'OCR attacca ai nomi e le spaziature doppie.
func cleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || r == ' ' || r == '\'' || r == '-' || r == '.' {
			return r
		}
		return -1
	}, s)
	return strings.ToUpper(strings.Join(strings.Fields(s), " "))
}

// nationality pulisce l'aggettivo di nazionalita' e ripara le letture sbagliate di una lettera (CREEK -> GREEK).
func nationality(s string) string {
	s = cleanName(s)
	if c, ok := verify.CorrectNationality(s); ok {
		return strings.ToUpper(c)
	}
	return s
}

// splitSlash divide «INDONESIA / INDONESIAN» in spagnolo e inglese. Senza barra, vale per entrambi.
func splitSlash(s string) (es, en string) {
	if i := strings.Index(s, "/"); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	return strings.TrimSpace(s), strings.TrimSpace(s)
}

// ---- CT (Panama Book provvisorio) ----

var (
	reCTNum = regexp.MustCompile(`CT\s*-?\s*(\d{4,8})\s*/\s*(\d{2})\s*-?\s*([A-Z]{2,4})\b`)
	// «con identificacion»: l'OCR rende male anche l'accento, quindi si guarda solo l'inizio della parola
	reConIdent = regexp.MustCompile(`^CON IDENTIF\w*\s*`)
	reDe       = regexp.MustCompile(`\s*,?\s*DE\s+`)
	reConsul   = regexp.MustCompile(`CONSUL\s+([A-Z]+)`)
	reReceipt  = regexp.MustCompile(`\b\d{7,8}\b`)
)

// consulates traduce la citta' stampata nel nome inglese; le altre restano come stampate.
var consulates = map[string]string{"LONDRES": "LONDON", "LONDON": "LONDON", "NAPOLES": "NAPLES", "NAPOLI": "NAPLES", "NAPLES": "NAPLES"}

func parseCT(d *doc, set func(k, v string)) {
	for _, f := range d.f {
		if m := reCTNum.FindStringSubmatch(f); m != nil {
			set(FCTNumber, "CT-"+m[1]+"/"+m[2]+"-"+m[3])
			break
		}
	}
	for _, f := range d.f {
		if m := reConsul.FindStringSubmatch(f); m != nil {
			if c, ok := consulates[m[1]]; ok {
				set(FConsulate, c)
			} else {
				set(FConsulate, m[1])
			}
			break
		}
	}
	// date: «October 29, 2026» (rilascio) e «January 29, 2027» (scadenza); il rilascio precede sempre la scadenza
	var all []time.Time
	for _, f := range d.f {
		all = append(all, datesIn(f)...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Before(all[j]) })
	if len(all) >= 2 {
		set(FIssue, fmtDate(all[0]))
		set(FExpiry, fmtDate(all[len(all)-1]))
	}
	for _, f := range d.f { // «Numero de Recibo / Receipt Number»: 8 cifre (il Marine ID ne ha 9)
		if strings.Contains(f, "RECIBO") || strings.Contains(f, "RECEIPT") || reReceipt.MatchString(strings.ReplaceAll(f, " ", "")) && !strings.ContainsAny(f, "/:,.") {
			if m := reReceipt.FindString(f); m != "" {
				set(FReceipt, m)
				break
			}
		}
	}
	for i, f := range d.f {
		switch {
		case reConIdent.MatchString(f):
			rest := reConIdent.ReplaceAllString(f, "")
			id, nat := rest, ""
			if loc := reDe.FindStringIndex(rest); loc != nil { // «A12345678 , de SUDAFRICANA»: la virgola puo' mancare o finire su un'altra riga
				id, nat = rest[:loc[0]], rest[loc[1]:]
			}
			set(FIdentity, strings.ReplaceAll(strings.Trim(id, " ,"), " ", ""))
			set(FNationalityES, cleanName(nat))
			if i > 0 && isNameRow(d.f[i-1]) {
				set(FName, cleanName(d.rows[i-1]))
			}
		case strings.HasPrefix(f, "WITH IDENTIFICATION"):
			if k := strings.Index(f, "NATIONALITY"); k >= 0 {
				set(FNationality, nationality(d.after(i, k+len("NATIONALITY"))))
			}
		case strings.HasPrefix(f, "CARGO"):
			set(FCapacityES, cleanName(between(f, d.rows[i], "CARGO", "Y HA PAGADO")))
		case strings.HasPrefix(f, "CAPACITY"):
			set(FCapacity, cleanName(between(f, d.rows[i], "CAPACITY", "AND HAS PAID")))
		}
	}
}

// isNameRow dice se la riga sembra un nome: solo lettere e spazi, almeno due parole, nessuna etichetta.
func isNameRow(f string) bool {
	w := strings.Fields(f)
	if len(w) < 2 || strings.ContainsAny(f, "0123456789/:,.") {
		return false
	}
	return !strings.Contains(f, "FAVOR") && !strings.Contains(f, "FAVOUR") && !strings.Contains(f, "APPLICATION")
}

// between prende da orig il testo tra due etichette (cercate nella versione piegata f, che ha gli stessi indici).
func between(f, orig, from, to string) string {
	a := strings.Index(f, from)
	if a < 0 {
		return ""
	}
	a += len(from)
	b := len(f)
	if k := strings.Index(f[a:], to); k >= 0 {
		b = a + k
	}
	r := []rune(orig)
	if b > len(r) {
		b = len(r)
	}
	return string(r[a:b])
}

// ---- controlli ----

// check riempie Missing e Warnings: campi richiesti non letti e date incoerenti.
func check(r *Result) {
	for _, k := range Required[r.Type] {
		if r.Fields[k] == "" {
			r.Missing = append(r.Missing, k)
		}
	}
	parse := func(k string) (time.Time, bool) {
		t, err := time.Parse("02-01-2006", r.Fields[k])
		return t, err == nil
	}
	iss, okI := parse(FIssue)
	exp, okE := parse(FExpiry)
	if okI && okE && !exp.After(iss) {
		r.Warnings = append(r.Warnings, "la data di scadenza non e' dopo quella di rilascio")
	}
	if b, ok := parse(FBirth); ok && okI && (iss.Year()-b.Year() < 16 || iss.Year()-b.Year() > 90) {
		r.Warnings = append(r.Warnings, "data di nascita poco plausibile rispetto al rilascio")
	}
	if r.Type == CT && okI && okE {
		if days := exp.Sub(iss).Hours() / 24; days < 60 || days > 120 {
			r.Warnings = append(r.Warnings, "un CT vale circa 3 mesi: controlla le due date")
		}
	}
}

// truncate accorcia a n caratteri (non a n byte).
func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
