package seafarer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Ricerca per Marine ID sul sito di verifica dell'AMP (applicazione Oracle APEX 100): lo stesso percorso che fa una persona nel
// browser. Si apre il sito (nasce una sessione), si apre la pagina «Search», si invia il modulo con nome, data di nascita e Marine
// ID (il sito li chiede tutti e tre, cosi' non si possono scorrere i numeri) e si legge la pagina del marittimo con l'elenco dei
// suoi documenti. Il modulo si invia come lo invia la pagina: con i valori di controllo (checksum) che la pagina stessa fornisce.
//
// Prudenze: indirizzo fisso (stesso host e porta, nessun redirect altrove), un solo giro di richieste a ogni ricerca, pausa minima
// tra due ricerche, risposte limitate, mai scorrere elenchi. Il sito e' in HTTP non cifrato: nome, data di nascita e Marine ID
// viaggiano in chiaro (come se si usasse il sito dal browser); la pagina lo dice e la ricerca parte solo su richiesta.

const apexBase = "http://190.34.205.206:8080/ords/"

// base e' l'indirizzo del servizio; cambia solo nelle prove.
var base = apexBase

const (
	apexMaxBody = 1 << 20
	apexMinGap  = 3 * time.Second
	// ApexTimeout e' il tempo massimo di una ricerca (quattro richieste).
	ApexTimeout = 30 * time.Second
)

var (
	gapMu   sync.Mutex
	lastRun time.Time
)

// Person sono i dati anagrafici della pagina del marittimo.
type Person struct {
	Name        string `json:"name"`
	Nationality string `json:"nationality"`
	Birth       string `json:"date_of_birth"` // GG-MM-AAAA
	Sex         string `json:"sex"`
	ID          string `json:"id"`        // documento d'identita' (passaporto)
	MarineID    string `json:"marine_id"` // «Seafarer ID»
}

// DocRow e' un documento dell'elenco.
type DocRow struct {
	DocNumber string `json:"doc_number"`
	CertType  string `json:"cert_type"` // Type of Certificate
	DocType   string `json:"doc_type"`  // Type of Document (SEAMANS BOOK, CT, TITLE, COURSE ENDORSEMENT...)
	Capacity  string `json:"capacity"`
	Status    string `json:"status"`
	Issue     string `json:"date_of_issue"`  // GG-MM-AAAA
	Expiry    string `json:"date_of_expiry"` // GG-MM-AAAA
}

// Found e' l'esito di una ricerca.
type Found struct {
	Person
	Docs []DocRow `json:"docs"`
	More bool     `json:"more"` // il sito ha altre pagine di documenti (qui si leggono solo i primi 15)
}

var monthNum = map[string]string{"JAN": "01", "FEB": "02", "MAR": "03", "APR": "04", "MAY": "05", "JUN": "06", "JUL": "07", "AUG": "08", "SEP": "09", "OCT": "10", "NOV": "11", "DEC": "12"}

var reMonDate = regexp.MustCompile(`^(\d{1,2})-([A-Za-z]{3})-(\d{4})$`)

// monDate porta «24-OCT-1992» a «24-10-1992»; il resto resta com'e'.
func monDate(s string) string {
	if m := reMonDate.FindStringSubmatch(strings.TrimSpace(s)); m != nil {
		if n, ok := monthNum[strings.ToUpper(m[2])]; ok {
			return fmt.Sprintf("%s-%s-%s", strings.Repeat("0", 2-len(m[1]))+m[1], n, m[3])
		}
	}
	return strings.TrimSpace(s)
}

// DMY porta una data del sito («24-OCT-1992») a GG-MM-AAAA.
func DMY(s string) string { return monDate(s) }

// SearchDate e' la data nella forma richiesta dal sito (GG-MES-AAAA) a partire da GG-MM-AAAA.
func SearchDate(d string) string {
	m := regexp.MustCompile(`^(\d{2})[-/.](\d{2})[-/.](\d{4})$`).FindStringSubmatch(strings.TrimSpace(d))
	if m == nil {
		return ""
	}
	n, _ := strconv.Atoi(m[2])
	mon := []string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}
	if n < 1 || n > 12 {
		return ""
	}
	return m[1] + "-" + mon[n-1] + "-" + m[3]
}

// NamePattern costruisce il nome per la ricerca: ogni parola con «%» finale, cosi' i secondi nomi non contano
// («MARIO ROSSI JR» -> «MARIO% %ROSSI% %JR»).
func NamePattern(name string) string {
	var w []string
	for _, p := range strings.Fields(strings.ToUpper(name)) {
		p = strings.Map(func(r rune) rune {
			if r == '\'' || r == '%' || r == '"' || r == '_' || r == ';' {
				return -1
			}
			return r
		}, p)
		if p != "" {
			w = append(w, p)
		}
	}
	return strings.Join(w, "% %")
}

var (
	reHiddenCk = regexp.MustCompile(`<input[^>]*data-for="([^"]+)"[^>]*value="([^"]*)"|<input[^>]*value="([^"]*)"[^>]*data-for="([^"]+)"`)
	reSubmit   = regexp.MustCompile(`name="p_page_submission_id"[^>]*value="([^"]*)"|value="([^"]*)"[^>]*name="p_page_submission_id"`)
	reSalt     = regexp.MustCompile(`value="([^"]*)"\s+id="pSalt"|id="pSalt"[^>]*value="([^"]*)"`)
	reProt     = regexp.MustCompile(`id="pPageItemsProtected"[^>]*value="([^"]*)"|value="([^"]*)"\s+id="pPageItemsProtected"`)
	reSession  = regexp.MustCompile(`f\?p=100:\d+:(\d+)`)
	reHidden   = regexp.MustCompile(`<input[^>]*type="hidden"[^>]*name="(P\d+_TEXT|TEXTOANTES|TEXTODESPUES)"[^>]*value="([^"]*)"`)
)

func first(m []string) string {
	for _, s := range m[1:] {
		if s != "" {
			return html.UnescapeString(s)
		}
	}
	return ""
}

// searchForm legge dalla pagina «Search» i valori che il modulo deve rimandare.
type searchForm struct {
	submission, salt, protected string
	hidden                      []jsonItem // elementi nascosti protetti, con il loro checksum
}

type jsonItem struct {
	N  string `json:"n"`
	V  string `json:"v"`
	Ck string `json:"ck,omitempty"`
}

func parseSearchForm(page string) (*searchForm, error) {
	f := &searchForm{}
	if m := reSubmit.FindStringSubmatch(page); m != nil {
		f.submission = first(m)
	}
	if m := reSalt.FindStringSubmatch(page); m != nil {
		f.salt = first(m)
	}
	if m := reProt.FindStringSubmatch(page); m != nil {
		f.protected = first(m)
	}
	cks := map[string]string{}
	for _, m := range reHiddenCk.FindAllStringSubmatch(page, -1) {
		if m[1] != "" {
			cks[m[1]] = html.UnescapeString(m[2])
		} else {
			cks[m[4]] = html.UnescapeString(m[3])
		}
	}
	for _, m := range reHidden.FindAllStringSubmatch(page, -1) {
		f.hidden = append(f.hidden, jsonItem{N: m[1], V: html.UnescapeString(m[2]), Ck: cks[m[1]]})
	}
	if f.submission == "" || f.salt == "" || f.protected == "" {
		return nil, errors.New("pagina di ricerca non riconosciuta")
	}
	return f, nil
}

// body costruisce il corpo della richiesta come fa la pagina.
func (f *searchForm) body(sid, name, dob, marineID string) string {
	items := []jsonItem{{N: "P2_NOMBRE", V: name}, {N: "P2_FECHA_NACIMIENTO", V: dob}, {N: "P2_NUM_DOC_MARINE_ID", V: "1"}, {N: "P2_ID_MARINO", V: marineID}}
	items = append(items, f.hidden...)
	js, _ := json.Marshal(map[string]any{
		"pageItems": map[string]any{"itemsToSubmit": items, "protected": f.protected, "rowVersion": ""},
		"salt":      f.salt,
	})
	v := url.Values{}
	v.Set("p_flow_id", "100")
	v.Set("p_flow_step_id", "2")
	v.Set("p_instance", sid)
	v.Set("p_debug", "")
	v.Set("p_request", "P2_BTN_Search")
	v.Set("p_reload_on_submit", "S")
	v.Set("p_page_submission_id", f.submission)
	v.Set("p_json", string(js))
	return v.Encode()
}

var (
	reResultLink = regexp.MustCompile(`href="(f\?p=100:6:\d+::NO:RP:P6_ID:(\d+)&cs=[^"]+)"`)
	reHref       = regexp.MustCompile(`href="([^"]*)"`)
	reRow        = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	reCell       = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	reDocNum     = regexp.MustCompile(`P9_DOC_NUMBER:(\d+)`)
)

func text(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(strings.ReplaceAll(reTag.ReplaceAllString(s, " "), " ", " "))), " ")
}

// unescHref decodifica le entita' dentro agli href (il sito scrive «f&#x3F;p&#x3D;100&#x3A;6...»), cosi' i collegamenti si riconoscono.
func unescHref(page string) string {
	return reHref.ReplaceAllStringFunc(page, func(h string) string {
		return `href="` + html.UnescapeString(h[len(`href="`):len(h)-1]) + `"`
	})
}

// parsePerson legge nome, nazionalita', nascita, sesso, ID e Marine ID dalla pagina del marittimo, e la tabella dei documenti.
func parsePerson(page string) (*Found, error) {
	page = unescHref(page)
	body := reScript.ReplaceAllString(page, " ")
	lines := strings.Split(reTag.ReplaceAllString(body, "\n"), "\n")
	var clean []string
	for _, l := range lines {
		if l = text(l); l != "" {
			clean = append(clean, l)
		}
	}
	f := &Found{}
	set := func(label string, dst *string) {
		for i, l := range clean {
			if strings.EqualFold(l, label) && i+1 < len(clean) {
				*dst = clean[i+1]
				return
			}
		}
	}
	set("Name", &f.Name)
	set("Nationality", &f.Nationality)
	set("Date of Birth", &f.Birth)
	f.Birth = monDate(f.Birth)
	set("Sex", &f.Sex)
	set("ID", &f.ID)
	set("Seafarers ID", &f.MarineID)
	if f.Name == "" || f.MarineID == "" {
		return nil, errors.New("pagina del marittimo non riconosciuta")
	}
	f.More = strings.Contains(body, "t-Report-paginationLink--next")
	for _, r := range reRow.FindAllStringSubmatch(body, -1) {
		dn := reDocNum.FindStringSubmatch(r[1])
		if dn == nil {
			continue
		}
		var cells []string
		for _, c := range reCell.FindAllStringSubmatch(r[1], -1) {
			cells = append(cells, text(c[1]))
		}
		if len(cells) < 7 {
			continue
		}
		f.Docs = append(f.Docs, DocRow{DocNumber: dn[1], CertType: cells[1], DocType: cells[2], Capacity: cells[3], Status: cells[4], Issue: monDate(cells[5]), Expiry: monDate(cells[6])})
	}
	return f, nil
}

// SearchMarine cerca il marittimo con nome, data di nascita (GG-MM-AAAA) e Marine ID, come dal sito. Restituisce ErrNotFound se il
// sito non trova nessuno con quei dati.
func SearchMarine(ctx context.Context, name, birth, marineID string) (*Found, error) {
	marineID = strings.TrimSpace(marineID)
	if !regexp.MustCompile(`^\d{9}$`).MatchString(marineID) {
		return nil, errors.New("il Marine ID deve essere di 9 cifre")
	}
	dob := SearchDate(birth)
	pat := NamePattern(name)
	if dob == "" || pat == "" {
		return nil, errors.New("servono il nome e la data di nascita (GG-MM-AAAA)")
	}
	gapMu.Lock()
	if wait := apexMinGap - time.Since(lastRun); wait > 0 {
		gapMu.Unlock()
		return nil, fmt.Errorf("attendi %d secondi prima di una nuova ricerca sul sito AMP", int(wait.Seconds())+1)
	}
	lastRun = time.Now()
	gapMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, ApexTimeout)
	defer cancel()
	jar, _ := cookiejar.New(nil)
	bu, _ := url.Parse(base)
	cl := &http.Client{Jar: jar, Timeout: ApexTimeout, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || r.URL.Host != bu.Host {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	do := func(method, u, ctype, body string) (string, string, error) {
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rd)
		if err != nil {
			return "", "", err
		}
		req.Header.Set("User-Agent", "PanamaBook (verifica marittimo)")
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
		}
		resp, err := cl.Do(req)
		if err != nil {
			return "", "", fmt.Errorf("sito AMP non raggiungibile (%s)", shortErr(err))
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(io.LimitReader(resp.Body, apexMaxBody+1))
		if err != nil || len(b) > apexMaxBody {
			return "", "", errors.New("risposta del sito AMP non valida")
		}
		if resp.StatusCode >= 400 {
			return "", "", fmt.Errorf("il sito AMP ha risposto %d", resp.StatusCode)
		}
		return resp.Request.URL.String(), string(b), nil
	}
	// 1) sessione
	u1, _, err := do(http.MethodGet, base+"f?p=100", "", "")
	if err != nil {
		return nil, err
	}
	m := reSession.FindStringSubmatch(u1)
	if m == nil {
		return nil, errors.New("il sito AMP non ha aperto una sessione")
	}
	sid := m[1]
	// 2) pagina di ricerca
	_, p2, err := do(http.MethodGet, base+"f?p=100:2:"+sid+"::NO:RP::", "", "")
	if err != nil {
		return nil, err
	}
	form, err := parseSearchForm(p2)
	if err != nil {
		return nil, err
	}
	// 3) invio del modulo, poi la pagina dei risultati (e' quella a cui la pagina porta dopo l'invio)
	if _, _, err = do(http.MethodPost, base+"wwv_flow.accept", "application/x-www-form-urlencoded; charset=UTF-8", form.body(sid, pat, dob, marineID)); err != nil {
		return nil, err
	}
	_, res, err := do(http.MethodGet, base+"f?p=100:2:"+sid+"::NO:::", "", "")
	if err != nil {
		return nil, err
	}
	res = unescHref(res)
	if strings.Contains(res, "protection violation") {
		return nil, errors.New("il sito AMP ha rifiutato la richiesta (protezione della pagina)")
	}
	var link string
	for _, l := range reResultLink.FindAllStringSubmatch(res, -1) {
		if l[2] == marineID {
			link = l[1]
			break
		}
	}
	if link == "" {
		return nil, ErrNotFound
	}
	// 4) pagina del marittimo con i suoi documenti
	_, p6, err := do(http.MethodGet, base+link, "", "")
	if err != nil {
		return nil, err
	}
	f, err := parsePerson(p6)
	if err != nil {
		return nil, err
	}
	if f.MarineID != marineID {
		return nil, errors.New("il sito AMP ha risposto con un altro marittimo: scartato")
	}
	return f, nil
}

// ErrNotFound: il sito non ha trovato nessuno con nome, data di nascita e Marine ID dati.
var ErrNotFound = errors.New("nessun marittimo trovato con questi dati sul sito AMP")
