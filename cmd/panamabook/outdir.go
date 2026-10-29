package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Cartella di output: dove si salvano immagini, JSON, PDF e la «verita» (sottocartella verita\). Di norma e' scans\ accanto al
// programma; l'operatore puo' sceglierne un'altra (un altro disco, una cartella sincronizzata da OneDrive) dalle Impostazioni.
// La scelta sta in dati\impostazioni.json (chiave "outdir", percorso assoluto). Vuota = scans\ predefinita.
// Cambiare cartella NON sposta i file gia' salvati: restano dove sono e la lista «Ultime scansioni» mostra la cartella attuale.
// Le anteprime (dati\anteprima\), la memoria e il profilo di Edge restano sempre accanto al programma: sono file dell'app.

// outDir e' la cartella di output attuale.
func (a *App) outDir() string {
	a.outMu.RLock()
	defer a.outMu.RUnlock()
	return a.outRoot
}

// truthPath e' la sottocartella verita\ della cartella di output.
func (a *App) truthPath() string { return filepath.Join(a.outDir(), "verita") }

// defaultOutDir e' scans\ accanto al programma.
func (a *App) defaultOutDir() string { return filepath.Join(a.cfg.Base, "scans") }

// absPath rende assoluto un percorso relativo alla cartella di lavoro (dove gira libretto.exe).
func (a *App) absPath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(a.cfg.Base, p)
}

// relPath e' il percorso da passare a libretto.exe (che gira nella cartella di lavoro) per un'immagine: relativo per le
// anteprime e per scans\ predefinita, assoluto per una cartella di output scelta dall'operatore.
func (a *App) relPath(dir, file string) string {
	if dir == dirPreview {
		return filepath.Join("dati", "anteprima", file)
	}
	if cur := a.outDir(); cur != a.defaultOutDir() {
		return filepath.Join(cur, file)
	}
	return filepath.Join("scans", file)
}

// checkOutDir verifica la cartella scelta e la crea se manca. Restituisce il percorso pulito.
// Rifiuta: percorsi non assoluti, la cartella dati\ dell'app (anteprime, profilo di Edge) e cartelle in cui non si riesce a scrivere.
func (a *App) checkOutDir(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", fmt.Errorf("cartella non indicata")
	}
	if len(p) > 240 || strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("percorso non valido")
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("serve un percorso completo, per esempio C:\\Documenti\\Libretti")
	}
	p = filepath.Clean(p)
	if inside(p, filepath.Join(a.cfg.Base, "dati")) {
		return "", fmt.Errorf("la cartella dati\\ è del programma: scegline un'altra")
	}
	if err := os.MkdirAll(p, 0o700); err != nil {
		return "", fmt.Errorf("non riesco a creare la cartella: %v", err)
	}
	f, err := os.CreateTemp(p, ".prova-*")
	if err != nil {
		return "", fmt.Errorf("non riesco a scrivere nella cartella: %v", err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return p, nil
}

// inside dice se p e' root o sta dentro root (confronto senza distinguere maiuscole: Windows).
func inside(p, root string) bool {
	p, root = strings.ToLower(filepath.Clean(p)), strings.ToLower(filepath.Clean(root))
	return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
}

// loadOutDir applica la cartella salvata nelle impostazioni all'avvio. Se non e' piu' utilizzabile (disco tolto, cartella
// cancellata) si torna a scans\ e si scrive nel registro: l'operatore lo vede nelle Impostazioni.
func (a *App) loadOutDir() {
	s, _ := a.prefs.load()["outdir"].(string)
	if s == "" {
		return
	}
	p, err := a.checkOutDir(s)
	if err != nil {
		a.logf("cartella di output %q non utilizzabile (%v): uso scans\\", s, err)
		return
	}
	a.outRoot = p
}

// oneDriveDirs sono le cartelle OneDrive del PC (aziendale e personale) che esistono davvero.
func oneDriveDirs() []string {
	var out []string
	seen := map[string]bool{}
	for _, env := range []string{"OneDriveCommercial", "OneDrive", "OneDriveConsumer"} {
		p := os.Getenv(env)
		if p == "" || seen[strings.ToLower(p)] {
			continue
		}
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			seen[strings.ToLower(p)] = true
			out = append(out, p)
		}
	}
	return out
}

// outView e' lo stato mostrato nelle Impostazioni.
func (a *App) outView() map[string]any {
	cur := a.outDir()
	return map[string]any{"path": cur, "default": a.defaultOutDir(), "is_default": cur == a.defaultOutDir(), "onedrive": oneDriveDirs()}
}

// hOutDir: POST /api/outdir {"path": "..."} cambia la cartella di output; "" torna a scans\ predefinita.
// Non si cambia durante una scansione o una lettura (i file in lavorazione andrebbero nella cartella sbagliata).
func (a *App) hOutDir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if a.busyNow() != nil {
		writeErr(w, http.StatusConflict, "occupato", "C'è una scansione o una lettura in corso: attendi la fine e riprova.")
		return
	}
	var saved any // null = predefinita
	p := a.defaultOutDir()
	if strings.TrimSpace(req.Path) != "" {
		var err error
		if p, err = a.checkOutDir(req.Path); err != nil {
			writeErr(w, http.StatusBadRequest, "cartella", err.Error())
			return
		}
		if p != a.defaultOutDir() {
			saved = p
		}
	}
	if _, err := a.prefs.merge(map[string]any{"outdir": saved}); err != nil {
		writeErr(w, http.StatusInternalServerError, "impostazioni", err.Error())
		return
	}
	a.outMu.Lock()
	a.outRoot = p
	a.outMu.Unlock()
	a.logf("cartella di output: %s", p)
	writeJSON(w, http.StatusOK, map[string]any{"outdir": a.outView()})
}

// hOutDirPick: POST /api/outdir/pick apre la finestra di Windows «Cerca cartella» e risponde con la cartella scelta
// ("" = annullato). Non cambia nulla da solo: la scelta si conferma con /api/outdir.
func (a *App) hOutDirPick(w http.ResponseWriter, r *http.Request) {
	p, err := pickFolder("Scegli la cartella dove salvare immagini, JSON e PDF", a.outDir())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "sfoglia", "Non riesco ad aprire la finestra di scelta: scrivi il percorso a mano.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": p})
}
