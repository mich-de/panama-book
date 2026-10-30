package main

import (
	"context"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"scanlibretto/internal/qrcode"
	"scanlibretto/internal/seafarer"
)

// Controllo doppio col QR anche per il LIBRETTO: la pagina dati ha un QR con il link al servizio di verifica dell'AMP (lo stesso dei
// certificati). Si legge dall'immagine del libretto (a finestre, perche' e' piccolo e sta sotto l'ologramma) e, su richiesta,
// si confrontano i campi correnti della scheda con la pagina online. Regole di sicurezza: vedi internal/seafarer.

// bookImagePath e' l'immagine del libretto da cui leggere il QR.
func (a *App) bookImagePath(d *Doc) string {
	if d.Dir == dirPreview {
		return filepath.Join(a.previewDir(), d.File)
	}
	return filepath.Join(a.outDir(), d.File)
}

// hBookQR: GET /api/doc/{id}/qr. Cerca il QR (una sola volta per libretto) e dice se e' un link del servizio AMP.
func (a *App) hBookQR(w http.ResponseWriter, r *http.Request) {
	d := a.getDoc(r.PathValue("id"))
	if d == nil {
		writeErr(w, http.StatusNotFound, "doc", "documento sconosciuto")
		return
	}
	d.mu.Lock()
	done, qr, path := d.qrDone, d.QR, a.bookImagePath(d)
	d.mu.Unlock()
	if !done {
		qr = readQRFile(path)
		d.mu.Lock()
		d.QR, d.qrDone = qr, true
		d.mu.Unlock()
		a.logf("libretto %s: QR %s", d.Name, map[bool]string{true: "letto", false: "non letto"}[qr != ""])
	}
	writeJSON(w, http.StatusOK, map[string]any{"qr": qr, "qr_amp": seafarer.ValidURL(qr)})
}

// readQRFile decodifica il QR di un file immagine; "" se non c'e', non si legge o il testo e' strano.
func readQRFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	im, _, err := image.Decode(f)
	if err != nil {
		return ""
	}
	txt, err := qrcode.Decode(im)
	if err != nil || len(txt) > 300 || strings.ContainsFunc(txt, unicode.IsControl) {
		return ""
	}
	return txt
}

// hBookVerify: POST /api/doc/{id}/verify {fields}. Stessa logica dei certificati, con i campi del libretto.
func (a *App) hBookVerify(w http.ResponseWriter, r *http.Request) {
	d := a.getDoc(r.PathValue("id"))
	if d == nil {
		writeErr(w, http.StatusNotFound, "doc", "documento sconosciuto")
		return
	}
	var req struct {
		Fields map[string]string `json:"fields"`
		Link   string            `json:"link"` // costruito dalla pagina col «Nº Título» quando il QR non si e' letto
	}
	if !readJSON(w, r, &req) {
		return
	}
	d.mu.Lock()
	qr := d.QR
	d.mu.Unlock()
	if !seafarer.ValidURL(qr) && seafarer.ValidURL(req.Link) { // stessa regola rigida: solo il servizio AMP
		qr = strings.TrimSpace(req.Link)
	}
	if !seafarer.ValidURL(qr) {
		writeErr(w, http.StatusConflict, "senza_qr", "Il libretto non ha un codice QR del servizio AMP letto: niente da controllare online.")
		return
	}
	fields := map[string]string{}
	for k, v := range req.Fields {
		if _, ok := findDef(k); !ok || len([]rune(v)) > 80 || strings.ContainsFunc(v, unicode.IsControl) {
			continue
		}
		fields[k] = strings.Join(strings.Fields(v), " ")
	}
	look := a.certs.lookup
	if look == nil {
		c := seafarer.NewClient()
		look = func(ctx context.Context, link string) (seafarer.Record, error) { return seafarer.Fetch(ctx, c, link) }
	}
	ctx, cancel := context.WithTimeout(r.Context(), seafarer.Timeout+2*time.Second)
	defer cancel()
	rec, err := look(ctx, qr)
	if err != nil {
		a.logf("libretto %s: controllo online non riuscito: %v", d.Name, err)
		writeErr(w, http.StatusBadGateway, "servizio", err.Error())
		return
	}
	v := &CertVerify{URL: qr, Status: rec["status"], Checks: seafarer.CompareBook(fields, rec), AllOK: true, CheckedA: time.Now().Format(time.RFC3339)}
	for _, c := range v.Checks {
		v.AllOK = v.AllOK && c.OK
	}
	v.AllOK = v.AllOK && len(v.Checks) > 0
	a.logf("libretto %s: controllo online fatto (%d campi, tutto uguale: %v)", d.Name, len(v.Checks), v.AllOK)
	writeJSON(w, http.StatusOK, v)
}
