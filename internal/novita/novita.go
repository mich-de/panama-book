// Package novita legge le novita' ufficiali di Panama (Autoridad Maritima de Panama e Panama Ship Registry) dai loro feed RSS.
//
// Solo fonti ufficiali, scritte nel codice (nessun indirizzo scelto dall'utente o letto da un file): HTTPS obbligatorio, l'host del
// feed e quello di ogni redirect devono essere nell'elenco della fonte, risposta limitata a 2 MB, tempo massimo per fonte, nessun
// cookie e nessun dato dell'utente nella richiesta. Il testo del feed e' trattato come NON fidato: si tolgono i tag HTML, si
// accorcia e si tiene un link solo se e' HTTPS verso un host della stessa fonte. Le notizie non si traducono.
package novita

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Source e' una fonte ufficiale.
type Source struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Lang  string   `json:"lang"` // lingua delle notizie: "es" | "en"
	Site  string   `json:"site"` // sito da mostrare
	Feed  string   `json:"-"`    // indirizzo del feed RSS (HTTPS)
	Hosts []string `json:"-"`    // host ammessi per il feed, i redirect e i link delle notizie
}

// Sources sono le fonti ufficiali: l'AMP (spagnolo) e il Registro delle navi di Panama, che e' dell'AMP (inglese).
var Sources = []Source{
	{ID: "amp", Name: "Autoridad Marítima de Panamá (AMP)", Lang: "es", Site: "amp.gob.pa", Feed: "https://www.amp.gob.pa/feed/",
		Hosts: []string{"www.amp.gob.pa", "amp.gob.pa"}},
	{ID: "psr", Name: "Panama Ship Registry (AMP)", Lang: "en", Site: "panamashipregistry.com", Feed: "https://www.panamashipregistry.com/feed/",
		Hosts: []string{"www.panamashipregistry.com", "panamashipregistry.com"}},
}

const (
	maxBody    = 2 << 20
	maxItems   = 15 // per fonte
	maxSummary = 300
	maxTitle   = 160
	// Timeout e' il tempo massimo per fonte.
	Timeout = 12 * time.Second
)

// Item e' una notizia.
type Item struct {
	Source  string    `json:"source"`
	Lang    string    `json:"lang"`
	Title   string    `json:"title"`
	Summary string    `json:"summary"`
	Link    string    `json:"link,omitempty"`
	Date    time.Time `json:"date"`
}

// Failure e' una fonte che non si e' potuta leggere.
type Failure struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

type rss struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			PubDate     string `xml:"pubDate"`
			Description string `xml:"description"`
		} `xml:"item"`
	} `xml:"channel"`
}

// NewClient e' il client HTTP delle fonti: usa il proxy di sistema (HTTPS_PROXY) e nessun cookie. I redirect si controllano in
// fetchOne (solo verso host ammessi della stessa fonte). Per le prove si puo' passare un altro client a Fetch.
func NewClient() *http.Client { return &http.Client{Timeout: Timeout} }

// Fetch legge tutte le fonti in parallelo. Restituisce le notizie piu' recenti per prime e le fonti che hanno fallito.
func Fetch(ctx context.Context, client *http.Client, srcs []Source) ([]Item, []Failure) {
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		items []Item
		fails []Failure
	)
	for _, s := range srcs {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			got, err := fetchOne(ctx, client, s)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fails = append(fails, Failure{Source: s.ID, Error: shortErr(err)})
				return
			}
			items = append(items, got...)
		}(s)
	}
	wg.Wait()
	sort.SliceStable(items, func(i, j int) bool { return items[i].Date.After(items[j].Date) })
	sort.Slice(fails, func(i, j int) bool { return fails[i].Source < fails[j].Source })
	return items, fails
}

func hostOK(s Source, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	for _, h := range s.Hosts {
		if strings.EqualFold(u.Hostname(), h) {
			return true
		}
	}
	return false
}

func fetchOne(ctx context.Context, client *http.Client, s Source) ([]Item, error) {
	if !hostOK(s, s.Feed) {
		return nil, errors.New("indirizzo del feed non ammesso")
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Feed, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, */*;q=0.1")
	req.Header.Set("User-Agent", "PanamaBook (feed reader)")
	c := *client
	c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return errors.New("troppi reindirizzamenti")
		}
		if !hostOK(s, r.URL.String()) {
			return errors.New("reindirizzamento verso un host non ammesso")
		}
		return nil
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("risposta %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, errors.New("risposta troppo grande")
	}
	return parse(s, body)
}

func parse(s Source, body []byte) ([]Item, error) {
	var f rss
	if err := xml.Unmarshal(body, &f); err != nil {
		return nil, errors.New("feed non leggibile")
	}
	var out []Item
	for _, it := range f.Channel.Items {
		if len(out) >= maxItems {
			break
		}
		date, err := time.Parse(time.RFC1123Z, strings.TrimSpace(it.PubDate))
		if err != nil {
			if date, err = time.Parse(time.RFC1123, strings.TrimSpace(it.PubDate)); err != nil {
				continue
			}
		}
		summary := clean(it.Description, maxSummary)
		title := clean(it.Title, maxTitle)
		if title == "" || (len(title) <= 14 && summary != "") { // «COMUNICADO», «Press Release»: non dicono nulla, meglio l'inizio del testo
			title = clean(it.Description, 90)
		}
		if title == "" {
			continue
		}
		link := strings.TrimSpace(it.Link)
		if !hostOK(s, link) {
			link = ""
		}
		out = append(out, Item{Source: s.ID, Lang: s.Lang, Title: title, Summary: summary, Link: link, Date: date.UTC()})
	}
	return out, nil
}

var (
	reTag   = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpace = regexp.MustCompile(`\s+`)
	// coda tipica di WordPress: «Continuar leyendo COMUNICADO», «The post ... appeared first on ...»
	reTail = regexp.MustCompile(`(?i)(continuar leyendo.*|continue reading.*|the post .*? (?:first )?appeared (?:first )?on .*|read more.*)$`)
)

// clean toglie i tag e la coda di WordPress, decodifica le entita' e accorcia al massimo a max caratteri (con «…»).
func clean(s string, max int) string {
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, " ", " ") // spazio unificatore (&nbsp;): \s di Go non lo conta
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' && r != '\n' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(reSpace.ReplaceAllString(s, " "))
	s = strings.TrimSpace(reTail.ReplaceAllString(s, ""))
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)[:max]
		s = strings.TrimSpace(string(r)) + "…"
	}
	return s
}

// shortErr riduce l'errore a un testo breve e senza indirizzi interni.
func shortErr(err error) string {
	var ue *url.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "tempo scaduto"
	case errors.As(err, &ue):
		if ue.Timeout() {
			return "tempo scaduto"
		}
		if m := ue.Err.Error(); strings.HasPrefix(m, "reindirizzamento") || strings.HasPrefix(m, "troppi") {
			return m
		}
		return "nessuna connessione"
	}
	return err.Error()
}
