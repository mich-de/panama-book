package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg" // registra i formati per image.Decode
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"scanlibretto/internal/certif"
	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/memoria"
	"scanlibretto/internal/ocr"
	"scanlibretto/internal/pdfin"
	"scanlibretto/internal/pdfout"
	"scanlibretto/internal/pdftext"
	"scanlibretto/internal/qrcode"
	"scanlibretto/internal/seafarer"
)

// Scheda «Certificati»: SAT, SD, SSO e CT di Panama (vedi internal/certif). Il file arriva da un PDF o da un'immagine scelta
// dall'operatore, oppure da un foglio A4 scansionato (libretto2 sheet). Si legge con l'OCR di Windows, l'operatore controlla e
// corregge i campi e «Salva» scrive il JSON (e una copia del file) in <cartella di salvataggio>\certificati\.
// I file di lavoro stanno in dati\certificati\ (cert-ID.*), si cancellano al salvataggio, con «Scarta» e dopo un giorno.

const (
	certMaxUpload = 25 << 20
	certMaxAge    = 24 * time.Hour
	certDirName   = "certificati"
)

// certEntry e' un certificato letto e non ancora salvato.
type certEntry struct {
	ID     string
	Result *certif.Result
	Src    string // file originale (PDF o immagine) copiato in dati\certificati
	View   string // immagine da mostrare (uguale a Src se e' gia' un'immagine, altrimenti la pagina del PDF resa in PNG)
	Name   string // nome del file come lo ha scelto l'operatore (solo per mostrarlo)
	QR     string // testo del codice QR del certificato (di solito il link al servizio di verifica dell'AMP)
	Verif  *CertVerify
}

// CertVerify e' l'esito del controllo doppio con il servizio AMP.
type CertVerify struct {
	URL      string            `json:"url"`
	Status   string            `json:"status"` // stato scritto dal servizio («Valid»)
	Checks   []seafarer.Check  `json:"checks"`
	AllOK    bool              `json:"all_ok"`
	Online   map[string]string `json:"online,omitempty"`
	CheckedA string            `json:"checked_at"`
}

type certStore struct {
	mu   sync.Mutex
	byID map[string]*certEntry
	// rows legge il testo del certificato da un'immagine (sostituibile nei test: l'OCR vero gira solo su Windows)
	rows func(ctx context.Context, img string) ([]string, error)
	// lookup interroga il servizio AMP (sostituibile nei test: il vero indirizzo e' fisso e HTTP)
	lookup func(ctx context.Context, link string) (seafarer.Record, error)
	// search fa la ricerca per Marine ID sul sito AMP (sostituibile nei test)
	search func(ctx context.Context, name, birth, marineID string) (*seafarer.Found, error)
}

func (a *App) certWorkDir() string { return filepath.Join(a.cfg.Base, "dati", certDirName) }

func (a *App) certGet(id string) *certEntry {
	a.certs.mu.Lock()
	defer a.certs.mu.Unlock()
	return a.certs.byID[id]
}

func (a *App) certPut(e *certEntry) {
	a.certs.mu.Lock()
	defer a.certs.mu.Unlock()
	if a.certs.byID == nil {
		a.certs.byID = map[string]*certEntry{}
	}
	a.certs.byID[e.ID] = e
}

// certDrop toglie il certificato dall'elenco e cancella i suoi file di lavoro (solo cert-ID.* nella cartella dell'app).
func (a *App) certDrop(id string) {
	a.certs.mu.Lock()
	delete(a.certs.byID, id)
	a.certs.mu.Unlock()
	ents, _ := os.ReadDir(a.certWorkDir())
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "cert-"+id+".") {
			_ = os.Remove(filepath.Join(a.certWorkDir(), e.Name()))
		}
	}
}

// certCleanOld cancella i file di lavoro piu' vecchi di un giorno (SOLO cert-*, mai altro).
func (a *App) certCleanOld() {
	ents, _ := os.ReadDir(a.certWorkDir())
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), "cert-") {
			continue
		}
		if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > certMaxAge {
			_ = os.Remove(filepath.Join(a.certWorkDir(), e.Name()))
		}
	}
}

// ---- campi per l'interfaccia ----

// certLabel e' l'etichetta bilingue di un campo, come sul certificato.
var certLabels = map[string][2]string{
	certif.FCTNumber:      {"CT No.", "CT No."},
	certif.FDocNumber:     {"Nº Documento", "Document No."},
	certif.FName:          {"Nombre", "Name"},
	certif.FBirth:         {"Fecha de nacimiento", "Date of birth"},
	certif.FNationality:   {"Nacionalidad", "Nationality"},
	certif.FNationalityES: {"Nacionalidad (ES)", "Nationality (ES)"},
	certif.FIdentity:      {"Identificación", "Identification"},
	certif.FCapacity:      {"Cargo", "Capacity"},
	certif.FCapacityES:    {"Cargo (ES)", "Capacity (ES)"},
	certif.FIssue:         {"Fecha de expedición", "Date of issue"},
	certif.FExpiry:        {"Fecha de expiración", "Date of expiry"},
	certif.FMarineID:      {"ID del Marino", "Marine ID"},
	certif.FConsulate:     {"Cónsul", "Consulate"},
	certif.FReceipt:       {"Nº de recibo", "Receipt No."},
	certif.FCourse:        {"Curso", "Course"},
}

// certOrder e' l'ordine dei campi nella scheda, per tipo (prima quelli richiesti, poi gli altri).
var certOrder = map[string][]string{
	certif.SAT: {certif.FDocNumber, certif.FName, certif.FBirth, certif.FNationality, certif.FCourse, certif.FIssue, certif.FExpiry, certif.FMarineID},
	certif.SD:  {certif.FDocNumber, certif.FName, certif.FBirth, certif.FNationality, certif.FCourse, certif.FIssue, certif.FExpiry, certif.FMarineID},
	certif.SSO: {certif.FDocNumber, certif.FName, certif.FBirth, certif.FNationality, certif.FCourse, certif.FIssue, certif.FExpiry, certif.FMarineID},
	certif.CT: {certif.FCTNumber, certif.FDocNumber, certif.FName, certif.FIdentity, certif.FNationality, certif.FCapacity, certif.FIssue,
		certif.FExpiry, certif.FConsulate, certif.FMarineID, certif.FReceipt},
}

type certRow struct {
	Key      string `json:"key"`
	ES       string `json:"es"`
	EN       string `json:"en"`
	Required bool   `json:"required"`
}

// CertView e' un certificato per l'interfaccia: tutti i campi letti e, per ogni tipo, quali righe mostrare (cosi' l'operatore puo'
// cambiare il tipo se il riconoscimento ha sbagliato).
type CertView struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Name     string               `json:"name"`  // nome del file
	Media    string               `json:"media"` // "pdf" (si mostra il PDF stesso) oppure "image"
	QR       string               `json:"qr,omitempty"`
	QRAMP    bool                 `json:"qr_amp"` // il QR e' un link del servizio AMP: si puo' fare il controllo online
	Fields   map[string]string    `json:"fields"`
	Layouts  map[string][]certRow `json:"layouts"`
	Missing  []string             `json:"missing,omitempty"`
	Warnings []string             `json:"warnings,omitempty"`
}

func certLayouts() map[string][]certRow {
	out := map[string][]certRow{}
	for typ, keys := range certOrder {
		req := map[string]bool{}
		for _, k := range certif.Required[typ] {
			req[k] = true
		}
		for _, k := range keys {
			l := certLabels[k]
			out[typ] = append(out[typ], certRow{Key: k, ES: l[0], EN: l[1], Required: req[k]})
		}
	}
	return out
}

func (a *App) certView(e *certEntry) *CertView {
	r := e.Result
	return &CertView{ID: e.ID, Type: r.Type, Name: e.Name, Media: e.media(), QR: e.QR, QRAMP: seafarer.ValidURL(e.QR), Fields: r.Fields, Layouts: certLayouts(), Missing: r.Missing, Warnings: r.Warnings}
}

// media dice come mostrare il certificato nella pagina.
func (e *certEntry) media() string {
	if strings.EqualFold(filepath.Ext(e.View), ".pdf") {
		return "pdf"
	}
	return "image"
}

// ---- lettura ----

var errCertFile = errors.New("file non valido: serve un PDF, PNG o JPEG")

// certKind riconosce il formato dai primi byte (l'estensione scelta dall'utente non basta) e restituisce l'estensione da usare.
func certKind(head []byte) (string, error) {
	switch {
	case bytes.HasPrefix(head, []byte("%PDF-")):
		return ".pdf", nil
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		return ".png", nil
	case bytes.HasPrefix(head, []byte("\xff\xd8\xff")):
		return ".jpg", nil
	}
	return "", errCertFile
}

// hCertUpload: POST /api/cert/upload?name=file.pdf, corpo = i byte del file. Avvia il lavoro di lettura.
func (a *App) hCertUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, certMaxUpload)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "troppo_grande", "File troppo grande (massimo 25 MB).")
		return
	}
	ext, err := certKind(b)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "formato", err.Error())
		return
	}
	a.certCleanOld()
	e := &certEntry{ID: randomHex(6), Name: certDisplayName(r.URL.Query().Get("name"), ext)}
	if err := os.MkdirAll(a.certWorkDir(), 0o700); err != nil {
		writeErr(w, http.StatusInternalServerError, "cartella", "Cartella di lavoro non disponibile: "+err.Error())
		return
	}
	e.Src = filepath.Join(a.certWorkDir(), "cert-"+e.ID+".src"+ext)
	if err := os.WriteFile(e.Src, b, 0o600); err != nil {
		writeErr(w, http.StatusInternalServerError, "scrittura", "File non copiato: "+err.Error())
		return
	}
	a.startCertJob(w, "cert", e, nil)
}

// hCertScan: POST /api/cert/scan. Scansiona un A4 intero (libretto2 sheet) e lo legge come un certificato.
func (a *App) hCertScan(w http.ResponseWriter, r *http.Request) {
	if _, err := os.Stat(a.cfg.Exe); err != nil {
		writeErr(w, http.StatusFailedDependency, "manca_libretto", "Manca "+filepath.Base(a.cfg.Exe)+" accanto a PanamaBook.exe.")
		return
	}
	a.certCleanOld()
	e := &certEntry{ID: randomHex(6), Name: "scansione"}
	if err := os.MkdirAll(a.certWorkDir(), 0o700); err != nil {
		writeErr(w, http.StatusInternalServerError, "cartella", "Cartella di lavoro non disponibile: "+err.Error())
		return
	}
	e.Src = filepath.Join(a.certWorkDir(), "cert-"+e.ID+".src.png")
	a.startCertJob(w, "cert", e, func(ctx context.Context, j *Job) bool {
		j.set(phaseScanning, "Scansione in corso: lo scanner sta lavorando. Non toccare il foglio e attendi.", false)
		if a.cfg.Demo != "" {
			if err := copyNew(a.cfg.Demo, e.Src); err != nil {
				a.certFail(j, e, phaseError, "", "Immagine di prova non copiata: "+err.Error())
				return false
			}
			return true
		}
		// MAI ctx qui: durante il trasferimento fisico non si interrompe nulla.
		out, code, err := a.runLibretto(context.Background(), []string{"sheet", "-yes", "-force", "-o", e.Src, "-dpi", "300"}, j.addLog)
		if err == nil && code == 3 {
			a.certFail(j, e, phaseError, "", "Nessun foglio nello scanner. Appoggia il certificato (foglio A4) sul vetro oppure mettilo nel caricatore, e riprova.")
			return false
		}
		if err != nil || code != 0 {
			msg := scanErrorText(code, err, j)
			if strings.Contains(string(out)+strings.Join(j.Log, "\n"), "uso:") {
				msg = "Per scansionare un certificato serve libretto2.exe (questa versione dello scanner non conosce il comando «sheet»)."
			}
			a.certFail(j, e, phaseError, "", msg)
			return false
		}
		if st, err := os.Stat(e.Src); err != nil || st.Size() == 0 {
			a.certFail(j, e, phaseError, "", "La scansione è finita ma il file non c'è: controlla lo scanner e riprova.")
			return false
		}
		return true
	})
}

// startCertJob avvia il lavoro: eventuale passo iniziale (scansione), poi PDF -> immagine, OCR e lettura dei campi.
func (a *App) startCertJob(w http.ResponseWriter, kind string, e *certEntry, first func(ctx context.Context, j *Job) bool) {
	j, err := a.startJob(kind, a.cfg.Demo != "" && first != nil, func(ctx context.Context, j *Job) {
		if first != nil && !first(ctx, j) {
			return
		}
		a.doCertRead(ctx, j, e)
	})
	if err != nil {
		a.certDrop(e.ID)
		writeErr(w, http.StatusConflict, "occupato", "Un altro lavoro è ancora in corso: attendi la fine.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job": j.ID})
}

// doCertRead legge il certificato. Un PDF digitale si legge dal suo testo incorporato (internal/pdftext: nessun programma esterno,
// nessuna finestra, nessun OCR) e si mostra com'e'. Immagini e PDF senza testo si leggono con l'OCR; un PDF senza testo prima
// si rende in immagine (PowerShell nascosto, solo di ripiego).
func (a *App) doCertRead(ctx context.Context, j *Job, e *certEntry) {
	j.set(phaseReading, "Lettura del certificato in corso (alcuni secondi).", true)
	e.View = e.Src
	var rows []string
	if strings.EqualFold(filepath.Ext(e.Src), ".pdf") {
		if b, err := os.ReadFile(e.Src); err == nil {
			rows, err = pdftext.Rows(b)
			if err != nil {
				a.logf("certificato %s: testo del PDF non letto (%v), si passa all'OCR", e.ID, err)
				rows = nil
			}
		}
		if rows == nil {
			e.View = filepath.Join(a.certWorkDir(), "cert-"+e.ID+".png")
			a.logf("certificato %s: rendo il PDF in immagine", e.ID)
			if err := pdfin.RenderFirstPage(ctx, e.Src, e.View, 0); err != nil {
				a.logf("certificato %s: PDF non reso: %v", e.ID, err)
				a.certFail(j, e, phaseError, "", "Non riesco ad aprire il PDF. Prova a salvarlo come immagine o a scansionare il foglio.")
				return
			}
		}
	}
	if rows == nil {
		a.certCleanImage(e) // raddrizza e ritaglia il foglio: l'OCR legge meglio e l'anteprima sta al suo posto
		a.logf("certificato %s: immagine pronta, avvio OCR", e.ID)
		read := a.certs.rows
		if read == nil {
			read = ocrRows
		}
		var err error
		rows, err = read(ctx, e.View)
		if ctx.Err() != nil {
			a.certFail(j, e, phaseCancelled, "Lettura annullata.", "")
			return
		}
		if err != nil {
			a.logf("certificato %s: OCR non riuscito: %v", e.ID, err)
			a.certFail(j, e, phaseError, "", "Lettura del testo non riuscita: "+err.Error())
			return
		}
		a.logf("certificato %s: OCR finito (%d righe)", e.ID, len(rows))
		// foglio girato (caricatore, appoggiato storto): l'OCR non lo raddrizza da solo, quindi si prova con 180, 90 e 270 gradi
		if _, perr := certif.Parse(rows); perr != nil {
			if r2, v2, ok := a.certTryRotations(ctx, e, read); ok {
				rows, e.View = r2, v2
			} else if ctx.Err() == nil {
				a.certKeepUnrecognized(e)
			}
		}
	}
	res, err := certif.Parse(rows)
	if err != nil {
		a.certFail(j, e, phaseError, "", "Questo foglio non sembra un certificato SAT, SD, SSO o CT di Panama.")
		return
	}
	e.Result = res
	e.QR = a.certQR(e)
	a.certPut(e)
	j.mu.Lock()
	j.Cert = a.certView(e)
	j.mu.Unlock()
	j.finish(phaseDone, "Certificato letto: controlla i campi e salva.", "", "")
}

// certFail chiude il lavoro con un errore (o un annullamento) e cancella i file di lavoro PRIMA, cosi' non ne restano in giro.
func (a *App) certFail(j *Job, e *certEntry, phase, msg, errText string) {
	a.certDrop(e.ID)
	j.finish(phase, msg, errText, "")
}

// certQR cerca il codice QR del certificato: nelle immagini incorporate del PDF, oppure nell'immagine stessa.
// Se non si legge si prosegue senza (il controllo doppio e' un di piu', non una condizione).
func (a *App) certQR(e *certEntry) string {
	var imgs []image.Image
	if strings.EqualFold(filepath.Ext(e.Src), ".pdf") {
		if b, err := os.ReadFile(e.Src); err == nil {
			imgs, _ = pdftext.Images(b)
		}
	}
	if len(imgs) == 0 {
		if f, err := os.Open(e.View); err == nil {
			if im, _, err := image.Decode(f); err == nil {
				imgs = append(imgs, im)
			}
			f.Close()
		}
	}
	for _, im := range imgs {
		if txt, err := qrcode.Decode(im); err == nil && len(txt) <= 300 && !strings.ContainsAny(txt, "\r\n\x00") {
			return txt
		}
	}
	if e.View != e.Src && !strings.EqualFold(filepath.Ext(e.Src), ".pdf") { // il raddrizzamento puo' sciupare un QR piccolo
		if txt := readQRFile(e.Src); txt != "" {
			return txt
		}
	}
	return ""
}

// hCertVerify: POST /api/cert/verify {id, fields}. Controllo doppio con il servizio pubblico dell'AMP: parte solo su richiesta
// dell'operatore e solo se il QR e' un link di quel servizio. Confronta i campi (come sono ora nella scheda) con la pagina online.
func (a *App) hCertVerify(w http.ResponseWriter, r *http.Request) {
	var req certSaveReq
	if !readJSON(w, r, &req) {
		return
	}
	e := a.certGet(req.ID)
	if !reCertID.MatchString(req.ID) || e == nil {
		writeErr(w, http.StatusNotFound, "certificato", "Il certificato non c'è più: rileggilo.")
		return
	}
	if !seafarer.ValidURL(e.QR) {
		writeErr(w, http.StatusConflict, "senza_qr", "Il certificato non ha un codice QR del servizio AMP: niente da controllare online.")
		return
	}
	fields, err := certClean(req.Fields)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "campi", err.Error())
		return
	}
	look := a.certs.lookup
	if look == nil {
		c := seafarer.NewClient()
		look = func(ctx context.Context, link string) (seafarer.Record, error) { return seafarer.Fetch(ctx, c, link) }
	}
	ctx, cancel := context.WithTimeout(r.Context(), seafarer.Timeout+2*time.Second)
	defer cancel()
	rec, err := look(ctx, e.QR)
	if err != nil {
		a.logf("certificato %s: controllo online non riuscito: %v", e.ID, err)
		writeErr(w, http.StatusBadGateway, "servizio", err.Error())
		return
	}
	v := &CertVerify{URL: e.QR, Status: rec["status"], Checks: seafarer.Compare(fields, rec), AllOK: true, CheckedA: time.Now().Format(time.RFC3339)}
	for _, c := range v.Checks {
		v.AllOK = v.AllOK && c.OK
	}
	v.AllOK = v.AllOK && len(v.Checks) > 0
	e.Verif = v
	a.logf("certificato %s: controllo online fatto (%d campi, tutto uguale: %v)", e.ID, len(v.Checks), v.AllOK)
	writeJSON(w, http.StatusOK, v)
}

// certTryRotations rilegge l'immagine ruotata di 180, 90 e 270 gradi e restituisce le righe della prima rotazione in cui il
// certificato si riconosce, con il file dell'immagine raddrizzata (che sostituisce quella da mostrare).
func (a *App) certTryRotations(ctx context.Context, e *certEntry, read func(context.Context, string) ([]string, error)) ([]string, string, bool) {
	img, err := imgproc.Load(e.View)
	if err != nil {
		return nil, "", false
	}
	for _, deg := range []int{180, 90, 270} {
		if ctx.Err() != nil {
			return nil, "", false
		}
		out := filepath.Join(a.certWorkDir(), "cert-"+e.ID+".rot"+strconv.Itoa(deg)+".png")
		if err := imgproc.SavePNG(imgproc.Rotate(img, deg), out); err != nil {
			continue
		}
		rows, err := read(ctx, out)
		if err == nil {
			if _, perr := certif.Parse(rows); perr == nil {
				a.logf("certificato %s: riconosciuto dopo aver ruotato di %d gradi", e.ID, deg)
				return rows, out, true
			}
		}
		_ = os.Remove(out)
	}
	return nil, "", false
}

// certCleanImage raddrizza un foglio scansionato storto e ne toglie i margini bianchi (un A4 appoggiato in basso sul vetro lascia una
// fascia vuota in alto): l'OCR legge meglio e l'anteprima mostra solo il certificato. L'originale (e.Src) resta com'e'.
func (a *App) certCleanImage(e *certEntry) {
	img, err := imgproc.Load(e.View)
	if err != nil {
		return
	}
	changed := false
	if d, ang := imgproc.Deskew(img); math.Abs(ang) >= 0.2 && math.Abs(ang) <= 15 {
		img, changed = d, true
		a.logf("certificato %s: raddrizzato di %.1f gradi", e.ID, ang)
	}
	if crop, ok := trimWhite(img); ok {
		img, changed = crop, true
	}
	if !changed {
		return
	}
	out := filepath.Join(a.certWorkDir(), "cert-"+e.ID+".clean.png")
	if imgproc.SavePNG(img, out) == nil {
		e.View = out
	}
}

// trimWhite ritaglia l'immagine al riquadro dei punti non bianchi (con un po' di margine). Falso se non c'e' niente da togliere.
func trimWhite(img image.Image) (image.Image, bool) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 100 || h < 100 {
		return nil, false
	}
	rows, cols := make([]int, h), make([]int, w)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if (r>>8)+(g>>8)+(bl>>8) < 3*215 { // punto piu' scuro del bianco della carta
				rows[y]++
				cols[x]++
			}
		}
	}
	span := func(v []int, min int) (int, int) {
		lo, hi := 0, len(v)-1
		for lo < len(v) && v[lo] < min {
			lo++
		}
		for hi > lo && v[hi] < min {
			hi--
		}
		return lo, hi
	}
	y0, y1 := span(rows, max(3, w/200)) // ignora le righe con pochi punti (polvere, bordo dello scanner)
	x0, x1 := span(cols, max(3, h/200))
	padX, padY := w/50, h/50
	x0, x1, y0, y1 = max(0, x0-padX), min(w-1, x1+padX), max(0, y0-padY), min(h-1, y1+padY)
	if x1-x0 < w/3 || y1-y0 < h/3 || (x1-x0 > w*92/100 && y1-y0 > h*92/100) {
		return nil, false
	}
	type sub interface {
		SubImage(image.Rectangle) image.Image
	}
	s, ok := img.(sub)
	if !ok {
		return nil, false
	}
	return s.SubImage(image.Rect(b.Min.X+x0, b.Min.Y+y0, b.Min.X+x1+1, b.Min.Y+y1+1)), true
}

// certKeepUnrecognized conserva l'ultimo foglio non riconosciuto in dati\certificati\cert-nonriconosciuto.png (si sovrascrive
// a ogni nuovo caso e si cancella da solo dopo un giorno), cosi' si puo' capire perche': contiene dati personali, resta solo qui.
func (a *App) certKeepUnrecognized(e *certEntry) {
	if b, err := os.ReadFile(e.View); err == nil && len(b) > 0 {
		if err := os.WriteFile(filepath.Join(a.certWorkDir(), "cert-nonriconosciuto"+filepath.Ext(e.View)), b, 0o600); err == nil {
			a.logf("certificato %s: foglio non riconosciuto, copia in dati/certificati/cert-nonriconosciuto", e.ID)
		}
	}
}

// ocrRows e' la lettura vera: OCR di Windows e ricostruzione delle righe fisiche.
func ocrRows(ctx context.Context, img string) ([]string, error) {
	res, err := ocr.Recognize(ctx, img)
	if err != nil {
		return nil, err
	}
	return certif.RowsFromOCR(res), nil
}

var reCertID = regexp.MustCompile(`^[0-9a-f]{12}$`)

// hCertImage: GET /api/cert/image?id=... l'immagine del certificato da mostrare accanto ai campi.
func (a *App) hCertImage(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	e := a.certGet(id)
	if !reCertID.MatchString(id) || e == nil {
		writeErr(w, http.StatusNotFound, "certificato", "certificato sconosciuto")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if e.media() == "pdf" {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
	}
	http.ServeFile(w, r, e.View)
}

// hCertDiscard: POST /api/cert/discard {id}.
func (a *App) hCertDiscard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if reCertID.MatchString(req.ID) {
		a.certDrop(req.ID)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- salvataggio ----

type certSaveReq struct {
	ID     string            `json:"id"`
	Type   string            `json:"type"`
	Fields map[string]string `json:"fields"`
}

var reSafe = regexp.MustCompile(`[^A-Z0-9]+`)

// certBase e' il nome (senza estensione) dei file salvati: «MARINEID - NOME (TIPO)», per esempio «500123456 - MARIO ROSSI (SD)».
// Il Marine ID e' la chiave univoca del marittimo per Panama: i file dello stesso marittimo stanno vicini; il tipo distingue i suoi
// certificati. Senza Marine ID resta «NOME (TIPO)»; senza niente, «TIPO - data e ora».
func certBase(typ string, f map[string]string) string {
	clean := func(s string) string {
		s = strings.Map(func(r rune) rune {
			if strings.ContainsRune(`\/:*?"<>|%;`, r) || unicode.IsControl(r) {
				return ' '
			}
			return r
		}, s)
		return strings.Join(strings.Fields(strings.ToUpper(s)), " ")
	}
	var parts []string
	if m := strings.Join(strings.FieldsFunc(f[certif.FMarineID], func(r rune) bool { return !unicode.IsDigit(r) }), ""); m != "" {
		parts = append(parts, m)
	}
	if n := clean(f[certif.FName]); n != "" {
		parts = append(parts, truncate(n, 60))
	}
	if len(parts) == 0 {
		return typ + " - " + time.Now().Format("20060102-150405")
	}
	return strings.Join(parts, " - ") + " (" + typ + ")"
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n]))
	}
	return s
}

// certPDFLimit e' il peso massimo del PDF salvato (come per il libretto).
const certPDFLimit = 1_000_000

// certPDF scrive il PDF del certificato in dst, entro 1 MB: un PDF originale abbastanza piccolo si copia com'e'; un'immagine (o un
// PDF piu' grande) diventa un PDF di una pagina con pdfout, abbassando la qualita' fino a stare nel limite.
func (a *App) certPDF(ctx context.Context, e *certEntry, dst string) error {
	if strings.EqualFold(filepath.Ext(e.Src), ".pdf") {
		if b, err := os.ReadFile(e.Src); err == nil && len(b) <= certPDFLimit {
			return memoria.ScriviFile(dst, b)
		}
	}
	img := e.View
	if strings.EqualFold(filepath.Ext(img), ".pdf") { // PDF troppo grande: serve un'immagine della pagina
		img = filepath.Join(a.certWorkDir(), "cert-"+e.ID+".page.png")
		if err := pdfin.RenderFirstPage(ctx, e.Src, img, 0); err != nil {
			return errors.New("il PDF originale pesa piu' di 1 MB e non riesco a ridurlo")
		}
	}
	im, err := imgproc.Load(img)
	if err != nil {
		return err
	}
	pdf, _, err := pdfout.Encode(im, float64(im.Bounds().Dx())/8.27, certPDFLimit) // pagina larga come un A4
	if err != nil {
		return err
	}
	return memoria.ScriviFile(dst, pdf)
}

// certMaxLen e' la lunghezza massima di un campo: il titolo del corso e' una frase, gli altri sono nomi e numeri.
func certMaxLen(key string) int {
	if key == certif.FCourse {
		return 300
	}
	return 80
}

// certClean controlla i campi che arrivano dalla pagina: solo le chiavi note, valori corti e senza caratteri di controllo.
func certClean(in map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range in {
		if _, ok := certLabels[k]; !ok {
			return nil, errors.New("campo sconosciuto: " + k)
		}
		v = strings.Join(strings.Fields(v), " ")
		if len([]rune(v)) > certMaxLen(k) {
			return nil, errors.New("valore troppo lungo nel campo " + k)
		}
		for _, r := range v {
			if unicode.IsControl(r) {
				return nil, errors.New("caratteri non validi nel campo " + k)
			}
		}
		if v != "" {
			out[k] = v
		}
	}
	return out, nil
}

// hCertSave: POST /api/cert/save. Scrive il JSON e una copia del file nella cartella di salvataggio, sotto «certificati».
func (a *App) hCertSave(w http.ResponseWriter, r *http.Request) {
	var req certSaveReq
	if !readJSON(w, r, &req) {
		return
	}
	e := a.certGet(req.ID)
	if !reCertID.MatchString(req.ID) || e == nil {
		writeErr(w, http.StatusNotFound, "certificato", "Il certificato non c'è più: rileggilo.")
		return
	}
	typ := req.Type
	if _, ok := certif.Required[typ]; !ok {
		writeErr(w, http.StatusBadRequest, "tipo", "Tipo di certificato non valido.")
		return
	}
	fields, err := certClean(req.Fields)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "campi", err.Error())
		return
	}
	var missing []string
	for _, k := range certif.Required[typ] {
		if fields[k] == "" {
			missing = append(missing, k)
		}
	}
	dir := filepath.Join(a.outDir(), certDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErr(w, http.StatusInternalServerError, "cartella", "Cartella di salvataggio non disponibile: "+err.Error())
		return
	}
	base := certBase(typ, fields)
	name := base
	for i := 2; fileExists(filepath.Join(dir, name+".json")) || fileExists(filepath.Join(dir, name+".pdf")); i++ {
		if i > 99 {
			writeErr(w, http.StatusConflict, "esiste", "Troppi file con lo stesso nome nella cartella.")
			return
		}
		name = base + " - " + strconv.Itoa(i)
	}
	if err := a.certPDF(r.Context(), e, filepath.Join(dir, name+".pdf")); err != nil {
		writeErr(w, http.StatusInternalServerError, "pdf", "PDF non salvato: "+err.Error())
		return
	}
	pdfSize := int64(0)
	if st, err := os.Stat(filepath.Join(dir, name+".pdf")); err == nil {
		pdfSize = st.Size()
	}
	ext := ".pdf"
	out := map[string]any{
		"schema_version": 1, "kind": "certificate", "type": typ, "fields": fields, "missing": missing,
		"source_file": name + ext, "pdf_bytes": pdfSize, "saved_at": time.Now().Format(time.RFC3339), "app_version": version,
	}
	if e.QR != "" {
		out["qr_url"] = e.QR
	}
	if e.Verif != nil {
		out["online_check"] = map[string]any{"checked_at": e.Verif.CheckedA, "status": e.Verif.Status, "all_ok": e.Verif.AllOK}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	if err := memoria.ScriviFile(filepath.Join(dir, name+".json"), append(b, '\n')); err != nil {
		writeErr(w, http.StatusInternalServerError, "scrittura", "JSON non salvato: "+err.Error())
		return
	}
	a.certDrop(e.ID)
	a.logf("certificato %s salvato (%s)", typ, name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "json": name + ".json", "file": name + ext, "dir": certDirName, "missing": missing})
}

// certDisplayName ripulisce il nome del file scelto (solo per mostrarlo): niente percorsi, caratteri di controllo e lunghezze assurde.
func certDisplayName(n, ext string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	n = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, n)
	if n == "." || n == "/" || n == "" {
		return "certificato" + ext
	}
	if r := []rune(n); len(r) > 80 {
		n = string(r[:80])
	}
	return n
}
