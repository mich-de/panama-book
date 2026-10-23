package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"scanlibretto/internal/memoria"
)

//go:embed web
var webFS embed.FS

const version = "0.1 (bozza)"

// Valori consigliati per la scansione del libretto (tools\scan-twain.cmd).
const (
	defDriver    = "wia"
	defDPI       = 300
	defLenMM     = 190.0
	defTopMargin = 2.0
)

// config è la configurazione del programma.
type config struct {
	Base     string   // cartella di lavoro: libretto.exe, scans\, dati\
	Exe      string   // percorso di libretto.exe
	Demo     string   // se non vuoto: immagine da usare al posto della scansione (niente scanner)
	ExtraEnv []string // variabili d'ambiente in più per il sottoprocesso (test)
	NoWindow bool     // non aprire la finestra e non uscire alla chiusura
	// PDFFallback è l'eseguibile da provare per il comando "pdf" se libretto.exe non lo conosce ancora.
	PDFFallback string
}

// App è il server locale con tutto il suo stato.
type App struct {
	cfg       config
	scansDir  string
	truthDir  string
	mem       *memoria.Archivio
	token     string
	demoDelay time.Duration

	allowedHosts   map[string]bool
	allowedOrigins map[string]bool

	jobsMu sync.Mutex
	jobs   map[string]*Job
	active *Job

	docsMu   sync.Mutex
	docs     map[string]*Doc
	docOrder []string
	shownDoc string // documento mostrato nella finestra ("" dopo "Ricomincia da capo")

	pdfMu   sync.Mutex
	pdfPath string // eseguibile che sa fare "pdf" (ricordato dopo la prima verifica)

	lg       *logger     // registro degli eventi (nil nei test)
	haveEdge atomic.Bool // la finestra è un Edge lanciato da noi: la sua fine è il segnale di chiusura

	lastBeat atomic.Int64 // ultimo heartbeat (UnixNano), 0 = mai
	byeAt    atomic.Int64 // ultimo "chiudo" (UnixNano)
	clock    atomic.Int64 // ultimo istante assegnato da stamp
	exitOnce sync.Once
	exitCh   chan struct{}
}

func newApp(cfg config) (*App, error) {
	mem, err := memoria.Apri(filepath.Join(cfg.Base, "dati", "memoria.json"))
	if err != nil {
		return nil, err
	}
	a := &App{
		cfg: cfg, scansDir: filepath.Join(cfg.Base, "scans"), truthDir: filepath.Join(cfg.Base, "scans", "verita"),
		mem: mem, token: randomHex(16), demoDelay: 1200 * time.Millisecond,
		jobs: map[string]*Job{}, docs: map[string]*Doc{}, exitCh: make(chan struct{}),
		allowedHosts: map[string]bool{}, allowedOrigins: map[string]bool{},
	}
	return a, nil
}

// setAddr dice al server su quale porta ascolta: serve ai controlli Host e Origin.
func (a *App) setAddr(port string) {
	for _, h := range []string{"127.0.0.1", "localhost"} {
		a.allowedHosts[h+":"+port] = true
		a.allowedOrigins["http://"+h+":"+port] = true
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// requestExit chiede di uscire (una volta sola).
func (a *App) requestExit() { a.exitOnce.Do(func() { close(a.exitCh) }) }

func (a *App) putDoc(d *Doc) {
	a.docsMu.Lock()
	defer a.docsMu.Unlock()
	a.docs[d.ID] = d
	a.shownDoc = d.ID
	a.docOrder = append(a.docOrder, d.ID)
	for len(a.docOrder) > 16 {
		delete(a.docs, a.docOrder[0])
		a.docOrder = a.docOrder[1:]
	}
}

func (a *App) getDoc(id string) *Doc {
	a.docsMu.Lock()
	defer a.docsMu.Unlock()
	return a.docs[id]
}

// Handler costruisce il gestore HTTP, protetto da controllo Host, Origin e token.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServerFS(sub))
	mux.HandleFunc("GET /api/state", a.hState)
	mux.HandleFunc("POST /api/heartbeat", a.hHeartbeat)
	mux.HandleFunc("POST /api/bye", a.hBye)
	mux.HandleFunc("POST /api/clientlog", a.hClientLog)
	mux.HandleFunc("GET /api/scans", a.hScans)
	mux.HandleFunc("GET /api/image", a.hImage)
	mux.HandleFunc("POST /api/scan", a.hScan)
	mux.HandleFunc("POST /api/open", a.hOpen)
	mux.HandleFunc("POST /api/preview", a.hPreviewScan)
	mux.HandleFunc("GET /api/previews", a.hPreviews)
	mux.HandleFunc("POST /api/preview/discard", a.hDiscard)
	mux.HandleFunc("GET /api/jobs/{id}", a.hJob)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", a.hCancel)
	mux.HandleFunc("GET /api/doc/{id}", a.hDoc)
	mux.HandleFunc("POST /api/save", a.hSave)
	mux.HandleFunc("POST /api/pdf", a.hPDF)
	mux.HandleFunc("POST /api/reset", a.hReset)
	mux.HandleFunc("POST /api/preview-name", a.hPreviewName)
	mux.HandleFunc("GET /api/suggest", a.hSuggest)
	mux.HandleFunc("GET /api/memory", a.hMemory)
	mux.HandleFunc("POST /api/memory/forget-field", a.hForgetField)
	mux.HandleFunc("POST /api/memory/forget", a.hForget)
	return a.guard(mux)
}

// guard impone: Host noto (contro il DNS rebinding), Origin noto se presente, e sulle API il token di sessione.
func (a *App) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w0 http.ResponseWriter, r *http.Request) {
		w := &statusWriter{ResponseWriter: w0, code: http.StatusOK}
		start := time.Now()
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				a.logf("PANIC nel gestore %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				if w.code == http.StatusOK {
					writeErr(w, http.StatusInternalServerError, "interno", "Errore interno del programma: i dettagli sono in dati\\panamabook.log.")
				}
				return
			}
			if w.code >= 400 || !quietPath(r.Method, r.URL.Path) {
				a.logf("API %s %s -> %d (%d ms)", r.Method, r.URL.Path, w.code, time.Since(start).Milliseconds())
			}
		}()
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob: data:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if !a.allowedHosts[r.Host] {
			http.Error(w, "host non ammesso", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !a.allowedOrigins[o] {
			http.Error(w, "origine non ammessa", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if s := r.Header.Get("Sec-Fetch-Site"); s == "cross-site" {
				http.Error(w, "richiesta da un altro sito", http.StatusForbidden)
				return
			}
			tok := r.Header.Get("X-PB-Token")
			if tok == "" {
				tok = r.URL.Query().Get("t")
			}
			if subtle.ConstantTimeCompare([]byte(tok), []byte(a.token)) != 1 {
				http.Error(w, "token mancante o errato", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, errCode, msg string) {
	writeJSON(w, code, map[string]any{"error": msg, "code": errCode})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "json", "richiesta non valida")
		return false
	}
	return true
}

// ---- stato, heartbeat -----------------------------------------------------------------------------------------------

func (a *App) hState(w http.ResponseWriter, r *http.Request) {
	_, exeErr := os.Stat(a.cfg.Exe)
	busy := ""
	if j := a.busyNow(); j != nil {
		busy = j.ID
	}
	var last any
	a.docsMu.Lock()
	var lastDoc *Doc
	lastDoc = a.docs[a.shownDoc]
	a.docsMu.Unlock()
	if lastDoc != nil { // dopo un ricaricamento della finestra si ritrova l'ultimo documento letto
		last = a.docView(lastDoc)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"last_doc": last,
		"version":  version, "demo": a.cfg.Demo != "", "demo_name": filepath.Base(a.cfg.Demo),
		"defaults":      map[string]any{"driver": defDriver, "dpi": defDPI, "len_mm": defLenMM, "top_margin_mm": defTopMargin},
		"suggested":     a.suggestName(),
		"libretto_ok":   exeErr == nil,
		"libretto_name": filepath.Base(a.cfg.Exe),
		"busy_job":      busy,
	})
}

func (a *App) hHeartbeat(w http.ResponseWriter, r *http.Request) {
	now := a.stamp()
	if prev := a.lastBeat.Swap(now); prev == 0 {
		a.logf("finestra collegata (primo heartbeat)")
	} else if gap := time.Duration(now - prev); gap > 20*time.Second {
		// la pagina in secondo piano rallenta i timer: il registro lo mostra
		a.logf("heartbeat ripreso dopo %d s (pagina in secondo piano?)", int(gap.Seconds()))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// hClientLog registra un errore della pagina (JavaScript) nel registro del programma. Solo testo, tagliato.
func (a *App) hClientLog(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Msg string `json:"msg"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.Msg) > 500 {
		req.Msg = req.Msg[:500]
	}
	a.logf("ERRORE DELLA PAGINA: %s", strings.ReplaceAll(req.Msg, "\n", " "))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) hBye(w http.ResponseWriter, r *http.Request) {
	a.byeAt.Store(a.stamp())
	a.logf("la pagina ha dichiarato di chiudersi (bye)")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// stamp da' un istante strettamente crescente: heartbeat e bye vicini non si confondono con l'orologio poco preciso di Windows.
func (a *App) stamp() int64 {
	for {
		prev := a.clock.Load()
		n := time.Now().UnixNano()
		if n <= prev {
			n = prev + 1
		}
		if a.clock.CompareAndSwap(prev, n) {
			return n
		}
	}
}

// shouldExit dice se la finestra risulta chiusa, e perche'. startedAt e' l'avvio del server. Regole prudenti (un'uscita a
// sproposito mentre l'operatore lavora e' il guaio peggiore):
//   - mai con un lavoro in corso (soprattutto durante la scansione fisica);
//   - mai se la pagina ha dato un heartbeat negli ultimi 60 s (una finestra coperta o in secondo piano rallenta i timer);
//   - con un Edge lanciato da noi la chiusura vera e' la fine di quel processo (gestita da run); qui resta solo la rete di
//     sicurezza: nessun heartbeat per 3 minuti;
//   - senza Edge nostro (browser predefinito) "bye" della pagina accelera l'uscita, ma solo se dopo 15 s non e' tornato nessun heartbeat.
func (a *App) shouldExit(now, startedAt time.Time) (bool, string) {
	if a.busyNow() != nil {
		return false, ""
	}
	last, bye := a.lastBeat.Load(), a.byeAt.Load()
	n := now.UnixNano()
	if last != 0 && n-last < int64(60*time.Second) && !(bye > last && !a.haveEdge.Load() && n-bye > int64(15*time.Second)) {
		return false, ""
	}
	if bye != 0 && bye > last && !a.haveEdge.Load() && n-bye > int64(15*time.Second) {
		return true, fmt.Sprintf("la pagina ha dichiarato la chiusura (bye) da %d s e non ha piu' dato segni di vita", (n-bye)/int64(time.Second))
	}
	if last == 0 {
		if now.Sub(startedAt) > 3*time.Minute {
			return true, "la finestra non si e' mai collegata (3 minuti)"
		}
		return false, ""
	}
	if n-last > int64(180*time.Second) {
		return true, fmt.Sprintf("heartbeat assente da %d s", (n-last)/int64(time.Second))
	}
	return false, ""
}

// ---- elenco immagini, immagine -------------------------------------------------------------------------------------

// ScanItem è una riga della cronologia / del selettore delle immagini salvate.
type ScanItem struct {
	File  string    `json:"file"`
	Name  string    `json:"name"`
	Time  string    `json:"time"`
	Size  int64     `json:"size"`
	Saved bool      `json:"saved"` // c'è scans\NOME.json
	Truth bool      `json:"truth"` // c'è scans\verita\NOME.json
	mod   time.Time `json:"-"`
}

func isImageName(n string) bool {
	switch strings.ToLower(filepath.Ext(n)) {
	case ".png", ".jpg", ".jpeg":
		return true
	}
	return false
}

func (a *App) hScans(w http.ResponseWriter, r *http.Request) {
	ents, err := os.ReadDir(a.scansDir)
	items := []ScanItem{}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		writeErr(w, http.StatusInternalServerError, "scans", "cartella scans non leggibile")
		return
	}
	for _, e := range ents {
		if e.IsDir() || !isImageName(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		it := ScanItem{File: e.Name(), Name: name, Time: info.ModTime().Format(time.RFC3339), Size: info.Size(), mod: info.ModTime()}
		it.Saved = fileExists(filepath.Join(a.scansDir, name+".json"))
		it.Truth = fileExists(filepath.Join(a.truthDir, name+".json"))
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// safeScanFile controlla che il nome sia un'immagine direttamente dentro scans\ (niente percorsi, niente "..").
func (a *App) safeScanFile(name string) (string, bool) {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\:`) || strings.HasPrefix(name, ".") || !isImageName(name) {
		return "", false
	}
	p := filepath.Join(a.scansDir, name)
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return "", false
	}
	return p, true
}

func (a *App) hImage(w http.ResponseWriter, r *http.Request) {
	p, ok := a.safeImageView(r.URL.Query().Get("dir"), r.URL.Query().Get("file"))
	if !ok {
		writeErr(w, http.StatusNotFound, "file", "immagine non trovata")
		return
	}
	http.ServeFile(w, r, p)
}

// ---- lavori --------------------------------------------------------------------------------------------------------

func (a *App) hScan(w http.ResponseWriter, r *http.Request) {
	var s scanSettings
	if !readJSON(w, r, &s) {
		return
	}
	if err := s.validate(); err != nil {
		writeErr(w, http.StatusBadRequest, "impostazioni", err.Error())
		return
	}
	if _, err := os.Stat(a.cfg.Exe); err != nil {
		writeErr(w, http.StatusFailedDependency, "manca_libretto", "Manca "+filepath.Base(a.cfg.Exe)+" accanto a PanamaBook.exe.")
		return
	}
	if fileExists(filepath.Join(a.scansDir, s.Name+".png")) {
		writeErr(w, http.StatusConflict, "esiste", "Esiste già una scansione con questo nome: scegline un altro.")
		return
	}
	j, err := a.startJob("scan", a.cfg.Demo != "", func(ctx context.Context, j *Job) { a.doScan(ctx, j, s) })
	if err != nil {
		writeErr(w, http.StatusConflict, "occupato", "Un altro lavoro è ancora in corso: attendi la fine.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job": j.ID})
}

func (a *App) hOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		File string `json:"file"`
		// TopMargin: mm sopra la scritta REPUBLICA DE PANAMA nella pagina dati ritagliata (solo anteprime; predefinito 2)
		TopMargin *float64 `json:"top_margin_mm"`
		Dir       string   `json:"dir"` // "preview" = anteprima di dati\anteprima\; altrimenti scans\
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Dir != dirPreview {
		req.Dir = dirScans
	}
	if _, ok := a.safeImage(req.Dir, req.File); !ok {
		writeErr(w, http.StatusNotFound, "file", "Immagine non trovata.")
		return
	}
	if _, err := os.Stat(a.cfg.Exe); err != nil {
		writeErr(w, http.StatusFailedDependency, "manca_libretto", "Manca "+filepath.Base(a.cfg.Exe)+" accanto a PanamaBook.exe.")
		return
	}
	margin := defTopMargin
	if req.TopMargin != nil && *req.TopMargin >= -1 && *req.TopMargin <= 20 {
		margin = *req.TopMargin
	}
	j, err := a.startJob("open", false, func(ctx context.Context, j *Job) { a.doExtractIn(ctx, j, req.Dir, req.File, margin) })
	if err != nil {
		writeErr(w, http.StatusConflict, "occupato", "Un altro lavoro è ancora in corso: attendi la fine.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job": j.ID})
}

func (a *App) getJob(id string) *Job {
	a.jobsMu.Lock()
	defer a.jobsMu.Unlock()
	return a.jobs[id]
}

func (a *App) hJob(w http.ResponseWriter, r *http.Request) {
	j := a.getJob(r.PathValue("id"))
	if j == nil {
		writeErr(w, http.StatusNotFound, "job", "lavoro sconosciuto")
		return
	}
	j.mu.Lock()
	v := JobView{ID: j.ID, Kind: j.Kind, Phase: j.Phase, Message: j.Message, Cancelable: j.Cancelable, Demo: j.Demo,
		Log: append([]string{}, j.Log...), Error: j.Err}
	end := time.Now()
	if !j.Finished.IsZero() {
		end = j.Finished
	}
	v.ElapsedSec = int(end.Sub(j.Started).Seconds())
	v.Done = j.Phase == phaseDone || j.Phase == phaseError || j.Phase == phaseCancelled
	v.Preview = j.Preview
	docID, phase := j.DocID, j.Phase
	j.mu.Unlock()
	if phase == phaseDone && docID != "" {
		if d := a.getDoc(docID); d != nil {
			dv := a.docView(d) // prende il lucchetto del documento: nome e campi sono letti in modo coerente
			v.Doc = &dv
		}
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *App) hCancel(w http.ResponseWriter, r *http.Request) {
	j := a.getJob(r.PathValue("id"))
	if j == nil {
		writeErr(w, http.StatusNotFound, "job", "lavoro sconosciuto")
		return
	}
	j.mu.Lock()
	ok := j.Cancelable && j.Phase == phaseReading
	j.mu.Unlock()
	if !ok {
		writeErr(w, http.StatusConflict, "non_annullabile", "Questo lavoro non si può annullare adesso (la scansione fisica va attesa fino in fondo).")
		return
	}
	j.cancel()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// cancelActive termina un'eventuale lettura in corso (non la scansione fisica): per l'uscita dal programma.
func (a *App) cancelActive() {
	if j := a.busyNow(); j != nil && !j.scanning() {
		j.cancel()
	}
}

func (a *App) hDoc(w http.ResponseWriter, r *http.Request) {
	d := a.getDoc(r.PathValue("id"))
	if d == nil {
		writeErr(w, http.StatusNotFound, "doc", "documento sconosciuto")
		return
	}
	writeJSON(w, http.StatusOK, a.docView(d))
}

func (a *App) docView(d *Doc) DocView {
	d.mu.Lock()
	defer d.mu.Unlock()
	dv := d.view(a.mem, a.jsonExists(d.Name))
	dv.ImageURL = imageURL(d.Dir, d.File)
	if d.Dir == dirPreview && d.PageFile != "" {
		dv.PageURL = imageURL(dirPreview, d.PageFile)
	}
	return dv
}

func (a *App) jsonExists(name string) bool {
	return fileExists(filepath.Join(a.scansDir, name+".json"))
}

// suggestName propone il prossimo nome libero "libretto-NNN" (numero progressivo, nessun file con quel nome in scans).
func (a *App) suggestName() string {
	max := 0
	ents, _ := os.ReadDir(a.scansDir)
	for _, e := range ents {
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if rest, ok := strings.CutPrefix(name, "libretto-"); ok && rest != "" {
			n := 0
			for _, c := range rest {
				if c < '0' || c > '9' || n > 1_000_000 {
					n = -1
					break
				}
				n = n*10 + int(c-'0')
			}
			if n > max {
				max = n
			}
		}
	}
	for i := max + 1; ; i++ {
		name := fmt.Sprintf("libretto-%03d", i)
		if !fileExists(filepath.Join(a.scansDir, name+".png")) && !fileExists(filepath.Join(a.scansDir, name+".jpg")) &&
			!fileExists(filepath.Join(a.scansDir, name+".json")) {
			return name
		}
	}
}

func (a *App) hSuggest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"name": a.suggestName()})
}

// hReset dimentica quale documento e' mostrato (un ricaricamento non lo ripropone). Non cancella nessun file ne' la memoria.
// Rifiutato durante la scansione fisica; una lettura in corso viene annullata.
func (a *App) hReset(w http.ResponseWriter, r *http.Request) {
	if a.scanningNow() {
		writeErr(w, http.StatusConflict, "scansione_in_corso", "La scansione è in corso: attendi la fine prima di ricominciare.")
		return
	}
	a.cancelActive()
	a.docsMu.Lock()
	a.shownDoc = ""
	a.docsMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"name": a.suggestName()})
}
