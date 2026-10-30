// Package seafarer fa il controllo doppio di un certificato di Panama con il servizio pubblico di verifica dell'AMP: il codice QR
// del certificato contiene un link del tipo
//
//	http://190.34.205.206:8080/ords/seafarersgm/seafarers/idid/207323124
//
// che mostra una pagina con nome, nazionalita', data di nascita, Marine ID («Seafarers ID»), tipo, stato e date del certificato.
// Si confrontano quei dati con quelli letti dal foglio.
//
// Sicurezza: il link arriva da un QR dentro un file non fidato, quindi NON si segue qualunque indirizzo: si accetta solo il
// servizio dell'AMP, scritto nel codice (stesso indirizzo, porta e percorso, solo cifre in fondo), senza redirect. La richiesta
// non porta nessun dato personale (solo il numero che sta gia' nel QR). Risposta limitata a 512 KB, tempo massimo breve.
// Il servizio e' in HTTP semplice (non cifrato): lo dice l'interfaccia e il controllo parte solo se l'operatore lo chiede.
package seafarer

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var reURL = regexp.MustCompile(`^http://190\.34\.205\.206:8080/ords/seafarersgm/seafarers/idid/(\d{1,12})$`)

// Timeout e' il tempo massimo della richiesta.
const Timeout = 8 * time.Second

const maxBody = 512 << 10

// ValidURL dice se il testo del QR e' un link del servizio AMP (e quindi si puo' interrogare).
func ValidURL(u string) bool { return reURL.MatchString(strings.TrimSpace(u)) }

// Record sono i dati della pagina del servizio, con chiavi fisse (name, nationality, date_of_birth, sex, id, marine_id,
// doc_number, type_of_certificate, type_of_document, capacity, status, date_of_issue, date_of_expiry, seamanbook_number).
type Record map[string]string

var labels = map[string]string{
	"name": "name", "nationality": "nationality", "date of birth": "date_of_birth", "sex": "sex", "id": "id",
	"seafarers id": "marine_id", "doc number": "doc_number", "type of certificate": "type_of_certificate",
	"type of document": "type_of_document", "capacity": "capacity", "regulation": "regulation", "level": "level",
	"status": "status", "date of issue": "date_of_issue", "date of expiration": "date_of_expiry",
	"seamanbook number": "seamanbook_number",
}

var (
	reScript = regexp.MustCompile(`(?is)<script.*?</script>|<style.*?</style>|<!--.*?-->`)
	reTag    = regexp.MustCompile(`(?s)<[^>]*>`)
)

// Parse legge la pagina del servizio: coppie «Etichetta:» seguita dal valore (che puo' mancare).
func Parse(page []byte) (Record, error) {
	s := string(page)
	if !strings.Contains(strings.ToUpper(s), "ONLINE CONSULTATION") {
		return nil, errors.New("risposta non riconosciuta")
	}
	s = reScript.ReplaceAllString(s, " ")
	s = reTag.ReplaceAllString(s, "\n")
	s = html.UnescapeString(strings.ReplaceAll(s, " ", " "))
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			lines = append(lines, l)
		}
	}
	rec := Record{}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if !strings.HasSuffix(l, ":") {
			continue
		}
		key, ok := labels[strings.ToLower(strings.TrimSuffix(l, ":"))]
		if !ok {
			continue
		}
		val := ""
		if i+1 < len(lines) && !strings.HasSuffix(lines[i+1], ":") && len(lines[i+1]) <= 120 {
			val = lines[i+1]
			i++
		}
		if _, dup := rec[key]; !dup {
			rec[key] = val
		}
	}
	if rec["marine_id"] == "" && rec["name"] == "" {
		return nil, errors.New("la pagina non contiene i dati del marittimo")
	}
	return rec, nil
}

// Fetch interroga il servizio. client puo' essere sostituito nelle prove; in uso normale si passa NewClient().
func Fetch(ctx context.Context, client *http.Client, link string) (Record, error) {
	link = strings.TrimSpace(link)
	if !ValidURL(link) {
		return nil, errors.New("il link del QR non e' quello del servizio AMP: non lo interrogo")
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PanamaBook (verifica certificato)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("servizio AMP non raggiungibile (%s)", shortErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("il servizio AMP ha risposto %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(b) > maxBody {
		return nil, errors.New("risposta del servizio AMP non valida")
	}
	return Parse(b)
}

// NewClient e' il client HTTP del controllo: nessun redirect (un redirect porterebbe fuori dall'indirizzo ammesso) e nessun cookie.
func NewClient() *http.Client {
	return &http.Client{Timeout: Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func shortErr(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "tempo scaduto"
	case errors.Is(err, context.Canceled):
		return "richiesta annullata"
	}
	if strings.Contains(err.Error(), "timeout") {
		return "tempo scaduto"
	}
	return "nessuna connessione"
}

// Check e' il confronto di un campo.
type Check struct {
	Key    string `json:"key"`
	Read   string `json:"read"`   // valore letto dal foglio
	Online string `json:"online"` // valore del servizio
	OK     bool   `json:"ok"`
}

var reDate = regexp.MustCompile(`^(\d{2})[-/](\d{2})[-/](\d{2}|\d{4})$`)

// sameDate confronta una data del foglio (GG-MM-AAAA) con quella del servizio (GG/MM/AA): giorno, mese e ultime due cifre dell'anno.
func sameDate(read, online string) bool {
	a, b := reDate.FindStringSubmatch(read), reDate.FindStringSubmatch(online)
	if a == nil || b == nil {
		return false
	}
	return a[1] == b[1] && a[2] == b[2] && a[3][len(a[3])-2:] == b[3][len(b[3])-2:]
}

func norm(s string) string { return strings.ToUpper(strings.Join(strings.Fields(s), " ")) }

// Pair collega un campo del foglio al campo della pagina online.
type Pair struct{ Key, RecKey string }

// CertPairs sono i campi confrontati per i certificati (chiavi di internal/certif); BookPairs per il libretto (chiavi di fieldDefs).
var (
	CertPairs = []Pair{{"marine_id", "marine_id"}, {"name", "name"}, {"date_of_birth", "date_of_birth"}, {"nationality", "nationality"},
		{"identification", "id"}, {"date_of_issue", "date_of_issue"}, {"date_of_expiry", "date_of_expiry"}}
	BookPairs = []Pair{{"seafarers_id", "marine_id"}, {"name", "name"}, {"date_of_birth", "date_of_birth"}, {"nationality", "nationality"},
		{"id_number", "id"}, {"seamans_book", "seamanbook_number"}, {"sex", "sex"}, {"capacity", "capacity"}, {"level", "level"},
		{"date_of_issue", "date_of_issue"}, {"date_of_expiry", "date_of_expiry"}}
)

// Compare confronta i campi di un certificato con la pagina online.
func Compare(fields map[string]string, rec Record) []Check { return compare(fields, rec, CertPairs) }

// CompareBook confronta i campi del libretto: il nome online e' «NOME COGNOME» e il sesso e' scritto per esteso.
func CompareBook(fields map[string]string, rec Record) []Check {
	f := map[string]string{}
	for k, v := range fields {
		f[k] = v
	}
	if n := strings.TrimSpace(f["given_names"] + " " + f["surname"]); n != "" {
		f["name"] = n
	}
	switch strings.ToUpper(f["sex"]) {
	case "M":
		f["sex"] = "MALE"
	case "F":
		f["sex"] = "FEMALE"
	}
	return compare(f, rec, BookPairs)
}

// compare confronta solo i campi presenti da entrambe le parti. Dice solo «uguale o diverso»: decide l'operatore.
func compare(fields map[string]string, rec Record, pairs []Pair) []Check {
	var out []Check
	for _, p := range pairs {
		r, o := fields[p.Key], rec[p.RecKey]
		if r == "" || o == "" {
			continue
		}
		c := Check{Key: p.Key, Read: r, Online: o}
		if strings.HasPrefix(p.Key, "date_") {
			c.OK = sameDate(r, o)
		} else {
			c.OK = norm(r) == norm(o)
		}
		out = append(out, c)
	}
	return out
}
