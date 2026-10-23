package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"scanlibretto/internal/filename"
)

// scanExts sono i file di una scansione che viaggiano insieme con lo stesso nome base.
var scanExts = []string{".png", ".jpg", ".jpeg", ".json", ".pdf", ".err"}

// futureName calcola il nome che i file avranno dopo "Conferma e salva": "NOME - COGNOME - NUMERO LIBRETTO"
// (internal/filename: solo lettere, cifre e spazi, sicuro per Windows) dai valori CORRENTI del form, quindi cambia se
// l'operatore corregge nomi, cognomi o numero libretto. Se esiste già un file con quel nome si aggiunge "(2)" (Unique:
// mai sovrascrivere). ok è falso se manca nome, cognome o numero libretto (i file restano col nome scelto).
// La condizione "MRZ valida" non serve: l'operatore ha controllato i dati. Il numero libretto, se il form lo ha vuoto,
// si prende dall'MRZ solo quando è valida.
func (a *App) futureName(d *Doc, final map[string]string) (name string, ok bool) {
	book := strings.TrimSpace(final["seamans_book"])
	if book == "" && mrzValid(d.Out.MRZ) {
		book = strings.TrimSpace(d.Out.Fields["seamans_book_mrz"])
		if book == "" {
			book = strings.Trim(strings.TrimSpace(d.Out.MRZ.DocNumber), "<")
		}
	}
	// fields.given_names = nomi, fields.surname = cognomi (l'ordine dei libretti di Panama è già giusto nell'uscita)
	base, ok := filename.Base(filename.Parts{Given: final["given_names"], Surname: final["surname"], Book: book})
	if !ok {
		return "", false
	}
	// il documento ha già questo nome (anche con "(2)" di una conferma precedente): non si cambia a ogni conferma
	if d.Name == base || (strings.HasPrefix(d.Name, base+" (") && strings.HasSuffix(d.Name, ")")) {
		return d.Name, true
	}
	return filename.Unique(a.scansDir, base, scanExts...), true
}

// renameAll rinomina old+ext in final+ext per ogni estensione presente; se un passo fallisce rimette a posto i precedenti.
func renameAll(dir, old, final string) error {
	var done []string
	for _, e := range scanExts {
		from := filepath.Join(dir, old+e)
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if err := os.Rename(from, filepath.Join(dir, final+e)); err != nil {
			for _, d := range done {
				_ = os.Rename(filepath.Join(dir, final+d), filepath.Join(dir, old+d))
			}
			return errors.Join(fmt.Errorf("rinomina di %s", old+e), err)
		}
		done = append(done, e)
	}
	return nil
}

// hPreviewName risponde a "come si chiameranno i file dopo la conferma?" con i valori correnti del form.
func (a *App) hPreviewName(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DocID    string            `json:"doc_id"`
		Values   map[string]string `json:"values"`
		Fallback string            `json:"fallback_name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	d := a.getDoc(req.DocID)
	if d == nil {
		writeErr(w, http.StatusNotFound, "doc", "documento sconosciuto")
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	final := make(map[string]string, len(fieldDefs))
	for _, f := range fieldDefs {
		v, ok := req.Values[f.Key]
		if !ok {
			v = d.Initial[f.Key]
		}
		final[f.Key] = normalizeValue(v)
	}
	name, ok := a.futureName(d, final)
	resp := map[string]any{"ok": ok, "name": name, "current": d.Name, "same": ok && name == d.Name, "unsaved": d.Dir == dirPreview}
	if !ok && d.Dir == dirPreview {
		resp["fallback"] = a.fallbackName(req.Fallback) // nome che avranno i file se non si completano i dati
	}
	writeJSON(w, http.StatusOK, resp)
}
