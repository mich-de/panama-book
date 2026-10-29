package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// logger scrive il registro degli eventi in dati\panamabook.log (append, con data e ora). Il registro serve a capire PERCHE' il
// programma si e' chiuso o ha avuto un problema (il terminale nero si chiude e nasconde tutto). CONTIENE DATI PERSONALI
// (nomi dei file): sta in dati\, ignorata da git. Nessun valore dei campi viene mai scritto. nil = nessun registro (test).
type logger struct {
	mu   sync.Mutex
	f    *os.File
	path string
}

const maxLogBytes = 5 << 20

// openLog apre (o crea) il registro. Se supera 5 MB lo ruota in .1. Se il file e' occupato (per esempio un "cmd" che ci ha
// rediretto sopra l'output della console, senza condivisione in scrittura) si usa lo stesso nome con "-eventi" prima
// dell'estensione. Se non si riesce ad aprire niente restituisce nil.
func openLog(path string) *logger {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil
	}
	for _, p := range []string{path, strings.TrimSuffix(path, filepath.Ext(path)) + "-eventi" + filepath.Ext(path)} {
		if st, err := os.Stat(p); err == nil && st.Size() > maxLogBytes {
			_ = os.Remove(p + ".1")
			_ = os.Rename(p, p+".1")
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err == nil {
			return &logger{f: f, path: p}
		}
	}
	return nil
}

func (l *logger) printf(format string, a ...any) {
	if l == nil {
		return
	}
	line := time.Now().Format("2006-01-02 15:04:05.000") + " " + fmt.Sprintf(format, a...)
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	l.mu.Lock()
	_, _ = l.f.WriteString(line)
	l.mu.Unlock()
}

func (l *logger) close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	_ = l.f.Close()
	l.mu.Unlock()
}

// logf scrive nel registro dell'app (non fa nulla se non c'e').
func (a *App) logf(format string, args ...any) { a.lg.printf(format, args...) }

// recoverTo va chiamata con defer in ogni goroutine: un panic non deve mai chiudere il programma in silenzio. Scrive lo stack nel
// registro e chiama onPanic (se non e' nil) con un messaggio per l'operatore.
func (a *App) recoverTo(where string, onPanic func(msg string)) {
	if r := recover(); r != nil {
		a.logf("PANIC in %s: %v\n%s", where, r, debug.Stack())
		if onPanic != nil {
			onPanic(fmt.Sprintf("Errore interno del programma (%s). I dettagli sono in dati\\panamabook.log: se si ripete, mandalo a chi segue il programma.", where))
		}
	}
}

// statusWriter ricorda il codice di risposta, per il registro delle richieste.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// quietPath dice se una richiesta riuscita e' troppo frequente per finire nel registro (heartbeat, avanzamento dei lavori, immagini).
func quietPath(method, path string) bool {
	return path == "/api/heartbeat" || path == "/api/image" || (method == "GET" && strings.HasPrefix(path, "/api/jobs/")) || !strings.HasPrefix(path, "/api/")
}
