package main

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"scanlibretto/internal/imgproc"
)

// Taglio dell'anteprima. L'immagine dell'anteprima (anteprima-NNN.png) e' la PAGINA INTERA: il passaporto con informativa e pagina
// dati. Si parte sempre da li'. Il taglio automatico e quello manuale la rifanno a partire dalla scansione grezza
// (anteprima-NNN-grezza.png, il foglio com'e' uscito dallo scanner), che non si tocca mai. La PAGINA DATI (anteprima-NNN-pagina.png,
// solo la pagina con la foto e l'MRZ) si ricava dalla lettura e l'operatore sceglie se salvare quella o la pagina intera.

// minCropPx: un rettangolo piu' piccolo non puo' essere un libretto (suggerimento contro i clic accidentali).
const minCropPx = 100

// ensureRaw garantisce che esista la scansione grezza dell'anteprima: se manca (scansione senza -raw) si conserva una copia
// dell'immagine com'e', prima di tagliarla, cosi' un taglio sbagliato si puo' rifare.
func (a *App) ensureRaw(file string) error {
	pv := a.previewDir()
	raw := filepath.Join(pv, rawName(file))
	if fileExists(raw) {
		return nil
	}
	return copyNew(filepath.Join(pv, file), raw)
}

// dropDerived toglie quello che dipende dal taglio: pagina dati, lettura in cache e PDF. Si rifanno con «Leggi i dati».
func (a *App) dropDerived(file string) {
	pv := a.previewDir()
	_ = os.Remove(filepath.Join(pv, pageName(file)))
	_ = os.Remove(filepath.Join(pv, readName(file)))
	_ = os.Remove(filepath.Join(pv, strings.TrimSuffix(file, ".png")+".pdf"))
	a.setPreviewDocsPage(file)
}

// cutAuto rifa' il taglio automatico della pagina intera dalla scansione grezza (libretto2 crop). found=false se il passaporto
// non e' stato riconosciuto e si e' ripiegato sul ritaglio generico. Errore = l'immagine non e' stata toccata.
func (a *App) cutAuto(ctx context.Context, j *Job, file string) (found bool, err error) {
	if err := a.ensureRaw(file); err != nil {
		return false, fmt.Errorf("copia della scansione grezza: %w", err)
	}
	found = true
	onLine := func(l string) {
		j.addLog(l)
		if strings.Contains(l, "non riconosciuto") {
			found = false
		}
	}
	args := []string{"crop", "-i", a.relPath(dirPreview, rawName(file)), "-o", a.relPath(dirPreview, file), "-force"}
	_, code, err := a.runLibretto(ctx, args, onLine)
	if err != nil {
		return false, err
	}
	if code != 0 {
		j.mu.Lock()
		last := lastErrLine(j.Log)
		j.mu.Unlock()
		msg := "il taglio automatico non è riuscito"
		if last != "" {
			msg += " (" + last + ")"
		}
		a.logf("anteprima %s: %s", file, msg)
		return false, fmt.Errorf("%s", msg)
	}
	return found, nil
}

// hPreviewCropBox: dove sta il documento nell'immagine di partenza (frazioni 0-1 della larghezza e dell'altezza), per
// far partire il rettangolo del taglio manuale sul passaporto invece che sul foglio intero.
func (a *App) hPreviewCropBox(w http.ResponseWriter, r *http.Request) {
	file := r.URL.Query().Get("file")
	dir, name := a.previewDir(), file
	scans := r.URL.Query().Get("dir") == dirScans
	if scans { // immagine salvata: si disegna sull'originale (se c'e') e non sull'immagine gia' tagliata
		if _, ok := a.safeScanFile(file); !ok {
			writeErr(w, http.StatusNotFound, "file", "Immagine non trovata.")
			return
		}
		dir, name = a.outDir(), a.scanCropSource(file)
	} else {
		if _, ok := a.safePreview(file); !ok {
			writeErr(w, http.StatusNotFound, "file", "Anteprima non trovata.")
			return
		}
		if r.URL.Query().Get("source") == "raw" {
			name = rawName(file)
		}
	}
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		writeErr(w, http.StatusNotFound, "file", "L'immagine di partenza non c'è più.")
		return
	}
	img, _, err := image.Decode(f)
	_ = f.Close()
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "immagine", "Immagine di partenza non leggibile.")
		return
	}
	box, ok := imgproc.BookRect(img) // preciso se il foglio e' dritto
	if !ok {
		box, ok = imgproc.DocumentBox(img) // altrimenti il riquadro del documento
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false})
		return
	}
	b := img.Bounds()
	box = box.Sub(b.Min)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "image": name,
		"x": float64(box.Min.X) / float64(b.Dx()), "y": float64(box.Min.Y) / float64(b.Dy()),
		"w": float64(box.Dx()) / float64(b.Dx()), "h": float64(box.Dy()) / float64(b.Dy())})
}

type cropReq struct {
	Dir    string `json:"dir"`    // "preview" (predefinito) | "scans": immagine gia' salvata, "Ultime scansioni"
	DocID  string `json:"doc_id"` // scans: documento aperto, ricaricato a fine taglio
	File   string `json:"file"`
	Mode   string `json:"mode"`   // "auto" | "manual"
	Source string `json:"source"` // manuale: immagine su cui l'operatore ha disegnato, "raw" | "full"
	X      int    `json:"x"`      // manuale: rettangolo in pixel dell'immagine di partenza
	Y      int    `json:"y"`
	W      int    `json:"w"`
	H      int    `json:"h"`
	// Quad: manuale a quattro angoli liberi (alto-sx, alto-dx, basso-dx, basso-sx), in pixel dell'immagine di partenza: si raddrizza
	Quad [][2]float64 `json:"quad"`
}

// hPreviewCrop: pulsanti "Taglio automatico" (lavoro asincrono) e "Applica taglio" del taglio manuale (immediato).
func (a *App) hPreviewCrop(w http.ResponseWriter, r *http.Request) {
	var req cropReq
	if !readJSON(w, r, &req) {
		return
	}
	if req.Dir == dirScans {
		a.cropScan(w, req)
		return
	}
	if _, ok := a.safePreview(req.File); !ok {
		writeErr(w, http.StatusNotFound, "file", "Anteprima non trovata.")
		return
	}
	if a.busyNow() != nil {
		writeErr(w, http.StatusConflict, "occupato", "Un altro lavoro è ancora in corso: attendi la fine.")
		return
	}
	switch req.Mode {
	case "auto":
		if _, err := os.Stat(a.cfg.Exe); err != nil {
			writeErr(w, http.StatusFailedDependency, "manca_libretto", "Manca "+filepath.Base(a.cfg.Exe)+" accanto a PanamaBook.exe.")
			return
		}
		j, err := a.startJob("crop", a.cfg.Demo != "", func(ctx context.Context, j *Job) { a.doCropAuto(ctx, j, req.File) })
		if err != nil {
			writeErr(w, http.StatusConflict, "occupato", "Un altro lavoro è ancora in corso: attendi la fine.")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"job": j.ID})
	case "manual":
		a.cropManual(w, req)
	default:
		writeErr(w, http.StatusBadRequest, "modo", "Modo di taglio sconosciuto.")
	}
}

// doCropAuto rifa' la pagina intera dalla scansione grezza, poi la pagina dati e la lettura (come alla scansione).
func (a *App) doCropAuto(ctx context.Context, j *Job, file string) {
	j.set(phaseReading, "Taglio automatico in corso (pochi secondi). Puoi annullare.", true)
	found, err := a.cutAuto(ctx, j, file)
	if ctx.Err() != nil {
		j.finish(phaseCancelled, "Taglio annullato.", "", "")
		return
	}
	if err != nil {
		j.finish(phaseError, "", "Taglio automatico: "+err.Error()+".", "")
		return
	}
	a.dropDerived(file)
	if _, msg, _ := a.extractCached(ctx, j, dirPreview, file, defTopMargin); msg != "" {
		a.logf("lettura dopo il taglio automatico di %s: %s", file, msg)
	}
	a.setPreviewDocsPage(file)
	pvw := a.previewView(file, time.Now())
	j.mu.Lock()
	j.Preview = &pvw
	j.mu.Unlock()
	if !found {
		j.finish(phaseDone, "Taglio automatico: il passaporto non è stato riconosciuto (MRZ non letta), ritaglio generico al documento. Prova il taglio manuale.", "", "")
		return
	}
	j.finish(phaseDone, "Taglio automatico fatto: si vede la pagina intera.", "", "")
}

// cropManual rifa' la pagina intera con il rettangolo scelto dall'operatore, sempre dalla scansione grezza.
func (a *App) cropManual(w http.ResponseWriter, req cropReq) {
	if req.Source != "raw" && req.Source != "full" {
		writeErr(w, http.StatusBadRequest, "sorgente", "Immagine di partenza sconosciuta.")
		return
	}
	if err := a.ensureRaw(req.File); err != nil {
		writeErr(w, http.StatusInternalServerError, "grezza", "Scansione grezza non conservata: "+err.Error())
		return
	}
	pv := a.previewDir()
	src := filepath.Join(pv, rawName(req.File))
	if err := cropManualFile(src, filepath.Join(pv, req.File), req); err != nil {
		writeErr(w, http.StatusBadRequest, "taglio", err.Error())
		return
	}
	a.dropDerived(req.File)
	a.logf("taglio manuale di %s: %d,%d %dx%d", req.File, req.X, req.Y, req.W, req.H)
	writeJSON(w, http.StatusOK, map[string]any{"preview": a.previewView(req.File, time.Now())})
}

// ---- immagini gia' salvate ("Ultime scansioni") --------------------------------------------------------------------------
// Il taglio sostituisce scans\NOME.png. Prima si conserva l'immagine com'era in «NOME (originale).png» (una volta sola): ogni
// nuovo taglio parte da quella, cosi' si puo' sempre rifare. JSON, verita' e PDF gia' salvati non si toccano.

const suffixOriginal = " (originale)"

// scanOriginal e' il nome della copia originale di un'immagine salvata (stessa estensione: PNG o JPEG).
func scanOriginal(file string) string {
	ext := filepath.Ext(file)
	return strings.TrimSuffix(file, ext) + suffixOriginal + ext
}

// isJPEG: immagini salvate in JPEG (per esempio i facsimili messi in scans\): il taglio le rifa' in JPEG, non cambia estensione.
func isJPEG(file string) bool {
	ext := strings.ToLower(filepath.Ext(file))
	return ext == ".jpg" || ext == ".jpeg"
}

// scanCropSource e' l'immagine da cui si taglia: l'originale se c'e', altrimenti l'immagine stessa.
func (a *App) scanCropSource(file string) string {
	if o := scanOriginal(file); fileExists(filepath.Join(a.outDir(), o)) {
		return o
	}
	return file
}

// cropScan: taglio automatico (lavoro) o manuale (immediato) di un'immagine di scans\.
func (a *App) cropScan(w http.ResponseWriter, req cropReq) {
	if _, ok := a.safeScanFile(req.File); !ok {
		writeErr(w, http.StatusNotFound, "file", "Immagine non trovata.")
		return
	}
	// aperta la copia «NOME (originale).ext»: si taglia da quella e il risultato va nell'immagine principale NOME.ext
	if strings.Contains(req.File, suffixOriginal) {
		req.File = strings.Replace(req.File, suffixOriginal, "", 1)
	}
	if !(strings.HasSuffix(strings.ToLower(req.File), ".png") || isJPEG(req.File)) || strings.Contains(req.File, suffixOriginal) {
		writeErr(w, http.StatusBadRequest, "file", "Si può tagliare solo un'immagine PNG o JPEG.")
		return
	}
	if a.busyNow() != nil {
		writeErr(w, http.StatusConflict, "occupato", "Un altro lavoro è ancora in corso: attendi la fine.")
		return
	}
	// l'originale si conserva PRIMA di qualunque modifica
	if !fileExists(filepath.Join(a.outDir(), req.File)) && fileExists(filepath.Join(a.outDir(), scanOriginal(req.File))) {
		// l'immagine principale non c'e' piu': c'e' solo l'originale, da cui si taglia
	} else if err := copyNew(filepath.Join(a.outDir(), req.File), filepath.Join(a.outDir(), scanOriginal(req.File))); err != nil && !strings.Contains(err.Error(), "esiste già") {
		writeErr(w, http.StatusInternalServerError, "originale", "Copia originale non conservata: "+err.Error())
		return
	}
	switch req.Mode {
	case "auto":
		if _, err := os.Stat(a.cfg.Exe); err != nil {
			writeErr(w, http.StatusFailedDependency, "manca_libretto", "Manca "+filepath.Base(a.cfg.Exe)+" accanto a PanamaBook.exe.")
			return
		}
		j, err := a.startJob("crop", a.cfg.Demo != "", func(ctx context.Context, j *Job) { a.doCropScanAuto(ctx, j, req.File, req.DocID) })
		if err != nil {
			writeErr(w, http.StatusConflict, "occupato", "Un altro lavoro è ancora in corso: attendi la fine.")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"job": j.ID})
	case "manual":
		src := filepath.Join(a.outDir(), scanOriginal(req.File))
		if err := cropManualFile(src, filepath.Join(a.outDir(), req.File), req); err != nil {
			writeErr(w, http.StatusBadRequest, "taglio", err.Error())
			return
		}
		a.logf("taglio manuale di scans\\%s: %d,%d %dx%d", req.File, req.X, req.Y, req.W, req.H)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "original": scanOriginal(req.File), "file": req.File})
	default:
		writeErr(w, http.StatusBadRequest, "modo", "Modo di taglio sconosciuto.")
	}
}

// doCropScanAuto rifa' il taglio automatico dall'originale e sostituisce scans\NOME.png.
func (a *App) doCropScanAuto(ctx context.Context, j *Job, file, docID string) {
	j.set(phaseReading, "Taglio automatico in corso (pochi secondi).", true)
	out := file
	if isJPEG(file) { // libretto2 scrive PNG: si taglia in un PNG temporaneo, poi si riscrive in JPEG
		out = file + ".taglio.png"
		defer os.Remove(filepath.Join(a.outDir(), out))
	}
	args := []string{"crop", "-i", a.relPath(dirScans, scanOriginal(file)), "-o", a.relPath(dirScans, out), "-force"}
	found := true
	_, code, err := a.runLibretto(ctx, args, func(l string) {
		j.addLog(l)
		if strings.Contains(l, "non riconosciuto") {
			found = false
		}
	})
	if ctx.Err() != nil {
		j.finish(phaseCancelled, "Taglio annullato.", "", "")
		return
	}
	if err != nil || code != 0 {
		j.finish(phaseError, "", "Il taglio automatico non è riuscito: l'immagine non è stata cambiata.", "")
		return
	}
	if out != file {
		if err := convertImage(filepath.Join(a.outDir(), out), filepath.Join(a.outDir(), file)); err != nil {
			j.finish(phaseError, "", "Il taglio automatico non è riuscito: "+err.Error()+". L'immagine non è stata cambiata.", "")
			return
		}
	}
	msg := "Taglio automatico fatto: l'originale è in «" + scanOriginal(file) + "»."
	if !found {
		msg = "Taglio automatico: il passaporto non è stato riconosciuto (MRZ non letta), ritaglio generico. Prova il taglio manuale. L'originale è in «" + scanOriginal(file) + "»."
	}
	j.finish(phaseDone, msg, "", docID)
}

// cropManualFile fa il taglio manuale richiesto: a quattro angoli liberi (raddrizzato) o rettangolo dritto.
func cropManualFile(src, dst string, req cropReq) error {
	if len(req.Quad) == 0 {
		return cropImage(src, dst, image.Rect(req.X, req.Y, req.X+req.W, req.Y+req.H))
	}
	if len(req.Quad) != 4 {
		return fmt.Errorf("servono quattro angoli")
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(f)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("immagine di partenza non leggibile: %w", err)
	}
	b := img.Bounds()
	var q [4][2]float64
	for i, p := range req.Quad {
		if p[0] < -0.5 || p[1] < -0.5 || p[0] > float64(b.Dx())+0.5 || p[1] > float64(b.Dy())+0.5 {
			return fmt.Errorf("un angolo esce dall'immagine")
		}
		q[i] = p
	}
	out, err := imgproc.WarpQuad(img, q)
	if err != nil {
		return err
	}
	return writeImage(dst, out)
}

// cropImage salva in dst il rettangolo r di src (PNG o JPEG); il formato e' quello dell'estensione di dst. Scrive su un file
// temporaneo e poi sostituisce dst: un errore non lascia mai un'immagine a meta'.
func cropImage(src, dst string, r image.Rectangle) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(f)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("immagine di partenza non leggibile: %w", err)
	}
	b := img.Bounds()
	if r.Dx() < minCropPx || r.Dy() < minCropPx {
		return fmt.Errorf("il taglio è troppo piccolo (almeno %d x %d pixel)", minCropPx, minCropPx)
	}
	if !r.In(image.Rect(0, 0, b.Dx(), b.Dy())) {
		return fmt.Errorf("il taglio esce dall'immagine (%d x %d pixel)", b.Dx(), b.Dy())
	}
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return fmt.Errorf("formato immagine non ritagliabile")
	}
	return writeImage(dst, sub.SubImage(r.Add(b.Min)))
}

// convertImage riscrive src in dst nel formato dell'estensione di dst.
func convertImage(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(f)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("immagine non leggibile: %w", err)
	}
	return writeImage(dst, img)
}

// writeImage scrive img in dst (JPEG se l'estensione e' .jpg/.jpeg, altrimenti PNG) passando da un file temporaneo.
func writeImage(dst string, img image.Image) error {
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if isJPEG(dst) {
		err = jpeg.Encode(out, img, &jpeg.Options{Quality: 95})
	} else {
		err = png.Encode(out, img)
	}
	if err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst) // sostituisce dst se esiste
}

// setPreviewDocsPage allinea i documenti gia' letti da quell'anteprima alla pagina dati che c'e' (o non c'e' piu').
func (a *App) setPreviewDocsPage(file string) {
	a.docsMu.Lock()
	defer a.docsMu.Unlock()
	has := fileExists(filepath.Join(a.previewDir(), pageName(file)))
	for _, d := range a.docs {
		d.mu.Lock()
		if d.Dir == dirPreview && d.File == file {
			if has {
				d.PageFile = pageName(file)
			} else {
				d.PageFile = ""
			}
		}
		d.mu.Unlock()
	}
}
