package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"scanlibretto/internal/memoria"
	"scanlibretto/internal/novita"
)

// Scheda «Novita' di Panama»: le notizie ufficiali dell'AMP (spagnolo) e del Panama Ship Registry (inglese), lette dai loro feed
// RSS solo quando si apre la scheda o si preme «Aggiorna» (mai in sottofondo e mai all'avvio). Vedi internal/novita per i controlli
// sulle fonti. L'ultima lettura riuscita si conserva in dati\novita.json (nessun dato personale) e si mostra senza Internet.
// Nelle Impostazioni si puo' spegnere (chiave "news": {"enabled": false}): in quel caso il programma non contatta nessun sito.

const (
	newsFresh  = 30 * time.Minute // entro questo tempo non si rilegge da soli
	newsMinGap = 30 * time.Second // «Aggiorna» ripetuto: non si martellano i siti
	newsFile   = "novita.json"
)

type newsCache struct {
	Items []novita.Item `json:"items"`
	At    time.Time     `json:"at"` // ultima lettura riuscita (anche solo di una fonte)
}

type newsState struct {
	mu      sync.Mutex
	loaded  bool
	cache   newsCache
	lastTry time.Time
	fetch   func(ctx context.Context) ([]novita.Item, []novita.Failure) // sostituibile nei test
}

func (a *App) newsPath() string { return filepath.Join(a.cfg.Base, "dati", newsFile) }

func (a *App) newsEnabled() bool {
	if m, ok := a.prefs.load()["news"].(map[string]any); ok {
		if v, ok := m["enabled"].(bool); ok {
			return v
		}
	}
	return true
}

func (a *App) newsLoadLocked() {
	if a.news.loaded {
		return
	}
	a.news.loaded = true
	if b, err := os.ReadFile(a.newsPath()); err == nil && len(b) < 1<<20 {
		var c newsCache
		if json.Unmarshal(b, &c) == nil {
			a.news.cache = c
		}
	}
}

// hNews: GET /api/news[?refresh=1]. Risponde sempre 200 con lo stato (anche senza rete: mostra l'ultima lettura).
func (a *App) hNews(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{"enabled": a.newsEnabled(), "sources": novita.Sources}
	if !resp["enabled"].(bool) {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	n := &a.news
	n.mu.Lock()
	defer n.mu.Unlock()
	a.newsLoadLocked()
	force := r.URL.Query().Get("refresh") == "1"
	stale := time.Since(n.cache.At) > newsFresh
	var fails []novita.Failure
	if (stale || force) && time.Since(n.lastTry) >= newsMinGap {
		n.lastTry = time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), novita.Timeout+3*time.Second)
		fetch := n.fetch
		if fetch == nil {
			c := novita.NewClient()
			fetch = func(ctx context.Context) ([]novita.Item, []novita.Failure) {
				return novita.Fetch(ctx, c, novita.Sources)
			}
		}
		items, f := fetch(ctx)
		cancel()
		fails = f
		if len(f) < len(novita.Sources) { // almeno una fonte ha risposto: si tiene la lettura nuova
			n.cache = newsCache{Items: items, At: time.Now().UTC()}
			if b, err := json.Marshal(n.cache); err == nil {
				if err := memoria.ScriviFile(a.newsPath(), b); err != nil {
					a.logf("novita: non riesco a salvare %s: %v", newsFile, err)
				}
			}
		}
		for _, x := range f {
			a.logf("novita: fonte %s non letta: %s", x.Source, x.Error)
		}
	}
	items := n.cache.Items
	if items == nil {
		items = []novita.Item{}
	}
	resp["items"] = items
	resp["fetched_at"] = n.cache.At
	resp["failed"] = fails
	resp["stale"] = time.Since(n.cache.At) > newsFresh
	writeJSON(w, http.StatusOK, resp)
}
