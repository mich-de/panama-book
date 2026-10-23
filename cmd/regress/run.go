package main

// Esecuzione di `libretto extract -i FILE` sul corpus, in parallelo, e raccolta dei risultati.
// Si lancia SOLO il sottocomando extract: mai scan, read o altro che avvii lo scanner.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"scanlibretto/internal/outsum"
)

// config raccoglie quello che serve per eseguire le prove.
type config struct {
	exe     string        // percorso completo dell'eseguibile (e' una copia privata, vedi copyExe)
	root    string        // radice: cartella di lavoro dei processi e base dei percorsi relativi
	jobs    int           // processi in parallelo
	timeout time.Duration // tempo massimo per immagine
	truth   string        // cartella delle verita' ("" = nessun confronto)
}

// runAll esegue extract su ogni file e restituisce le righe nello stesso ordine. progress (puo' essere nil)
// riceve una riga per file finito.
func runAll(ctx context.Context, cfg config, files []string, progress io.Writer) []Row {
	rows := make([]Row, len(files))
	next := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex // serializza le righe di avanzamento
	done := 0
	for w := 0; w < max(1, cfg.jobs); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				rows[i] = measure(ctx, cfg, files[i])
				if progress != nil {
					mu.Lock()
					done++
					fmt.Fprintf(progress, "[%*d/%d] %s\n", len(fmt.Sprint(len(files))), done, len(files), progressLine(rows[i]))
					mu.Unlock()
				}
			}
		}()
	}
	for i := range files {
		if ctx.Err() != nil {
			break // interrotto: i file non partiti restano righe vuote (il chiamante controlla ctx)
		}
		next <- i
	}
	close(next)
	wg.Wait()
	return rows
}

// progressLine e' la riga di avanzamento: nome del file e solo esiti numerici.
func progressLine(r Row) string {
	if r.Err {
		return fmt.Sprintf("%s  ERRORE (%s)  %.1f s", r.File, r.Note, r.Seconds)
	}
	mrz := "no"
	if r.MRZ {
		mrz = "si"
	}
	return fmt.Sprintf("%s  rot %d  MRZ %s  check %d/%d  campi %d/%d  %.1f s", r.File, r.Rotation, mrz,
		r.checksOK(), outsum.NChecks, r.found(), outsum.NFields, r.Seconds)
}

// measure esegue extract su un'immagine e ne riassume l'esito. Non legge mai valori personali fuori dalla memoria.
func measure(ctx context.Context, cfg config, rel string) Row {
	row := Row{File: rel}
	cctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, cfg.exe, "extract", "-i", absPath(cfg.root, rel))
	cmd.Dir = cfg.root
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 5 * time.Second // un OCR rimasto appeso non deve bloccare la prova
	start := time.Now()
	err := cmd.Run()
	row.Seconds = time.Since(start).Seconds()
	if err != nil {
		row.Err, row.Note = true, describeFailure(cctx, err, stderr.Bytes())
		return row
	}
	sum, err := outsum.Parse(stdout.Bytes())
	if err != nil {
		row.Err, row.Note = true, "JSON non valido"
		return row
	}
	truthMap, name := "", byte(0)
	if cfg.truth != "" {
		truth, ok, terr := loadTruth(cfg.truth, rel)
		switch {
		case terr != nil:
			row.Note = "verita' non letta: " + terr.Error()
		case ok:
			truthMap, name = scoreTruth(sum, truth)
		}
	}
	notes := row.Note
	row = rowFromSummary(rel, sum, row.Seconds, truthMap, name)
	row.Note = notes
	return row
}

// describeFailure spiega un fallimento senza riportare dati: tempo scaduto, codice di uscita e prima riga di stderr.
func describeFailure(ctx context.Context, err error, stderr []byte) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "tempo scaduto"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "interrotto"
	}
	msg := fmt.Sprintf("uscita: %v", err)
	if first, _, _ := strings.Cut(strings.TrimSpace(string(stderr)), "\n"); first != "" {
		if len(first) > 120 {
			first = first[:120]
		}
		msg += "; " + strings.TrimSpace(first)
	}
	return msg
}

// copyExe copia l'eseguibile in una cartella temporanea privata (stesso nome, stessa data di modifica) e ne
// restituisce il percorso e una funzione che la elimina. Si prova la copia, non l'originale: cosi' chi sta
// compilando lo stesso file (altri agenti) non lo trova bloccato da Windows e la prova non cambia a meta' se
// l'eseguibile viene ricompilato.
func copyExe(src string) (dst string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "regress-exe-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	in, err := os.Open(src)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		cleanup()
		return "", nil, err
	}
	dst = filepath.Join(dir, filepath.Base(src))
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o700)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		cleanup()
		return "", nil, err
	}
	if err := out.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	_ = os.Chtimes(dst, st.ModTime(), st.ModTime()) // solo informativo (riga di intestazione della tabella)
	return dst, cleanup, nil
}

// describeExe identifica l'eseguibile provato: nome, dimensione, data di modifica e impronta SHA-256 (12 caratteri).
func describeExe(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s (%d byte, modificato %s, sha256 %s)", filepath.Base(path), st.Size(),
		st.ModTime().Format("2006-01-02T15:04:05"), hex.EncodeToString(h.Sum(nil))[:12]), nil
}

// stamp e' lo stato di un'immagine (dimensione e data di modifica) per accorgersi se qualcuno la cambia.
type stamp struct {
	size int64
	mod  time.Time
}

// snapshot registra lo stato dei file del corpus.
func snapshot(root string, files []string) map[string]stamp {
	out := make(map[string]stamp, len(files))
	for _, f := range files {
		if st, err := os.Stat(absPath(root, f)); err == nil {
			out[f] = stamp{st.Size(), st.ModTime()}
		}
	}
	return out
}

// changedSince elenca i file il cui stato non e' piu' quello registrato in before (modificati, spostati o spariti).
func changedSince(root string, before map[string]stamp) []string {
	var out []string
	for f, b := range before {
		st, err := os.Stat(absPath(root, f))
		if err != nil || st.Size() != b.size || !st.ModTime().Equal(b.mod) {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}
