package verify

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"scanlibretto/internal/outsum"
)

// now e' l'orologio per i controlli sugli anni plausibili: iniettabile nei test.
var now = time.Now

// Severity e' la gravita' di un'incoerenza.
type Severity int

const (
	// SevInfo: nota, non e' un problema.
	SevInfo Severity = iota
	// SevWarn: sospetto (campo corto, simbolo strano, differenza che puo' avere un motivo).
	SevWarn
	// SevError: contraddice un formato del libretto o un'altra parte dello stesso JSON: almeno una delle due e' sbagliata.
	SevError
)

// String e' l'etichetta stampata nei report.
func (s Severity) String() string {
	switch s {
	case SevInfo:
		return "INFO"
	case SevWarn:
		return "AVVISO"
	case SevError:
		return "ERRORE"
	}
	return "?"
}

// Finding e' un'incoerenza trovata senza bisogno della verita'. Il testo e' fisso: non contiene mai valori letti.
type Finding struct {
	Code     string   // codice stabile
	Severity Severity //
	Fields   []string // campi coinvolti, con i nomi del JSON
	Text     string   // descrizione fissa
}

var (
	reSeamansBook = regexp.MustCompile(`^PA?[0-9]{7}$`) // PA + 7 cifre (2026), P + 7 cifre (2017 e 2020)
	reNineDigits  = regexp.MustCompile(`^[0-9]{9}$`)
	reDDMMYYYY    = regexp.MustCompile(`^[0-9]{2}-[0-9]{2}-[0-9]{4}$`)
	reIDChars     = regexp.MustCompile(`^[A-Z0-9-]+$`)
	// reGrade: sigle di grado in un cargo ("II/2", "IV/2", "A-VI/2", "1/2", "III").
	reGrade = regexp.MustCompile(`^(?:[A-Z]{1,2}-)?(?:[IVX]+(?:/[0-9]+)?|[0-9]+/[0-9]+)$`)
)

// Validita' plausibile tra emissione e scadenza, in giorni: i libretti visti durano da circa 5 mesi a 5 anni.
const (
	minValidityDays = 30
	maxValidityDays = 5*366 + 31
)

// levels sono i soli livelli stampati dal libretto.
var levels = map[string]bool{"SUPPORT": true, "OPERATIONAL": true, "MANAGEMENT": true}

// Check applica le regole di coerenza al riassunto di un'uscita JSON, senza bisogno della verita'. I messaggi non
// contengono valori.
func Check(sum outsum.Summary) []Finding {
	c := &checker{f: sum.Fields, sum: sum}
	c.formats()
	c.dates()
	c.texts()
	c.nationality()
	c.mrz()
	return c.out
}

type checker struct {
	f   map[string]string
	sum outsum.Summary
	out []Finding
}

func (c *checker) add(code string, sev Severity, text string, fields ...string) {
	c.out = append(c.out, Finding{Code: code, Severity: sev, Fields: fields, Text: text})
}

// formats controlla i formati fissi osservati sul libretto.
func (c *checker) formats() {
	if v := c.f["type"]; v != "" && v != "SI" {
		c.add("FORMAT_TYPE", SevError, "il tipo stampato dev'essere SI (nell'MRZ e' IS)", "type")
	}
	if v := c.f["country_code"]; v != "" && v != "PAN" {
		c.add("FORMAT_COUNTRY_CODE", SevError, "il codice paese dev'essere PAN", "country_code")
	}
	if v := c.f["seamans_book"]; v != "" && !reSeamansBook.MatchString(v) {
		c.add("FORMAT_SEAMANS_BOOK", SevError, "il numero libretto dev'essere PA + 7 cifre (P + 7 cifre sui modelli 2017 e 2020)", "seamans_book")
	}
	if v := c.f["seafarers_id"]; v != "" && !reNineDigits.MatchString(v) {
		c.add("FORMAT_SEAFARERS_ID", SevError, "l'Id Marino dev'essere di 9 cifre", "seafarers_id")
	}
	// Il certificato (`Nº Título / Certificate`) esiste solo su alcuni titolari (ufficiali con una qualifica di grado):
	// se manca non e' un errore, si controlla solo quando c'e'. Visto su due titolari, entrambi di 9 cifre: con cosi'
	// pochi esemplari un formato diverso e' un avviso, non un errore.
	if v := lookup(c.f, FieldCertificate); v != "" && !reNineDigits.MatchString(v) {
		c.add("FORMAT_CERTIFICATE", SevWarn, "il numero del certificato dev'essere di 9 cifre (visto su due titolari)", FieldCertificate)
	}
	if v := c.f["id_number"]; v != "" {
		n := strings.ReplaceAll(v, "-", "")
		if !reIDChars.MatchString(v) || len(n) < 5 || len(n) > 12 {
			c.add("FORMAT_ID_NUMBER", SevError, "l'identificazione dev'essere di 5-12 lettere o cifre (con trattini)", "id_number")
		}
	}
	if v := c.f["level"]; v != "" && !levels[v] {
		c.add("FORMAT_LEVEL", SevError, "il livello dev'essere SUPPORT, OPERATIONAL o MANAGEMENT", "level")
	}
}

// dates controlla il formato, l'ordine e la plausibilita' delle tre date stampate.
func (c *checker) dates() {
	parsed := map[string]ymd{}
	for _, k := range []string{"date_of_issue", "date_of_expiry", "date_of_birth"} {
		v := c.f[k]
		if v == "" {
			continue
		}
		d, ok := parseDate(v)
		if !ok || !reDDMMYYYY.MatchString(v) {
			c.add("FORMAT_DATE", SevError, "la data non e' una data di calendario GG-MM-AAAA", k)
			continue
		}
		parsed[k] = d
		if iso := c.f[k+"_iso"]; iso != "" {
			if di, ok := parseDate(iso); !ok || di != d {
				c.add("DATE_ISO_MISMATCH", SevWarn, "la forma ISO della data non coincide con quella stampata", k, k+"_iso")
			}
		}
	}
	issue, hasIssue := parsed["date_of_issue"]
	expiry, hasExpiry := parsed["date_of_expiry"]
	birth, hasBirth := parsed["date_of_birth"]
	if hasBirth && hasIssue && !birth.before(issue) {
		c.add("DATE_BIRTH_NOT_BEFORE_ISSUE", SevError, "la nascita non precede l'emissione", "date_of_birth", "date_of_issue")
	}
	if hasIssue && hasExpiry && !issue.before(expiry) {
		c.add("DATE_EXPIRY_NOT_AFTER_ISSUE", SevError, "la scadenza non segue l'emissione", "date_of_issue", "date_of_expiry")
	}
	if hasBirth && hasExpiry && !birth.before(expiry) {
		c.add("DATE_EXPIRY_NOT_AFTER_BIRTH", SevError, "la scadenza non segue la nascita", "date_of_birth", "date_of_expiry")
	}
	year := now().Year()
	if hasIssue && (issue.y < 2000 || issue.y > year+1) {
		c.add("DATE_YEAR_RANGE", SevWarn, "l'anno di emissione e' fuori dal periodo plausibile (2000 - anno corrente + 1)", "date_of_issue")
	}
	if hasExpiry && (expiry.y < 2000 || expiry.y > year+15) {
		c.add("DATE_YEAR_RANGE", SevWarn, "l'anno di scadenza e' fuori dal periodo plausibile (2000 - anno corrente + 15)", "date_of_expiry")
	}
	if hasBirth && (birth.y < 1920 || birth.y > year-14) {
		c.add("DATE_YEAR_RANGE", SevWarn, "l'anno di nascita e' fuori dal periodo plausibile (1920 - anno corrente - 14)", "date_of_birth")
	}
	if hasIssue && hasExpiry && issue.before(expiry) {
		// la validita' osservata va da circa 5 mesi a 5 anni (mai oltre): fuori da 30 giorni - 5 anni e un mese e' sospetto
		days := int(expiry.time().Sub(issue.time()).Hours() / 24)
		if days < minValidityDays || days > maxValidityDays {
			c.add("DATE_VALIDITY_IMPLAUSIBLE", SevWarn, "la validita' tra emissione e scadenza e' fuori da 30 giorni - 5 anni e un mese (osservato: da 5 mesi a 5 anni)", "date_of_issue", "date_of_expiry")
		}
	}
	if hasBirth && hasIssue && birth.before(issue) {
		if age := ageAt(birth, issue); age < 16 || age > 75 {
			c.add("DATE_AGE_IMPLAUSIBLE", SevWarn, "l'eta' del titolare all'emissione e' fuori da 16-75 anni", "date_of_birth", "date_of_issue")
		}
	}
}

// ageAt e' l'eta' in anni compiuti alla data at.
func ageAt(birth, at ymd) int {
	a := at.y - birth.y
	if at.m < birth.m || (at.m == birth.m && at.d < birth.d) {
		a--
	}
	return a
}

// textSpec dice come deve essere fatto un campo di testo libero.
type textSpec struct {
	field      string
	minLetters int  // meno di cosi' e' sospetto (campo troncato)
	digits     bool // ammette cifre (solo il cargo, per i gradi come II/2)
}

var textSpecs = []textSpec{
	{"nationality", 4, false},
	{"place_of_birth", 3, false},
	{"capacity", 4, true},
	{"regional", 3, false},
}

// allowedPunct sono i simboli ammessi nei campi stampati oltre a lettere, cifre e spazio.
const allowedPunct = "-/.'"

// texts cerca caratteri anomali e campi troppo corti o con simboli.
func (c *checker) texts() {
	// caratteri anomali su tutti i 14 campi
	for _, key := range outsum.FieldKeys {
		v := lookup(c.f, key)
		k := Canonical(key) // i messaggi usano il nome attuale del campo
		if v == "" {
			continue
		}
		var accent, control, symbols, lower bool
		for _, r := range v {
			switch {
			case r < 0x20 || r == 0x7F:
				control = true
			case r > unicode.MaxASCII:
				if unicode.IsLetter(r) {
					accent = true
				} else {
					control = true // simbolo o carattere non stampabile fuori dall'ASCII
				}
			case unicode.IsLower(r):
				lower = true
			case unicode.IsUpper(r) || unicode.IsDigit(r) || r == ' ' || strings.ContainsRune(allowedPunct, r):
			default:
				symbols = true
			}
		}
		if control {
			c.add("CHAR_NON_PRINTABLE", SevError, "contiene caratteri non ASCII stampabili (controllo o simboli fuori dall'ASCII)", k)
		}
		if accent {
			c.add("CHAR_ACCENT", SevWarn, "contiene lettere accentate (la stampa del libretto e' senza accenti: probabile lettura spuria)", k)
		}
		if symbols {
			c.add("CHAR_SYMBOLS", SevWarn, "contiene simboli che non compaiono nei campi stampati", k)
		}
		if lower {
			c.add("CHAR_LOWERCASE", SevWarn, "contiene minuscole (la stampa e' tutta maiuscola)", k)
		}
		if v != strings.TrimSpace(v) || strings.Contains(v, "  ") {
			c.add("FIELD_SPACES", SevWarn, "spazi iniziali, finali o doppi", k)
		}
	}
	// forma dei testi liberi
	for _, s := range textSpecs {
		v := c.f[s.field]
		if v == "" {
			continue
		}
		letters, digits := 0, 0
		for _, r := range v {
			if unicode.IsLetter(r) {
				letters++
			} else if unicode.IsDigit(r) {
				digits++
			}
		}
		if letters < s.minLetters {
			c.add("FIELD_TOO_SHORT", SevWarn, "campo troppo corto: probabile lettura troncata", s.field)
		}
		if !s.digits && digits > 0 {
			c.add("FIELD_HAS_DIGITS", SevWarn, "contiene cifre dove ci sono solo lettere", s.field)
		}
		if s.field == "capacity" {
			c.capacityTokens(v)
		}
		c.noiseTokens(s.field, v)
	}
}

// capacityTokens controlla le parole del cargo: le cifre sono ammesse solo nei gradi (II/2, IV/2, A-VI/2).
func (c *checker) capacityTokens(v string) {
	for _, tok := range strings.Fields(v) {
		hasDigit := false
		for _, r := range tok {
			if unicode.IsDigit(r) {
				hasDigit = true
			}
		}
		if hasDigit && !reGrade.MatchString(tok) {
			c.add("CAPACITY_SUSPICIOUS_TOKEN", SevWarn, "una parola del cargo ha cifre ma non e' un grado (II/2, IV/2, ...)", "capacity")
			return
		}
	}
}

// noiseTokens segnala le parole di una sola lettera nei testi liberi (tipico del rumore di fondo letto come testo).
// Fanno eccezione le cifre romane del cargo (I, V, X) e le sigle di grado.
func (c *checker) noiseTokens(field, v string) {
	for _, tok := range strings.Fields(v) {
		if len([]rune(tok)) != 1 {
			continue
		}
		r := []rune(tok)[0]
		if !unicode.IsLetter(r) {
			continue
		}
		if field == "capacity" && strings.ContainsRune("IVX", r) {
			continue
		}
		c.add("FIELD_NOISE_TOKEN", SevWarn, "contiene una parola di una sola lettera: probabile rumore letto come testo", field)
		return
	}
}

// nationality confronta l'aggettivo di nazionalita' con il codice ISO dell'MRZ e con la riga del nome.
func (c *checker) nationality() {
	nat := c.f["nationality"]
	if nat == "" {
		return
	}
	n := Norm(nat)
	var names []string
	for _, k := range []string{"surname", "given_names"} {
		for _, w := range strings.Fields(c.f[k]) {
			if t := Norm(w); len(t) >= 4 {
				names = append(names, t)
			}
		}
	}
	if hasNameWord(nat, names) && !isNationalityAdjective(n) {
		c.add("NATIONALITY_LOOKS_LIKE_NAME", SevError, "la nazionalita' contiene parole della riga del nome: e' stato letto il testo sbagliato", "nationality", "surname", "given_names")
		return
	}
	iso := strings.ToUpper(c.f["nationality_iso"])
	if iso == "" {
		if !isNationalityAdjective(n) {
			c.add("NATIONALITY_NOT_IN_TABLE", SevWarn, "la nazionalita' non e' un aggettivo noto e non c'e' il codice ISO per controllarla", "nationality")
		}
		return
	}
	adjs, known := nationalityAdjectives[iso]
	if !known {
		c.add("NATIONALITY_ISO_UNKNOWN", SevInfo, "il codice ISO dell'MRZ non e' nella tabella: nazionalita' non controllabile", "nationality", "nationality_iso")
		return
	}
	for _, a := range adjs {
		if Norm(a) == n {
			return
		}
	}
	if isNationalityAdjective(n) {
		c.add("NATIONALITY_ISO_MISMATCH", SevError, "la nazionalita' stampata e' l'aggettivo di un altro paese rispetto al codice ISO dell'MRZ", "nationality", "nationality_iso")
		return
	}
	c.add("NATIONALITY_NOT_IN_TABLE", SevError, "la nazionalita' stampata non e' l'aggettivo atteso per il codice ISO dell'MRZ e non e' un aggettivo noto", "nationality", "nationality_iso")
}

// isNationalityAdjective dice se il testo (gia' normalizzato con Norm) e' un aggettivo di nazionalita' della tabella.
func isNationalityAdjective(n string) bool {
	for _, adjs := range nationalityAdjectives {
		for _, a := range adjs {
			if Norm(a) == n {
				return true
			}
		}
	}
	return false
}

// mrz confronta la zona visiva con l'MRZ trovato.
func (c *checker) mrz() {
	if !c.sum.HasMRZ {
		c.add("MRZ_NOT_FOUND", SevInfo, "MRZ non trovato: i confronti con l'MRZ non sono stati fatti")
		return
	}
	if c.sum.ChecksOK() < outsum.NChecks {
		c.add("MRZ_CHECK_DIGITS_INVALID", SevWarn, "uno o piu' check digit dell'MRZ non tornano: i confronti con l'MRZ sono meno affidabili")
	}
	pairs := []struct{ printed, mrz, code, text string }{
		{"date_of_birth", "date_of_birth_mrz", "DATE_OF_BIRTH_DIFFERS_FROM_MRZ", "la data di nascita stampata non coincide con l'MRZ"},
		{"date_of_expiry", "date_of_expiry_mrz", "DATE_OF_EXPIRY_DIFFERS_FROM_MRZ", "la data di scadenza stampata non coincide con l'MRZ"},
		{"seamans_book", "seamans_book_mrz", "SEAMANS_BOOK_DIFFERS_FROM_MRZ", "il numero libretto stampato non coincide con l'MRZ"},
	}
	for _, p := range pairs {
		a, b := c.f[p.printed], c.f[p.mrz]
		if a != "" && b != "" && !Equal(p.printed, a, b) {
			c.add(p.code, SevError, p.text, p.printed, p.mrz)
		}
	}
	// il campo opzionale dell'MRZ e' il nome dell'ufficio, eventualmente accorciato ("PANAMA" per "PANAMA DR")
	if reg, regMRZ := Norm(c.f["regional"]), Norm(c.f["regional_mrz"]); reg != "" && regMRZ != "" &&
		!strings.HasPrefix(reg, regMRZ) && !strings.HasPrefix(regMRZ, reg) {
		c.add("REGIONAL_DIFFERS_FROM_MRZ", SevWarn, "il regionale stampato non comincia come il campo opzionale dell'MRZ", "regional", "regional_mrz")
	}
	if s := c.f["sex"]; s != "" && s != "M" && s != "F" && s != "X" {
		c.add("SEX_UNKNOWN_VALUE", SevWarn, "il sesso dell'MRZ non e' M, F o X", "sex")
	}
}
