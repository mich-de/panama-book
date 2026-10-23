package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // per image.DecodeConfig
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"scanlibretto/internal/filename"
)

// Le anteprime sono immagini temporanee create dall'app in dati\anteprima\ (anteprima-001.png, ...): NON sono nella cartella scans\.
// Solo "Conferma e salva" le sposta in scans\ con il nome definitivo. Sono file dell'app, non dati dell'utente: si possono scartare
// e quelle piu' vecchie di 7 giorni si eliminano all'avvio.
const (
	dirScans   = "scans"
	dirPreview = "preview"
	// previewMaxAge: oltre questa eta' un'anteprima non salvata si elimina all'avvio.
	previewMaxAge = 7 * 24 * time.Hour
	// lowRatio: sotto questo rapporto altezza/larghezza l'immagine non puo' essere una pagina dati intera (suggerimento, non blocca).
	lowRatio = 0.5
)

var rePreviewPNG = regexp.MustCompile(`^anteprima-(\d+)\.png$`)
var rePreviewAny = regexp.MustCompile(`^anteprima-(\d+)(\.png|\.pdf|-grezza\.png|-pagina\.png|-diagnosi\.json|-lettura\.json)$`)
var rePreviewRaw = regexp.MustCompile(`^anteprima-(\d+)-(grezza|pagina)\.png$`)

// rawName e diagName sono i file che viaggiano con l'anteprima: la scansione grezza e la diagnosi (si scartano e si puliscono insieme).
func rawName(file string) string { return strings.TrimSuffix(file, ".png") + "-grezza.png" }

// readName e' la lettura dei dati in cache (JSON di extract), fatta in automatico a fine scansione.
func readName(file string) string { return strings.TrimSuffix(file, ".png") + "-lettura.json" }

func diagName(file string) string { return strings.TrimSuffix(file, ".png") + "-diagnosi.json" }

// pageName e' la SOLA PAGINA DATI ritagliata (extract -page-out): l'immagine che "Conferma e salva" salva e da cui si fa il PDF.
func pageName(file string) string { return strings.TrimSuffix(file, ".png") + "-pagina.png" }

func (a *App) previewDir() string { return filepath.Join(a.cfg.Base, "dati", "anteprima") }

// safePreview controlla che il nome sia un'anteprima dell'app (anteprima-NNN.png) esistente in dati\anteprima\.
func (a *App) safePreview(name string) (string, bool) {
	if !rePreviewPNG.MatchString(name) {
		return "", false
	}
	p := filepath.Join(a.previewDir(), name)
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return "", false
	}
	return p, true
}

// safeImageView e' safeImage per la VISUALIZZAZIONE: per le anteprime ammette anche la scansione grezza.
func (a *App) safeImageView(dir, name string) (string, bool) {
	if dir == dirPreview && rePreviewRaw.MatchString(name) {
		p := filepath.Join(a.previewDir(), name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, true
		}
		return "", false
	}
	return a.safeImage(dir, name)
}

// safeImage restituisce il percorso di un'immagine di scans\ o di dati\anteprima\ (dir "preview").
func (a *App) safeImage(dir, name string) (string, bool) {
	if dir == dirPreview {
		return a.safePreview(name)
	}
	return a.safeScanFile(name)
}

// relPath e' il percorso relativo alla cartella di lavoro (dove gira libretto.exe) dell'immagine.
func relPath(dir, file string) string {
	if dir == dirPreview {
		return filepath.Join("dati", "anteprima", file)
	}
	return filepath.Join("scans", file)
}

func imageURL(dir, file string) string {
	if dir == dirPreview {
		return "/api/image?dir=preview&file=" + urlEscape(file)
	}
	return "/api/image?file=" + urlEscape(file)
}

// nextPreviewName da' il prossimo nome libero anteprima-NNN (numero progressivo sopra tutti quelli presenti, anche PDF).
func (a *App) nextPreviewName() string {
	max := 0
	ents, _ := os.ReadDir(a.previewDir())
	for _, e := range ents {
		if m := rePreviewAny.FindStringSubmatch(e.Name()); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("anteprima-%03d", max+1)
}

// cleanOldPreviews elimina da dati\anteprima\ SOLO i file dell'app (anteprima-NNN.png/.pdf) piu' vecchi di maxAge.
func (a *App) cleanOldPreviews(maxAge time.Duration) int {
	n := 0
	ents, _ := os.ReadDir(a.previewDir())
	for _, e := range ents {
		if e.IsDir() || !rePreviewAny.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) <= maxAge {
			continue
		}
		if os.Remove(filepath.Join(a.previewDir(), e.Name())) == nil {
			n++
		}
	}
	if n > 0 {
		a.logf("eliminate %d anteprime vecchie (oltre %d giorni)", n, int(maxAge.Hours()/24))
	}
	return n
}

// imageSize legge le dimensioni senza decodificare tutta l'immagine.
func imageSize(path string) (w, h int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	c, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return c.Width, c.Height
}

// PreviewView e' un'anteprima non salvata per l'interfaccia.
type PreviewView struct {
	File   string `json:"file"`
	URL    string `json:"url"`
	Time   string `json:"time"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// Low: immagine molto bassa (altezza/larghezza < 0,5): di solito solo l'informativa, libretto inserito male. Solo un suggerimento.
	Low    bool   `json:"low"`
	DocID  string `json:"doc_id,omitempty"` // la lettura dei dati e' gia' stata fatta in questa sessione
	HasPDF bool   `json:"has_pdf"`
	// Scansione grezza (come esce dallo scanner, prima del ritaglio) e diagnosi automatica della posizione del libretto.
	HasPage bool       `json:"has_page"` // dopo "Leggi i dati": pagina dati ritagliata disponibile
	PageURL string     `json:"page_url,omitempty"`
	HasRaw  bool       `json:"has_raw"`
	RawURL  string     `json:"raw_url,omitempty"`
	Diag    *Diagnosis `json:"diag,omitempty"`
}

func (a *App) previewView(file string, mod time.Time) PreviewView {
	p := filepath.Join(a.previewDir(), file)
	w, h := imageSize(p)
	v := PreviewView{File: file, URL: imageURL(dirPreview, file), Time: mod.Format(time.RFC3339), Width: w, Height: h,
		Low: w > 0 && float64(h)/float64(w) < lowRatio}
	v.HasPDF = fileExists(filepath.Join(a.previewDir(), strings.TrimSuffix(file, ".png")+".pdf"))
	if v.HasPage = fileExists(filepath.Join(a.previewDir(), pageName(file))); v.HasPage {
		v.PageURL = imageURL(dirPreview, pageName(file))
	}
	if v.HasRaw = fileExists(filepath.Join(a.previewDir(), rawName(file))); v.HasRaw {
		v.RawURL = imageURL(dirPreview, rawName(file))
	}
	if b, err := os.ReadFile(filepath.Join(a.previewDir(), diagName(file))); err == nil {
		var dg Diagnosis
		if json.Unmarshal(b, &dg) == nil {
			v.Diag = &dg
		}
	}
	// documento gia' letto da questa anteprima (ordine dei lucchetti: docsMu poi d.mu, come forgetInDocs)
	a.docsMu.Lock()
	for id, d := range a.docs {
		d.mu.Lock()
		match := d.Dir == dirPreview && d.File == file
		d.mu.Unlock()
		if match {
			v.DocID = id
		}
	}
	a.docsMu.Unlock()
	return v
}

// hPreviews elenca le anteprime non salvate, le piu' recenti per prime.
func (a *App) hPreviews(w http.ResponseWriter, r *http.Request) {
	type item struct {
		v   PreviewView
		mod time.Time
	}
	var items []item
	ents, _ := os.ReadDir(a.previewDir())
	for _, e := range ents {
		if e.IsDir() || !rePreviewPNG.MatchString(e.Name()) {
			continue
		}
		if info, err := e.Info(); err == nil {
			items = append(items, item{a.previewView(e.Name(), info.ModTime()), info.ModTime()})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].mod.Equal(items[j].mod) {
			return items[i].mod.After(items[j].mod)
		}
		return items[i].v.File > items[j].v.File
	})
	out := make([]PreviewView, 0, len(items))
	for _, it := range items {
		out = append(out, it.v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// hPreviewScan: pulsante "Anteprima". Scansione fisica (o copia dell'immagine demo) in dati\anteprima\: niente in scans\, niente JSON,
// niente lettura automatica.
func (a *App) hPreviewScan(w http.ResponseWriter, r *http.Request) {
	var s scanSettings
	if !readJSON(w, r, &s) {
		return
	}
	if err := s.validateParams(); err != nil {
		writeErr(w, http.StatusBadRequest, "impostazioni", err.Error())
		return
	}
	if a.cfg.Demo == "" {
		if _, err := os.Stat(a.cfg.Exe); err != nil {
			writeErr(w, http.StatusFailedDependency, "manca_libretto", "Manca "+filepath.Base(a.cfg.Exe)+" accanto a PanamaBook.exe.")
			return
		}
	}
	j, err := a.startJob("preview", a.cfg.Demo != "", func(ctx context.Context, j *Job) { a.doPreview(ctx, j, s) })
	if err != nil {
		writeErr(w, http.StatusConflict, "occupato", "Un altro lavoro è ancora in corso: attendi la fine.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job": j.ID})
}

// doPreview e' il lavoro "Anteprima".
func (a *App) doPreview(ctx context.Context, j *Job, s scanSettings) {
	if err := os.MkdirAll(a.previewDir(), 0o700); err != nil {
		j.finish(phaseError, "", "Cartella delle anteprime non creata: "+err.Error(), "")
		return
	}
	name := a.nextPreviewName() + ".png"
	dst := filepath.Join(a.previewDir(), name)
	j.set(phaseScanning, "Scansione in corso: lo scanner sta lavorando. Non toccare il libretto e attendi (circa 10 secondi).", false)
	if a.cfg.Demo != "" {
		j.set(phaseScanning, "PROVA (demo): nessuno scanner viene usato, si copia l'immagine di prova.", false)
		select {
		case <-time.After(a.demoDelay):
		case <-ctx.Done():
		}
		if err := copyNew(a.cfg.Demo, dst); err != nil {
			j.finish(phaseError, "", "Immagine di prova non copiata: "+err.Error(), "")
			return
		}
	} else {
		// MAI ctx qui: durante il trasferimento fisico non si interrompe nulla.
		rawDst := filepath.Join(a.previewDir(), rawName(name))
		_, code, err := a.runLibretto(context.Background(), a.scanArgsTo(s, relPath(dirPreview, name), relPath(dirPreview, rawName(name))), j.addLog)
		if err != nil || code != 0 {
			_ = os.Remove(dst) // un file a meta' non e' un'anteprima
			_ = os.Remove(rawDst)
			j.finish(phaseError, "", scanErrorText(code, err, j), "")
			return
		}
		if st, err := os.Stat(dst); err != nil || st.Size() == 0 {
			j.finish(phaseError, "", "La scansione è finita ma il file non c'è: controlla lo scanner e riprova.", "")
			return
		}
	}
	a.diagnose(j, name, s)
	// lettura automatica (senza pulsante): pagina dati ritagliata + JSON in cache; fallisce senza far fallire l'anteprima
	j.set(phaseReading, "Anteprima: ritaglio della pagina dati in corso (pochi secondi).", false)
	if _, msg, _ := a.extractCached(context.Background(), j, dirPreview, name, s.TopMargin); msg != "" {
		a.logf("lettura automatica dell'anteprima %s: %s", name, msg)
	}
	info, _ := os.Stat(dst)
	mod := time.Now()
	if info != nil {
		mod = info.ModTime()
	}
	pv := a.previewView(name, mod)
	j.mu.Lock()
	j.Preview = &pv
	j.mu.Unlock()
	msg := "Anteprima pronta: si vede la sola pagina dati ritagliata; per i campi premi «Leggi i dati» (istantaneo)."
	if !pv.HasPage {
		msg = "La pagina dati non è stata trovata: si vede la scansione intera. Controlla l'inserimento (vedi i suggerimenti) e riscansiona."
	}
	if pv.Low {
		msg += " ATTENZIONE: l'immagine è molto bassa."
	}
	j.finish(phaseDone, msg, "", "")
}

// hDiscard cancella UN'anteprima (solo il file temporaneo creato dall'app, e il suo PDF di anteprima).
func (a *App) hDiscard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		File string `json:"file"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	p, ok := a.safePreview(req.File)
	if !ok {
		writeErr(w, http.StatusNotFound, "file", "Anteprima non trovata.")
		return
	}
	if err := os.Remove(p); err != nil {
		writeErr(w, http.StatusInternalServerError, "scarto", "Anteprima non scartata: "+err.Error())
		return
	}
	_ = os.Remove(strings.TrimSuffix(p, ".png") + ".pdf")
	_ = os.Remove(filepath.Join(a.previewDir(), rawName(req.File)))
	_ = os.Remove(filepath.Join(a.previewDir(), pageName(req.File)))
	_ = os.Remove(filepath.Join(a.previewDir(), diagName(req.File)))
	_ = os.Remove(filepath.Join(a.previewDir(), readName(req.File)))
	a.forgetPreviewDocs(req.File)
	a.logf("anteprima scartata: %s", req.File)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// forgetPreviewDocs toglie dallo stato i documenti letti da quell'anteprima.
func (a *App) forgetPreviewDocs(file string) {
	a.docsMu.Lock()
	defer a.docsMu.Unlock()
	for id, d := range a.docs {
		d.mu.Lock()
		match := d.Dir == dirPreview && d.File == file
		d.mu.Unlock()
		if match {
			delete(a.docs, id)
			if a.shownDoc == id {
				a.shownDoc = ""
			}
		}
	}
}

// fallbackName e' il nome dei file quando mancano nome, cognome o numero libretto: quello scelto dall'operatore (se valido) o il
// prossimo "libretto-NNN", sempre libero in scans\ (Unique: mai sovrascrivere).
func (a *App) fallbackName(chosen string) string {
	chosen = strings.TrimSpace(chosen)
	if !reName.MatchString(chosen) || strings.Contains(chosen, "..") {
		chosen = a.suggestName()
	}
	return filename.Unique(a.scansDir, chosen, scanExts...)
}

// movePreview salva l'anteprima in scans\\ col nome definitivo, tutto o niente. Se la lettura ha prodotto la SOLA PAGINA DATI ritagliata
// (extract -page-out) e' quella l'immagine principale `NOME.png`; la scansione intera si conserva come `NOME (intera).png` solo con
// keepFull, la grezza come `NOME (grezza).png` solo con keepRaw. Senza pagina (MRZ non trovata) si salva l'anteprima cosi' com'e'.
// Il PDF di anteprima, se c'e', segue con lo stesso nome base. Restituisce se e' stata usata la pagina dati.
func (a *App) movePreview(d *Doc, newName string, keepRaw, keepFull bool) (usedPage bool, err error) {
	if err := os.MkdirAll(a.scansDir, 0o700); err != nil {
		return false, err
	}
	pv := a.previewDir()
	full := filepath.Join(pv, d.File)
	page := filepath.Join(pv, pageName(d.File))
	main := full
	if fileExists(page) {
		main, usedPage = page, true
	}
	if !fileExists(main) {
		return false, errors.New("l'anteprima non c'è più")
	}
	var moved [][2]string
	rollback := func() {
		for i := len(moved) - 1; i >= 0; i-- {
			_ = os.Rename(moved[i][1], moved[i][0])
		}
	}
	steps := [][2]string{{main, filepath.Join(a.scansDir, newName+".png")}}
	if pdf := filepath.Join(pv, strings.TrimSuffix(d.File, ".png")+".pdf"); fileExists(pdf) {
		steps = append(steps, [2]string{pdf, filepath.Join(a.scansDir, newName+".pdf")})
	}
	if usedPage && keepFull {
		steps = append(steps, [2]string{full, filepath.Join(a.scansDir, filename.Unique(a.scansDir, newName+" (intera)", ".png")+".png")})
	}
	if raw := filepath.Join(pv, rawName(d.File)); keepRaw && fileExists(raw) {
		steps = append(steps, [2]string{raw, filepath.Join(a.scansDir, filename.Unique(a.scansDir, newName+" (grezza)", ".png")+".png")})
	}
	for _, st := range steps {
		if err := moveFile(st[0], st[1]); err != nil {
			rollback()
			return false, errors.Join(fmt.Errorf("spostamento di %s", filepath.Base(st[0])), err)
		}
		moved = append(moved, st)
	}
	// file di lavoro dell'app rimasti (scansione intera/grezza non conservate, diagnosi): si tolgono
	for _, f := range []string{full, page, filepath.Join(pv, rawName(d.File)), filepath.Join(pv, diagName(d.File)), filepath.Join(pv, readName(d.File))} {
		_ = os.Remove(f)
	}
	d.File, d.Name, d.Dir, d.PageFile = newName+".png", newName, dirScans, ""
	return usedPage, nil
}

// moveFile sposta un file senza mai sovrascrivere la destinazione; se il rename non e' possibile copia e poi toglie l'originale.
func moveFile(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return errors.New("la destinazione esiste già")
	}
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = in.Close()
		return err
	}
	_, cerr := io.Copy(out, in)
	_ = in.Close()
	if err := errors.Join(cerr, out.Close()); err != nil {
		_ = os.Remove(to)
		return err
	}
	return os.Remove(from)
}

// diagnose analizza la grezza dell'anteprima e scrive la diagnosi accanto (anteprima-NNN-diagnosi.json). In prova (demo) non c'e' una
// grezza vera: si copia l'immagine demo come grezza simulata (serve a provare l'interruttore) e non si fa nessuna diagnosi.
func (a *App) diagnose(j *Job, name string, s scanSettings) {
	rawPath := filepath.Join(a.previewDir(), rawName(name))
	if a.cfg.Demo != "" {
		_ = copyNew(a.cfg.Demo, rawPath)
		return
	}
	if !fileExists(rawPath) {
		a.logf("anteprima %s: nessuna scansione grezza (libretto.exe senza -raw?)", name)
		return
	}
	defer a.recoverTo("diagnosi della grezza", nil) // la diagnosi e' accessoria: un guasto non deve far fallire l'anteprima
	dg, err := diagnoseRaw(rawPath, s.DPI, s.LenMM, s.Driver == "twain")
	if err != nil {
		a.logf("diagnosi della grezza non riuscita: %v", err)
		return
	}
	b, _ := json.Marshal(dg)
	_ = os.WriteFile(filepath.Join(a.previewDir(), diagName(name)), b, 0o600)
	a.logf("diagnosi %s: documento %.0fx%.0f mm, spostamento %.0f mm, inclinazione %.1f, suggerimenti %d", name, dg.DocWidthMM, dg.DocHeightMM, dg.OffsetMM, dg.SkewDeg, len(dg.Hints))
}
