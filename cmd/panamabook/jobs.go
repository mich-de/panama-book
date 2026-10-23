package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Fasi di un lavoro.
const (
	phaseScanning  = "scanning"  // lo scanner sta lavorando: NIENTE annulla
	phaseReading   = "reading"   // estrazione dei dati: si può annullare
	phaseDone      = "done"      // finito bene: c'è un documento
	phaseError     = "error"     // finito male: c'è un messaggio
	phaseCancelled = "cancelled" // annullato dall'operatore
)

var errBusy = errors.New("un altro lavoro è ancora in corso")

// Job è un lavoro asincrono: scansione e lettura, oppure solo lettura.
type Job struct {
	mu         sync.Mutex
	ID         string
	Kind       string // "scan" | "open"
	Phase      string
	Message    string
	Cancelable bool
	Demo       bool
	Started    time.Time
	Finished   time.Time
	Log        []string
	DocID      string
	Preview    *PreviewView // lavoro "Anteprima" finito: l'immagine temporanea creata
	Err        string
	cancel     context.CancelFunc
	lg         *logger
}

// JobView è lo stato di un lavoro per l'interfaccia.
type JobView struct {
	ID         string       `json:"id"`
	Kind       string       `json:"kind"`
	Phase      string       `json:"phase"`
	Message    string       `json:"message"`
	Cancelable bool         `json:"cancelable"`
	Demo       bool         `json:"demo"`
	ElapsedSec int          `json:"elapsed_s"`
	Done       bool         `json:"done"`
	Log        []string     `json:"log"`
	Error      string       `json:"error,omitempty"`
	Doc        *DocView     `json:"doc,omitempty"`
	Preview    *PreviewView `json:"preview,omitempty"`
}

func (j *Job) set(phase, msg string, cancelable bool) {
	j.mu.Lock()
	j.Phase, j.Message, j.Cancelable = phase, msg, cancelable
	j.mu.Unlock()
	j.lg.printf("lavoro %s (%s): fase %s", j.ID, j.Kind, phase)
}

func (j *Job) addLog(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	j.mu.Lock()
	j.Log = append(j.Log, line)
	if len(j.Log) > 40 {
		j.Log = j.Log[len(j.Log)-40:]
	}
	j.mu.Unlock()
}

func (j *Job) finish(phase, msg, errText, docID string) {
	j.mu.Lock()
	j.Phase, j.Message, j.Err, j.DocID, j.Cancelable = phase, msg, errText, docID, false
	j.Finished = time.Now()
	j.mu.Unlock()
	if errText != "" {
		j.lg.printf("lavoro %s (%s): FINITO fase %s: %s", j.ID, j.Kind, phase, errText)
	} else {
		j.lg.printf("lavoro %s (%s): finito fase %s", j.ID, j.Kind, phase)
	}
}

func (j *Job) finished() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.Phase == phaseDone || j.Phase == phaseError || j.Phase == phaseCancelled
}

func (j *Job) scanning() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.Phase == phaseScanning
}

// startJob avvia un lavoro. Uno solo alla volta (c'è un solo scanner e le letture sono pesanti).
func (a *App) startJob(kind string, demo bool, run func(ctx context.Context, j *Job)) (*Job, error) {
	a.jobsMu.Lock()
	if a.active != nil && !a.active.finished() {
		a.jobsMu.Unlock()
		return nil, errBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job{ID: randomHex(6), Kind: kind, Phase: "queued", Started: time.Now(), Demo: demo, cancel: cancel, lg: a.lg}
	a.active = j
	a.jobs[j.ID] = j
	if len(a.jobs) > 20 { // tieni solo gli ultimi
		for id, o := range a.jobs {
			if o != j && o.finished() && time.Since(o.Finished) > time.Minute {
				delete(a.jobs, id)
			}
		}
	}
	a.jobsMu.Unlock()
	a.logf("lavoro %s (%s) avviato%s", j.ID, kind, map[bool]string{true: " [PROVA, nessuno scanner]", false: ""}[demo])
	go func() {
		defer cancel()
		// un panic nel lavoro non deve chiudere il programma: diventa un errore visibile nell'interfaccia
		defer a.recoverTo("lavoro "+kind, func(msg string) { j.finish(phaseError, "", msg, "") })
		run(ctx, j)
	}()
	return j, nil
}

// scanningNow dice se lo scanner sta lavorando: finché è vero il programma non esce.
func (a *App) scanningNow() bool {
	a.jobsMu.Lock()
	defer a.jobsMu.Unlock()
	return a.active != nil && a.active.scanning()
}

// busyNow dice se c'è un lavoro in corso di qualunque tipo.
func (a *App) busyNow() *Job {
	a.jobsMu.Lock()
	defer a.jobsMu.Unlock()
	if a.active != nil && !a.active.finished() {
		return a.active
	}
	return nil
}

// ---- esecuzione di libretto.exe -----------------------------------------------------------------------------------

// limitedBuffer tiene al massimo max byte (l'uscita JSON è piccola: oltre è un guasto).
type limitedBuffer struct {
	bytes.Buffer
	max      int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		b.overflow = true
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

// runLibretto lancia libretto.exe nella cartella di lavoro. Lo stderr va riga per riga a onLine; restituisce lo stdout e il
// codice di uscita (-1 se non è partito). Con ctx annullato il processo viene terminato.
func (a *App) runLibretto(ctx context.Context, args []string, onLine func(string)) ([]byte, int, error) {
	return a.runExe(ctx, a.cfg.Exe, args, onLine)
}

// runExe è runLibretto con l'eseguibile a scelta (libretto.exe oppure il candidato con il comando pdf).
func (a *App) runExe(ctx context.Context, exe string, args []string, onLine func(string)) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = a.cfg.Base
	cmd.Env = append(os.Environ(), a.cfg.ExtraEnv...)
	cmd.WaitDelay = 3 * time.Second
	hideWindow(cmd)
	out := &limitedBuffer{max: 8 << 20}
	cmd.Stdout = out
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, -1, err
	}
	verb := ""
	if len(args) > 0 {
		verb = args[0]
	}
	t0 := time.Now()
	if err := cmd.Start(); err != nil {
		a.logf("sottoprocesso %s %s: AVVIO FALLITO: %v", filepath.Base(exe), verb, err)
		return nil, -1, fmt.Errorf("non riesco ad avviare %s: %w", filepath.Base(exe), err)
	}
	a.logf("sottoprocesso %s %s avviato (pid %d)", filepath.Base(exe), verb, cmd.Process.Pid)
	sc := bufio.NewScanner(io.LimitReader(errPipe, 1<<20))
	sc.Buffer(make([]byte, 64<<10), 256<<10)
	for sc.Scan() {
		if onLine != nil {
			onLine(sc.Text())
		}
	}
	_, _ = io.Copy(io.Discard, errPipe) // mai lasciare il sottoprocesso bloccato su uno stderr non svuotato
	werr := cmd.Wait()
	code := 0
	if werr != nil {
		code = -1
		var ee *exec.ExitError
		if errors.As(werr, &ee) {
			code = ee.ExitCode()
		}
	}
	a.logf("sottoprocesso %s %s terminato: codice %d dopo %d ms", filepath.Base(exe), verb, code, time.Since(t0).Milliseconds())
	if out.overflow {
		return nil, -1, errors.New("uscita di libretto.exe troppo grande")
	}
	if werr != nil {
		var ee *exec.ExitError
		if errors.As(werr, &ee) {
			return out.Bytes(), ee.ExitCode(), nil
		}
		return out.Bytes(), -1, werr
	}
	return out.Bytes(), 0, nil
}

// lastErrLine cerca nel registro l'ultima riga "errore: ..." (o l'ultima riga) di libretto.exe.
func lastErrLine(log []string) string {
	for i := len(log) - 1; i >= 0; i-- {
		if rest, ok := strings.CutPrefix(log[i], "errore:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	if len(log) > 0 {
		return log[len(log)-1]
	}
	return ""
}

// ---- scansione e lettura ------------------------------------------------------------------------------------------

// scanSettings sono le impostazioni di scansione scelte dall'operatore.
type scanSettings struct {
	Name      string  `json:"name"`
	Driver    string  `json:"driver"`
	DPI       int     `json:"dpi"`
	LenMM     float64 `json:"len_mm"`
	TopMargin float64 `json:"top_margin_mm"`
}

var reName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,59}$`)

// validate controlla le impostazioni (arrivano dalla rete locale: niente fiducia).
func (s *scanSettings) validate() error {
	if !reName.MatchString(s.Name) || strings.Contains(s.Name, "..") {
		return errors.New("nome non valido: usa lettere, cifre, punto, trattino o sottolineato (massimo 60 caratteri)")
	}
	return s.validateParams()
}

// validateParams controlla le impostazioni dello scanner (senza il nome).
func (s *scanSettings) validateParams() error {
	if s.Driver != "twain" && s.Driver != "wia" {
		return errors.New("driver non valido: scegli twain o wia")
	}
	if s.DPI < 75 || s.DPI > 600 {
		return errors.New("risoluzione non valida: tra 75 e 600 dpi")
	}
	if s.LenMM < 30 || s.LenMM > 356 {
		return errors.New("lunghezza non valida: tra 30 e 356 mm")
	}
	if s.TopMargin < -1 || s.TopMargin > 20 {
		return errors.New("margine non valido: tra -1 e 20 mm")
	}
	return nil
}

func (a *App) scanArgs(s scanSettings) []string {
	return a.scanArgsTo(s, filepath.Join("scans", s.Name+".png"), "")
}

// scanArgsTo sono gli argomenti di "libretto.exe scan" con le impostazioni consigliate, scrivendo nel file rel (relativo alla cartella di lavoro).
// Con raw non vuoto aggiunge "-raw RAW": libretto salva anche la scansione GREZZA (cosi' come esce dal driver, prima di ritaglio e controllo del vuoto).
func (a *App) scanArgsTo(s scanSettings, rel, raw string) []string {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	// Ricetta collaudata (tools\scan-wia.cmd) con WIA: nessun -from e nessun -rotate (l'immagine esce gia' dritta).
	// Con TWAIN (tools\scan-twain.cmd) l'immagine esce capovolta: -from 0 -rotate 180.
	args := []string{"scan", "-yes", "-o", rel, "-driver", s.Driver, "-dpi", strconv.Itoa(s.DPI), "-len", f(s.LenMM)}
	if s.Driver == "twain" {
		args = append(args, "-from", "0", "-rotate", "180")
	}
	args = append(args, "-top-margin", f(s.TopMargin))
	if raw != "" {
		args = append(args, "-raw", raw)
	}
	return args
}

// doScan è il lavoro "Scansiona": scansione (o copia demo) e poi lettura dei dati.
func (a *App) doScan(ctx context.Context, j *Job, s scanSettings) {
	rel := filepath.Join("scans", s.Name+".png")
	dst := filepath.Join(a.cfg.Base, rel)
	j.set(phaseScanning, "Scansione in corso: lo scanner sta lavorando. Non toccare il libretto e attendi (circa 10 secondi).", false)
	if a.cfg.Demo != "" {
		j.set(phaseScanning, "PROVA (demo): nessuno scanner viene usato, si copia l'immagine di prova.", false)
		select {
		case <-time.After(a.demoDelay):
		case <-ctx.Done(): // in demo non c'è hardware: annullare è innocuo
		}
		if err := copyNew(a.cfg.Demo, dst); err != nil {
			j.finish(phaseError, "", "Immagine di prova non copiata: "+err.Error(), "")
			return
		}
	} else {
		_ = os.MkdirAll(a.scansDir, 0o700)
		// MAI ctx qui: durante il trasferimento fisico non si interrompe nulla.
		_, code, err := a.runLibretto(context.Background(), a.scanArgs(s), j.addLog)
		if err != nil || code != 0 {
			j.finish(phaseError, "", scanErrorText(code, err, j), "")
			return
		}
		if st, err := os.Stat(dst); err != nil || st.Size() == 0 {
			j.finish(phaseError, "", "La scansione è finita ma il file non c'è: controlla lo scanner e riprova.", "")
			return
		}
	}
	a.doExtractIn(ctx, j, dirScans, s.Name+".png", s.TopMargin)
}

// scanErrorText traduce l'esito di libretto.exe scan in una frase per l'operatore.
func scanErrorText(code int, err error, j *Job) string {
	j.mu.Lock()
	last := lastErrLine(j.Log)
	j.mu.Unlock()
	switch {
	case err != nil:
		return err.Error()
	case code == 3:
		return "Nessun libretto nello scanner. Inseriscilo dal bordo con le righe MRZ (<<<), faccia verso l'alto, centrato e dritto, spinto fino in fondo, e riprova."
	case code == 4:
		return "Inceppamento. NON tirare con forza: apri prima il coperchio dello scanner, poi estrai il libretto."
	}
	msg := "La scansione non è riuscita"
	if last != "" {
		msg += " (" + last + ")"
	}
	return msg + ". Chiudi gli altri programmi che usano lo scanner e riprova."
}

// doExtract lancia "libretto extract -i scans\FILE" e, se va bene, registra il documento nel lavoro.
func (a *App) doExtract(ctx context.Context, j *Job, file string) {
	a.doExtractIn(ctx, j, dirScans, file, defTopMargin)
}

// doExtractIn legge un'immagine di scans\ (dir "scans") o un'anteprima di dati\anteprima\ (dir "preview").
// Su un'anteprima si chiede anche la SOLA PAGINA DATI ritagliata (-page-out, 2 mm sopra la scritta REPUBLICA DE PANAMA, dritta, senza
// bianchi): libretto la salva solo se trova le righe MRZ. E' l'immagine che "Conferma e salva" salvera' e da cui si fa il PDF.
func (a *App) doExtractIn(ctx context.Context, j *Job, dir, file string, topMargin float64) {
	start := time.Now()
	j.set(phaseReading, "Lettura dei dati in corso (da 10 a 25 secondi). Puoi annullare.", true)
	out, msg, cancelled := a.extractCached(ctx, j, dir, file, topMargin)
	if cancelled {
		j.finish(phaseCancelled, "Lettura annullata.", "", "")
		return
	}
	if msg != "" {
		j.finish(phaseError, "", msg, "")
		return
	}
	o, err := parseExtract(out)
	if err != nil {
		j.finish(phaseError, "", err.Error(), "")
		return
	}
	d := newDoc(randomHex(6), file, out, o, a.mem)
	d.Dir = dir
	if dir == dirPreview && fileExists(filepath.Join(a.previewDir(), pageName(file))) {
		d.PageFile = pageName(file)
	}
	a.putDoc(d)
	j.finish(phaseDone, fmt.Sprintf("Lettura finita in %d secondi.", int(time.Since(start).Seconds())), "", d.ID)
}

// extractCached restituisce l'uscita di "libretto extract" per l'immagine. Per un'anteprima: se la lettura in cache
// (anteprima-NNN-lettura.json) e la pagina dati ci sono, le usa SENZA rilanciare libretto (istantaneo); altrimenti lancia
// extract con -page-out (la sola pagina dati, 2 mm sopra la scritta REPUBLICA DE PANAMA, dritta, senza bianchi; libretto la salva
// solo se trova le righe MRZ) e, se va bene, mette il JSON in cache. msg non vuoto = errore da mostrare.
func (a *App) extractCached(ctx context.Context, j *Job, dir, file string, topMargin float64) (out []byte, msg string, cancelled bool) {
	args := []string{"extract", "-i", relPath(dir, file)}
	cache := ""
	if dir == dirPreview {
		cache = filepath.Join(a.previewDir(), readName(file))
		if b, err := os.ReadFile(cache); err == nil {
			if _, perr := parseExtract(b); perr == nil {
				a.logf("lettura di %s dalla cache", file)
				return b, "", false
			}
		}
		pageFile := pageName(file)
		_ = os.Remove(filepath.Join(a.previewDir(), pageFile))
		args = append(args, "-page-out", relPath(dir, pageFile), "-top-margin", strconv.FormatFloat(topMargin, 'f', -1, 64), "-force")
	}
	out, code, err := a.runLibretto(ctx, args, j.addLog)
	if ctx.Err() != nil {
		return nil, "", true
	}
	if err != nil {
		return nil, err.Error(), false
	}
	if code != 0 {
		j.mu.Lock()
		last := lastErrLine(j.Log)
		j.mu.Unlock()
		m := "La lettura dei dati non è riuscita"
		if last != "" {
			m += " (" + last + ")"
		}
		return nil, m + ".", false
	}
	if dir == dirPreview {
		// un PDF fatto dall'immagine intera non e' piu' quello giusto: si rifara' dalla pagina dati
		if fileExists(filepath.Join(a.previewDir(), pageName(file))) {
			_ = os.Remove(filepath.Join(a.previewDir(), strings.TrimSuffix(file, ".png")+".pdf"))
		}
		if _, perr := parseExtract(out); perr == nil {
			_ = os.WriteFile(cache, out, 0o600)
		}
	}
	return out, "", false
}

// copyNew copia src in dst senza sovrascrivere un file esistente.
func copyNew(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("il file esiste già")
		}
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}
