package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// Impostazioni dell'operatore (lingua, scansione, copia negli appunti): un solo file dati\impostazioni.json accanto al programma.
// Non contiene dati personali. Il file e' un oggetto JSON; il programma conosce solo le chiavi sotto, il resto si ignora.
// Prima stavano nel browser (localStorage del profilo Edge in dati\edge): ora si copiano da un PC all'altro e non si perdono
// se il profilo di Edge viene cancellato.

const (
	prefsFileName = "impostazioni.json"
	prefsMaxBytes = 16 << 10
)

// prefsLangs sono le lingue dell'interfaccia: inglese (predefinita), spagnolo (lingua di Panama), italiano.
var prefsLangs = map[string]bool{"en": true, "es": true, "it": true}

// prefsKeys sono le chiavi accettate: lang (testo), scan (driver, dpi, lunghezza, margine), copy (formato di copia) e outdir
// (cartella di output, testo: si cambia solo con /api/outdir, che la verifica) e news ({"enabled": bool}, scheda Novita').
var prefsKeys = map[string]bool{"lang": true, "scan": true, "copy": true, "outdir": true, "news": true}

type prefsStore struct {
	mu   sync.Mutex
	path string
}

func newPrefsStore(base string) *prefsStore {
	return &prefsStore{path: filepath.Join(base, "dati", prefsFileName)}
}

// load legge il file; se manca o e' rovinato restituisce un oggetto vuoto (il programma parte con i valori predefiniti).
func (p *prefsStore) load() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loadLocked()
}

func (p *prefsStore) loadLocked() map[string]any {
	out := map[string]any{}
	b, err := os.ReadFile(p.path)
	if err != nil || len(b) > prefsMaxBytes {
		return out
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return out
	}
	for k, v := range m {
		if prefsKeys[k] {
			out[k] = v
		}
	}
	if l, ok := out["lang"].(string); ok && !prefsLangs[l] {
		delete(out, "lang")
	}
	return out
}

// merge unisce le chiavi ricevute a quelle salvate e riscrive il file in modo atomico (file temporaneo + rinomina).
func (p *prefsStore) merge(in map[string]any) (map[string]any, error) {
	for k, v := range in {
		if !prefsKeys[k] {
			return nil, fmt.Errorf("impostazione sconosciuta: %q", k)
		}
		if k == "lang" {
			if l, ok := v.(string); !ok || !prefsLangs[l] {
				return nil, fmt.Errorf("lingua non valida")
			}
		} else if k == "outdir" {
			if _, ok := v.(string); !ok && v != nil {
				return nil, fmt.Errorf("cartella di output non valida")
			}
		} else if _, ok := v.(map[string]any); !ok && v != nil {
			return nil, fmt.Errorf("impostazione %q non valida", k)
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cur := p.loadLocked()
	for k, v := range in {
		if v == nil {
			delete(cur, k)
		} else {
			cur[k] = v
		}
	}
	b, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(b) > prefsMaxBytes {
		return nil, fmt.Errorf("impostazioni troppo grandi")
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return nil, err
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, p.path); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	return cur, nil
}

// hPrefsSet: POST /api/prefs con un oggetto JSON con le sole chiavi da cambiare ("null" la toglie). Risponde con tutte le impostazioni.
func (a *App) hPrefsSet(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	if !readJSON(w, r, &in) {
		return
	}
	if _, ok := in["outdir"]; ok { // la cartella di output si cambia solo da /api/outdir (verifica e crea la cartella)
		writeErr(w, http.StatusBadRequest, "impostazioni", "impostazione non modificabile da qui: \"outdir\"")
		return
	}
	out, err := a.prefs.merge(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "impostazioni", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"prefs": out})
}
