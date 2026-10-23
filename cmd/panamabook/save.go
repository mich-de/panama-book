package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"net/http"

	"scanlibretto/internal/memoria"
	"scanlibretto/internal/outsum"
)

func urlEscape(s string) string { return url.QueryEscape(s) }

// normalizeValue pulisce quanto scritto dall'operatore: spazi singoli, maiuscolo (sul libretto è tutto maiuscolo).
func normalizeValue(s string) string {
	s = strings.ToUpper(strings.Join(strings.Fields(s), " "))
	if r := []rune(s); len(r) > 120 {
		s = string(r[:120])
	}
	return s
}

// dateISO converte GG-MM-AAAA in AAAA-MM-GG.
func dateISO(v string) (string, bool) {
	t, err := time.Parse("02-01-2006", v)
	if err != nil {
		return "", false
	}
	return t.Format("2006-01-02"), true
}

type saveReq struct {
	DocID     string            `json:"doc_id"`
	Mode      string            `json:"mode"` // "corrections": solo memoria; "confirm": memoria, verità e scans\NOME.json
	Values    map[string]string `json:"values"`
	Overwrite bool              `json:"overwrite"`
	// Reviewed: i campi "da controllare" che l'operatore ha segnato "controllato" (anche da memoria, se non li ha annullati).
	Reviewed []string `json:"reviewed"`
	// AcceptOpen: conferma anche se restano campi da controllare (la verità non li scrive).
	AcceptOpen bool `json:"accept_open"`
	// FallbackName: nome dei file se mancano nome, cognome o numero libretto (solo per le anteprime non ancora salvate).
	FallbackName string `json:"fallback_name"`
	// KeepRaw: conserva anche la scansione grezza come "NOME (grezza).png" (solo salvando un'anteprima).
	KeepRaw bool `json:"keep_raw"`
	// KeepFull: conserva anche la scansione intera come "NOME (intera).png" (solo se la lettura ha prodotto la pagina dati ritagliata).
	KeepFull bool `json:"keep_full"`
}

// reviewItem è un campo segnato "controllato" nel salvataggio.
type reviewItem struct {
	Field  string `json:"field"`
	Source string `json:"source"` // "user" | "memory"
}

type correction struct {
	Field  string `json:"field"`
	Source string `json:"source"` // "user" | "memory"
	Read   string `json:"read"`
}

// hSave salva le correzioni (memoria) e, con mode "confirm", anche la verità per il verificatore e scans\NOME.json.
func (a *App) hSave(w http.ResponseWriter, r *http.Request) {
	var req saveReq
	if !readJSON(w, r, &req) {
		return
	}
	if req.Mode != "corrections" && req.Mode != "confirm" {
		writeErr(w, http.StatusBadRequest, "modo", "modo di salvataggio sconosciuto")
		return
	}
	d := a.getDoc(req.DocID)
	if d == nil {
		writeErr(w, http.StatusNotFound, "doc", "documento sconosciuto: rileggi l'immagine")
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
	var corr []correction
	for _, f := range fieldDefs {
		if final[f.Key] == d.Read[f.Key] {
			continue
		}
		src := "user"
		if ap, ok := d.Applied[f.Key]; ok && ap.Corretto == final[f.Key] {
			src = "memory"
		}
		corr = append(corr, correction{Field: f.Key, Source: src, Read: d.Read[f.Key]})
	}

	// campi da controllare: segnati controllati o ancora aperti
	revSet := map[string]bool{}
	for _, k := range req.Reviewed {
		if _, ok := findDef(k); ok {
			revSet[k] = true
		}
	}
	var reviewed []reviewItem
	var open []string
	confirmNew := map[string]string{}
	var confirmDrop []string
	for _, f := range fieldDefs {
		k, v := f.Key, final[f.Key]
		unedited := v == d.Read[k]
		if !unedited || len(d.reasons(f, v)) == 0 { // non c'è nulla da controllare su questo campo
			if _, ok := d.Confirmed[k]; ok && !unedited {
				confirmDrop = append(confirmDrop, k) // il valore è cambiato: la conferma ricordata non vale più
			}
			continue
		}
		if revSet[k] {
			src := "user"
			if d.Reviewed[k] == "memory" {
				src = "memory"
			}
			reviewed = append(reviewed, reviewItem{Field: k, Source: src})
			if c, ok := d.Confirmed[k]; !ok || c.Valore != v {
				confirmNew[k] = v
			}
		} else {
			open = append(open, k)
			if d.Reviewed[k] == "memory" { // l'operatore ha rimesso "da controllare" un campo confermato dalla memoria
				confirmDrop = append(confirmDrop, k)
			}
		}
	}
	if req.Mode == "confirm" && len(open) > 0 && !req.AcceptOpen {
		items := make([]map[string]string, 0, len(open))
		for _, k := range open {
			items = append(items, map[string]string{"key": k, "label": fieldLabelIT(k)})
		}
		writeJSON(w, http.StatusConflict, map[string]any{"code": "campi_aperti", "open": items,
			"error": "Restano dei campi da controllare."})
		return
	}

	// nome dei file: la rinomina avviene SOLO con "Conferma e salva", dai valori confermati del form
	newName := d.Name
	isPrev := d.Dir == dirPreview
	var msgs []string
	if req.Mode == "confirm" {
		if nm, ok := a.futureName(d, final); ok {
			newName = nm
		} else if isPrev {
			newName = a.fallbackName(req.FallbackName)
			msgs = append(msgs, "Mancano nome, cognome o numero libretto: i file si chiamano «"+newName+"».")
		} else {
			msgs = append(msgs, "Mancano nome, cognome o numero libretto: i file restano col nome «"+d.Name+"».")
		}
	}
	jsonPath := filepath.Join(a.scansDir, newName+".json")
	truthPath := filepath.Join(a.truthDir, newName+".json")
	if req.Mode == "confirm" && !req.Overwrite {
		var existing []string
		names := []string{newName}
		if newName != d.Name && !isPrev {
			names = append(names, d.Name) // anche i file di una conferma precedente di questo documento
		}
		for _, n := range names {
			if fileExists(filepath.Join(a.truthDir, n+".json")) {
				existing = append(existing, filepath.Join("scans", "verita", n+".json"))
			}
			if fileExists(filepath.Join(a.scansDir, n+".json")) {
				existing = append(existing, filepath.Join("scans", n+".json"))
			}
		}
		if len(existing) > 0 {
			writeJSON(w, http.StatusConflict, map[string]any{"code": "esistono_file", "existing": existing,
				"error": "Esistono già dei file con questo nome: confermi di sostituirli? (i vecchi file della verità si conservano con il suffisso .prec)"})
			return
		}
	}

	resp := map[string]any{"ok": true, "mode": req.Mode}

	// memoria
	var toSave map[string]memoria.Correzione
	for _, c := range corr {
		if c.Source == "user" {
			if toSave == nil {
				toSave = map[string]memoria.Correzione{}
			}
			toSave[c.Field] = memoria.Correzione{Corretto: final[c.Field], Letto: c.Read}
		}
	}
	var forget []string
	for field := range d.Applied {
		if final[field] == d.Read[field] { // l'operatore ha rimesso il valore letto: la correzione ricordata non vale più
			forget = append(forget, field)
		}
	}
	sort.Strings(forget)
	memSaved, memForgot := 0, 0
	switch {
	case d.Key == "" && len(toSave) > 0:
		msgs = append(msgs, "Correzioni NON memorizzate: l'MRZ non è valida, il libretto non si può riconoscere con sicurezza.")
		resp["memory_skipped"] = true
	case d.Key != "":
		for _, f := range forget {
			if err := a.mem.DimenticaCampo(d.Key, f); err != nil {
				writeErr(w, http.StatusInternalServerError, "memoria", "Memoria non aggiornata: "+err.Error())
				return
			}
			memForgot++
		}
		if len(toSave) > 0 {
			label := strings.TrimSpace(final["surname"] + " " + final["given_names"])
			number := ""
			if d.Out.MRZ != nil {
				number = strings.Trim(strings.TrimSpace(d.Out.MRZ.DocNumber), "<")
			}
			if err := a.mem.Salva(d.Key, label, number, toSave); err != nil {
				writeErr(w, http.StatusInternalServerError, "memoria", "Memoria non salvata: "+err.Error())
				return
			}
			memSaved = len(toSave)
		}
	}
	// conferme "valore X controllato per questo libretto"
	confSaved := 0
	if d.Key != "" {
		if len(confirmNew) > 0 {
			label := strings.TrimSpace(final["surname"] + " " + final["given_names"])
			number := ""
			if d.Out.MRZ != nil {
				number = strings.Trim(strings.TrimSpace(d.Out.MRZ.DocNumber), "<")
			}
			if err := a.mem.SalvaConferme(d.Key, label, number, confirmNew); err != nil {
				writeErr(w, http.StatusInternalServerError, "memoria", "Conferme non salvate: "+err.Error())
				return
			}
			confSaved = len(confirmNew)
		}
		if len(confirmDrop) > 0 {
			if err := a.mem.RimuoviConferme(d.Key, confirmDrop...); err != nil {
				writeErr(w, http.StatusInternalServerError, "memoria", "Conferme non aggiornate: "+err.Error())
				return
			}
		}
	} else if len(confirmNew) > 0 {
		msgs = append(msgs, "Le conferme NON sono state ricordate: l'MRZ non è valida, il libretto non si può riconoscere con sicurezza.")
		resp["memory_skipped"] = true
	}
	resp["confirmed_saved"] = confSaved
	if confSaved > 0 {
		msgs = append(msgs, fmt.Sprintf("Ricordati %d campi controllati: la prossima volta, se la lettura è la stessa, saranno già controllati.", confSaved))
	}
	if memSaved > 0 {
		msgs = append(msgs, fmt.Sprintf("Memorizzati %d campi: alla prossima scansione di questo libretto saranno corretti da soli.", memSaved))
	}
	if memForgot > 0 {
		msgs = append(msgs, fmt.Sprintf("Dimenticati %d campi che avevi rimesso al valore letto.", memForgot))
	}
	if req.Mode == "corrections" && memSaved == 0 && memForgot == 0 && confSaved == 0 && len(confirmDrop) == 0 && d.Key != "" {
		msgs = append(msgs, "Nessuna correzione nuova da memorizzare.")
	}
	resp["memory_saved"], resp["memory_forgotten"] = memSaved, memForgot

	// verità e JSON
	if req.Mode == "confirm" {
		if isPrev { // anteprima temporanea: SOLO ORA l'immagine (ed eventuale PDF) passa in scans\ col nome definitivo
			usedPage, err := a.movePreview(d, newName, req.KeepRaw, req.KeepFull)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "salvataggio", "Immagine non salvata in scans: "+err.Error())
				return
			}
			resp["renamed"] = true
			resp["used_page"] = usedPage
			if usedPage {
				msgs = append(msgs, "Salvata in scans la sola pagina dati ritagliata come «"+newName+"».")
			} else {
				msgs = append(msgs, "La pagina dati non è stata trovata (MRZ non rilevata): salvata in scans l'anteprima così com'è, come «"+newName+"».")
			}
		} else if newName != d.Name { // rinomina tutto-o-niente di .png/.jpg/.json/.pdf/.err; la verità già scritta per questo documento la segue
			if err := renameAll(a.scansDir, d.Name, newName); err != nil {
				writeErr(w, http.StatusInternalServerError, "rinomina", "Rinomina dei file non riuscita (non è stato cambiato nulla): "+err.Error())
				return
			}
			if oldTruth := filepath.Join(a.truthDir, d.Name+".json"); fileExists(oldTruth) && !fileExists(truthPath) {
				_ = os.Rename(oldTruth, truthPath)
			}
			d.File, d.Name = newName+fileExt(d.File), newName
			resp["renamed"] = true
			msgs = append(msgs, "File rinominati in «"+newName+"» (nome - cognome - numero libretto).")
		}
		if fileExists(truthPath) {
			bak := truthPath + ".prec-" + time.Now().Format("20060102-150405")
			if err := os.Rename(truthPath, bak); err != nil {
				writeErr(w, http.StatusInternalServerError, "verita", "Non riesco a conservare la verità precedente: "+err.Error())
				return
			}
			resp["truth_backup"] = filepath.Join("scans", "verita", filepath.Base(bak))
		}
		if err := memoria.ScriviFile(truthPath, buildTruth(final, corr, open)); err != nil {
			writeErr(w, http.StatusInternalServerError, "verita", "Verità non scritta: "+err.Error())
			return
		}
		out, err := buildSavedJSON(d.Raw, final, corr, reviewed, open)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "json", "JSON non costruito: "+err.Error())
			return
		}
		if err := memoria.ScriviFile(jsonPath, out); err != nil {
			writeErr(w, http.StatusInternalServerError, "json", "JSON non scritto: "+err.Error())
			return
		}
		resp["truth"] = filepath.Join("scans", "verita", d.Name+".json")
		resp["json"] = filepath.Join("scans", d.Name+".json")
		msgs = append(msgs, "Salvati scans\\"+d.Name+".json e la verità per la regressione (scans\\verita\\"+d.Name+".json).")
	}

	// nuovo stato di riferimento: quello salvato
	d.Initial = final
	d.Applied = map[string]memoria.Correzione{}
	d.Reviewed = map[string]string{}
	for _, r := range reviewed {
		d.Reviewed[r.Field] = r.Source
	}
	if d.Key != "" {
		d.Confirmed = a.mem.Confermati(d.Key)
		for field, c := range a.mem.Campi(d.Key) {
			if _, ok := findDef(field); ok && d.Read[field] != c.Corretto && final[field] == c.Corretto {
				d.Applied[field] = c
			}
		}
	}
	dv := d.view(a.mem, a.jsonExists(d.Name))
	dv.ImageURL = imageURL(d.Dir, d.File)
	resp["doc"] = dv
	resp["messages"] = msgs
	writeJSON(w, http.StatusOK, resp)
}

// buildTruth costruisce scans\verita\NOME.json: i 14 campi stampati con i nomi inglesi (formato di internal/verify) e i
// metadati con "_". Dati personali: il file sta in scans\, ignorata da git.
// I campi ancora "da controllare" (open) non si scrivono: restano vuoti, con una nota (vuoto = non si confronta).
func buildTruth(final map[string]string, corr []correction, open []string) []byte {
	t := map[string]any{}
	isOpen := map[string]bool{}
	for _, k := range open {
		isOpen[k] = true
	}
	notes := map[string]string{}
	for _, k := range outsum.FieldKeys {
		if isOpen[k] {
			t[k] = ""
			notes[k] = "non controllato dall'operatore: lasciato vuoto"
			continue
		}
		t[k] = final[k]
	}
	t["_origine"] = "PanamaBook: valori confermati dall'operatore"
	if n := strings.TrimSpace(final["given_names"] + " " + final["surname"]); n != "" && !isOpen["given_names"] && !isOpen["surname"] {
		t["_riga_nome"] = n
	}
	for _, c := range corr {
		if _, ok := t[c.Field]; !ok || isOpen[c.Field] {
			continue
		}
		if c.Source == "memory" {
			notes[c.Field] = "corretto dalla memoria di PanamaBook e confermato dall'operatore"
		} else {
			notes[c.Field] = "corretto a mano dall'operatore (il lettore aveva letto un altro valore)"
		}
	}
	if len(notes) > 0 {
		t["_note"] = notes
	}
	b, _ := json.MarshalIndent(t, "", "  ")
	return append(b, '\n')
}

// buildSavedJSON parte dal JSON dell'estrazione, sostituisce i campi con quelli confermati e aggiunge il blocco corrections.
func buildSavedJSON(raw []byte, final map[string]string, corr []correction, reviewed []reviewItem, open []string) ([]byte, error) {
	if i := strings.IndexByte(string(raw), '{'); i > 0 {
		raw = raw[i:]
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	fields, _ := m["fields"].(map[string]any)
	if fields == nil {
		fields = map[string]any{}
	}
	delete(fields, "endorsement") // vecchio nome di certificate
	for _, f := range fieldDefs {
		v := final[f.Key]
		if v == "" {
			delete(fields, f.Key)
		} else {
			fields[f.Key] = v
		}
		if f.Kind == "data" {
			if iso, ok := dateISO(v); ok {
				fields[f.Key+"_iso"] = iso
			} else {
				delete(fields, f.Key+"_iso")
			}
		}
	}
	m["fields"] = fields
	if corr == nil {
		corr = []correction{}
	}
	m["corrections"] = corr
	if reviewed == nil {
		reviewed = []reviewItem{}
	}
	if open == nil {
		open = []string{}
	}
	// review: campi segnati "controllato" (solo nome e fonte, nessun valore) e campi lasciati da controllare
	m["review"] = map[string]any{"reviewed": reviewed, "unchecked": open}
	m["panamabook"] = map[string]any{"version": version, "confirmed_at": time.Now().Format(time.RFC3339)}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// ---- memoria: elenco e dimenticanza --------------------------------------------------------------------------------

type memField struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	Corretto string `json:"corretto"`
	Letto    string `json:"letto"`
	Data     string `json:"data"`
	Tipo     string `json:"tipo"` // "correzione" | "conferma" (per la conferma Corretto e' il valore confermato)
}

type memItem struct {
	Key        string     `json:"key"`
	Etichetta  string     `json:"etichetta"`
	Numero     string     `json:"numero"`
	Aggiornato string     `json:"aggiornato"`
	Campi      []memField `json:"campi"`
}

func (a *App) hMemory(w http.ResponseWriter, r *http.Request) {
	items := []memItem{}
	for _, v := range a.mem.Elenco() {
		it := memItem{Key: v.Chiave, Etichetta: v.Etichetta, Numero: v.Numero, Aggiornato: v.Aggiornato.Format(time.RFC3339)}
		for _, f := range fieldDefs {
			if c, ok := v.Campi[f.Key]; ok {
				it.Campi = append(it.Campi, memField{Field: f.Key, Label: f.ES + " / " + f.EN, Corretto: c.Corretto, Letto: c.Letto, Data: c.Data.Format("2006-01-02"), Tipo: "correzione"})
			}
			if c, ok := v.Confermati[f.Key]; ok {
				it.Campi = append(it.Campi, memField{Field: f.Key, Label: f.ES + " / " + f.EN, Corretto: c.Valore, Data: c.Data.Format("2006-01-02"), Tipo: "conferma"})
			}
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// forgetInDocs allinea i documenti aperti dopo che la memoria ha dimenticato qualcosa.
// kind: "conferma" = solo le conferme, "correzione" = solo le correzioni, "" = tutto.
func (a *App) forgetInDocs(key, field, kind string) {
	a.docsMu.Lock()
	defer a.docsMu.Unlock()
	for _, d := range a.docs {
		d.mu.Lock()
		if d.Key == key {
			if kind != "conferma" {
				for f, ap := range d.Applied {
					if field == "" || f == field {
						if d.Initial[f] == ap.Corretto {
							d.Initial[f] = d.Read[f]
						}
						delete(d.Applied, f)
					}
				}
			}
			if kind != "correzione" {
				for f := range d.Confirmed {
					if field == "" || f == field {
						delete(d.Confirmed, f)
						if d.Reviewed[f] == "memory" {
							delete(d.Reviewed, f)
						}
					}
				}
			}
		}
		d.mu.Unlock()
	}
}

func (a *App) hForgetField(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DocID string `json:"doc_id"`
		Field string `json:"field"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	d := a.getDoc(req.DocID)
	if d == nil {
		writeErr(w, http.StatusNotFound, "doc", "documento sconosciuto")
		return
	}
	if _, ok := findDef(req.Field); !ok {
		writeErr(w, http.StatusBadRequest, "campo", "campo sconosciuto")
		return
	}
	d.mu.Lock()
	key := d.Key
	d.mu.Unlock()
	if key == "" {
		writeErr(w, http.StatusConflict, "senza_chiave", "Questo libretto non ha una MRZ valida: non c'è nessuna memoria da togliere.")
		return
	}
	if err := a.mem.DimenticaCampo(key, req.Field); err != nil {
		writeErr(w, http.StatusInternalServerError, "memoria", err.Error())
		return
	}
	a.forgetInDocs(key, req.Field, "correzione")
	writeJSON(w, http.StatusOK, a.docView(d))
}

func (a *App) hForget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string `json:"key"`
		Field string `json:"field"` // vuoto = tutto il libretto
		Kind  string `json:"kind"`  // "conferma" | "correzione" (con field)
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Key == "" {
		writeErr(w, http.StatusBadRequest, "chiave", "manca il libretto")
		return
	}
	var err error
	switch {
	case req.Field == "":
		err = a.mem.DimenticaLibretto(req.Key)
		req.Kind = ""
	case req.Kind == "conferma":
		err = a.mem.RimuoviConferme(req.Key, req.Field)
	default:
		req.Kind = "correzione"
		err = a.mem.DimenticaCampo(req.Key, req.Field)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "memoria", err.Error())
		return
	}
	a.forgetInDocs(req.Key, req.Field, req.Kind)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- esportazione PDF ----------------------------------------------------------------------------------------------

// pdfExe sceglie l'eseguibile che sa fare "pdf": libretto.exe se la sua guida contiene "libretto pdf", altrimenti il
// candidato di ripiego (_out\libretto_pdf.exe). Il risultato positivo si ricorda.
func (a *App) pdfExe(ctx context.Context) (string, bool) {
	a.pdfMu.Lock()
	defer a.pdfMu.Unlock()
	if a.pdfPath != "" {
		return a.pdfPath, true
	}
	for _, exe := range []string{a.cfg.Exe, a.cfg.PDFFallback} {
		if exe == "" || !fileExists(exe) {
			continue
		}
		supports := false
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		// senza argomenti libretto stampa solo la guida (non avvia nulla)
		_, _, _ = a.runExe(cctx, exe, nil, func(l string) {
			if strings.Contains(l, "libretto pdf") {
				supports = true
			}
		})
		cancel()
		if supports {
			a.pdfPath = exe
			return exe, true
		}
	}
	return "", false
}

func (a *App) hPDF(w http.ResponseWriter, r *http.Request) {
	var req struct {
		File      string `json:"file"`
		Dir       string `json:"dir"` // "preview": il PDF va in dati\anteprima\ (accanto all'anteprima), non in scans\
		Overwrite bool   `json:"overwrite"`
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
	name := strings.TrimSuffix(req.File, filepath.Ext(req.File))
	pdfRel := relPath(req.Dir, name+".pdf")
	pdfAbs := filepath.Join(a.cfg.Base, pdfRel)
	if fileExists(pdfAbs) && !req.Overwrite {
		writeJSON(w, http.StatusConflict, map[string]any{"code": "esiste", "file": pdfRel,
			"error": "Il PDF " + pdfRel + " esiste già: vuoi sostituirlo?"})
		return
	}
	exe, ok := a.pdfExe(r.Context())
	if !ok {
		writeErr(w, http.StatusFailedDependency, "manca_pdf", "Questa versione di libretto.exe non sa ancora creare il PDF (comando 'libretto pdf'), e _out\\libretto_pdf.exe non c'è.")
		return
	}
	in := relPath(req.Dir, req.File)
	if req.Dir == dirPreview && fileExists(filepath.Join(a.previewDir(), pageName(req.File))) {
		in = relPath(req.Dir, pageName(req.File)) // il PDF e' della sola pagina dati, come l'immagine che si salvera'
	}
	args := []string{"pdf", "-i", in, "-o", pdfRel}
	if req.Overwrite {
		args = append(args, "-force")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	var log []string
	stdout, code, err := a.runExe(ctx, exe, args, func(l string) { log = append(log, l) })
	if err != nil || code != 0 {
		msg := "Il PDF non è stato creato"
		if last := lastErrLine(log); last != "" {
			msg += " (" + last + ")"
		}
		writeErr(w, http.StatusInternalServerError, "pdf", msg+".")
		return
	}
	st, err := os.Stat(pdfAbs)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "pdf", "Il PDF non è stato creato.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"file": pdfRel, "bytes": st.Size(), "kb": (st.Size() + 512) / 1024,
		"limit_ok": st.Size() <= 1_000_000, "info": strings.TrimSpace(string(stdout)), "preview": req.Dir == dirPreview})
}
