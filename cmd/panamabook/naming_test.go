package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var scanBody = map[string]any{"name": "tmp1", "driver": "twain", "dpi": 300, "len_mm": 97, "top_margin_mm": 2}

func (e *env) scanDone(body map[string]any) map[string]any {
	e.t.Helper()
	code, r := e.do("POST", "/api/scan", body)
	if code != 202 {
		e.t.Fatalf("scan: %d %v", code, r)
	}
	j := e.waitJob(r["job"].(string))
	if j["phase"] != "done" {
		e.t.Fatalf("scan non riuscita: %v", j)
	}
	return j
}

func (e *env) previewName(docID string, values map[string]string) map[string]any {
	e.t.Helper()
	code, r := e.do("POST", "/api/preview-name", map[string]any{"doc_id": docID, "values": values})
	if code != 200 {
		e.t.Fatalf("preview-name: %d %v", code, r)
	}
	return r
}

func (e *env) exists(parts ...string) bool {
	_, err := os.Stat(filepath.Join(append([]string{e.base}, parts...)...))
	return err == nil
}

// Dopo la lettura i file restano col nome scelto dall'operatore e non si rinomina niente, qualunque sia l'MRZ.
func TestNessunaRinominaDopoLaLettura(t *testing.T) {
	for _, mode := range []string{"ok", "mrz_invalid", "no_mrz"} {
		e := newEnv(t, nil, "FAKE_EXTRACT="+mode)
		scans := filepath.Join(e.base, "scans")
		_ = os.MkdirAll(scans, 0o700)
		_ = os.WriteFile(filepath.Join(scans, "tmp1.err"), []byte("x"), 0o600)
		j := e.scanDone(scanBody)
		doc := j["doc"].(map[string]any)
		if doc["file"] != "tmp1.png" || doc["name"] != "tmp1" || !e.exists("scans", "tmp1.png") || !e.exists("scans", "tmp1.err") {
			t.Errorf("%s: file = %v", mode, doc["file"])
		}
		if ents, _ := os.ReadDir(scans); len(ents) != 2 {
			t.Errorf("%s: file in scans = %d, attesi 2 (immagine e log)", mode, len(ents))
		}
		if doc["rename_note"] != nil || doc["renamed"] != nil {
			t.Errorf("%s: nessun avviso di rinomina dopo la lettura: %v", mode, doc)
		}
	}
}

func TestSalvaCorrezioniNonRinomina(t *testing.T) {
	e := newEnv(t, nil)
	doc := e.scanDone(scanBody)["doc"].(map[string]any)
	code, r := e.save(doc["doc_id"].(string), "corrections", map[string]string{"surname": "BIANCHI"}, nil, nil)
	if code != 200 || r["renamed"] != nil {
		t.Fatalf("salva correzioni: %d %v", code, r)
	}
	if !e.exists("scans", "tmp1.png") || e.exists("scans", "MARIO - BIANCHI - P1234567.png") {
		t.Error("\"Salva correzioni\" non deve rinominare")
	}
	if r["doc"].(map[string]any)["file"] != "tmp1.png" {
		t.Errorf("file = %v", r["doc"].(map[string]any)["file"])
	}
}

func TestAnteprimaDelNomeFuturoSegueIlForm(t *testing.T) {
	e := newEnv(t, nil, "FAKE_GIVEN=JOSÉ MARÍA", "FAKE_SURNAME=O'NEILL-MÜLLER / Ø")
	doc := e.scanDone(scanBody)["doc"].(map[string]any)
	id := doc["doc_id"].(string)
	r := e.previewName(id, nil)
	if r["ok"] != true || r["name"] != "JOSE MARIA - O NEILL MULLER O - P1234567" || r["current"] != "tmp1" || r["same"] != false {
		t.Errorf("anteprima: %v", r)
	}
	// l'operatore corregge cognome e numero libretto: il nome cambia
	r = e.previewName(id, map[string]string{"surname": "d'Ângelo", "seamans_book": "pa 7654321"})
	if r["name"] != "JOSE MARIA - D ANGELO - PA 7654321" {
		t.Errorf("nome dopo la correzione: %v", r["name"])
	}
	// mancano pezzi: nessun nome
	if r = e.previewName(id, map[string]string{"given_names": ""}); r["ok"] != false || r["current"] != "tmp1" {
		t.Errorf("senza nome: %v", r)
	}
	if r = e.previewName(id, map[string]string{"surname": "  / ' "}); r["ok"] != false {
		t.Errorf("cognome fatto solo di simboli: %v", r)
	}
	// nessun file toccato
	if !e.exists("scans", "tmp1.png") {
		t.Error("l'anteprima non rinomina")
	}
	if code, _ := e.do("POST", "/api/preview-name", map[string]any{"doc_id": "x"}); code != 404 {
		t.Errorf("doc sconosciuto: %d", code)
	}
}

func TestConfermaRinominaConNomiConAccentiESimboli(t *testing.T) {
	// nomi sintetici con accenti, apostrofo, trattino e barra: il nome del file deve restare sicuro per Windows
	e := newEnv(t, nil, "FAKE_GIVEN=JOSÉ MARÍA", "FAKE_SURNAME=O'NEILL-MÜLLER / Ø")
	scans := filepath.Join(e.base, "scans")
	_ = os.MkdirAll(scans, 0o700)
	for _, ext := range []string{".json", ".err"} { // file gemelli dello stesso nome provvisorio
		_ = os.WriteFile(filepath.Join(scans, "tmp1"+ext), []byte("x"), 0o600)
	}
	doc := e.scanDone(scanBody)["doc"].(map[string]any)
	// il PDF prima della conferma usa il nome corrente
	if code, r := e.do("POST", "/api/pdf", map[string]any{"file": doc["file"]}); code != 200 || r["file"] != filepath.Join("scans", "tmp1.pdf") {
		t.Fatalf("pdf prima della conferma: %d %v", code, r)
	}
	code, r := e.save(doc["doc_id"].(string), "confirm", nil, []string{"capacity"}, map[string]any{"overwrite": true})
	if code != 200 || r["renamed"] != true {
		t.Fatalf("conferma: %d %v", code, r)
	}
	const base = "JOSE MARIA - O NEILL MULLER O - P1234567"
	nd := r["doc"].(map[string]any)
	if nd["file"] != base+".png" || nd["name"] != base || !regexp.MustCompile(`^[A-Za-z0-9 ()-]+$`).MatchString(base) {
		t.Fatalf("file dopo la conferma = %v", nd["file"])
	}
	found := false
	for _, m := range r["messages"].([]any) {
		if strings.Contains(m.(string), "File rinominati in «"+base+"»") {
			found = true
		}
	}
	if !found {
		t.Errorf("messaggio di rinomina assente: %v", r["messages"])
	}
	// tutto il gruppo di file ha cambiato nome (immagine, JSON, PDF, log), niente è rimasto col vecchio
	for _, ext := range []string{".png", ".json", ".pdf", ".err"} {
		if !e.exists("scans", base+ext) {
			t.Errorf("manca %s", base+ext)
		}
		if e.exists("scans", "tmp1"+ext) {
			t.Errorf("%s doveva essere rinominato", "tmp1"+ext)
		}
	}
	// il JSON salvato e la verità usano il nome definitivo
	if !e.exists("scans", "verita", base+".json") || e.exists("scans", "verita", "tmp1.json") {
		t.Error("la verità deve avere il nome definitivo")
	}
	var saved map[string]any
	jb, _ := os.ReadFile(filepath.Join(scans, base+".json"))
	if err := json.Unmarshal(jb, &saved); err != nil || saved["review"] == nil {
		t.Errorf("JSON salvato: %v", err)
	}
	// i nomi mostrati nei campi non si toccano
	if field(nd, "given_names")["value"] != "JOSÉ MARÍA" || field(nd, "surname")["value"] != "O'NEILL-MÜLLER / Ø" {
		t.Error("campi alterati")
	}
	// anteprima con il nome nuovo (spazi) e vecchio introvabile
	get := func(file string) int {
		req, _ := http.NewRequest("GET", e.srv.URL+"/api/image?file="+url.QueryEscape(file), nil)
		req.Header.Set("X-PB-Token", e.app.token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if get(base+".png") != 200 || get("tmp1.png") != 404 {
		t.Error("l'anteprima deve seguire il nuovo nome")
	}
	// PDF dopo la conferma: segue la rinomina
	if code, r := e.do("POST", "/api/pdf", map[string]any{"file": nd["file"], "overwrite": true}); code != 200 || r["file"] != filepath.Join("scans", base+".pdf") {
		t.Errorf("pdf dopo la conferma: %d %v", code, r)
	}
	// l'anteprima del nome ora dice "già così"
	if p := e.previewName(nd["doc_id"].(string), nil); p["same"] != true {
		t.Errorf("anteprima dopo la conferma: %v", p)
	}
	// cronologia con il nome nuovo
	_, l := e.do("GET", "/api/scans", nil)
	okHist := false
	for _, it := range l["items"].([]any) {
		m := it.(map[string]any)
		if m["name"] == base && m["saved"] == true && m["truth"] == true {
			okHist = true
		}
		if m["name"] == "tmp1" {
			t.Error("tmp1 ancora in cronologia")
		}
	}
	if !okHist {
		t.Error("cronologia senza il nome definitivo")
	}
}

func TestConfermaUsaIlCognomeCorretto(t *testing.T) {
	e := newEnv(t, nil)
	doc := e.scanDone(scanBody)["doc"].(map[string]any)
	code, r := e.save(doc["doc_id"].(string), "confirm", map[string]string{"surname": "Bianchi", "seamans_book": "pa0123456"}, []string{"capacity"}, nil)
	if code != 200 {
		t.Fatalf("conferma: %d %v", code, r)
	}
	if f := r["doc"].(map[string]any)["file"]; f != "MARIO - BIANCHI - PA0123456.png" {
		t.Errorf("file = %v", f)
	}
	// correggere ancora il cognome dopo la conferma: i file e la verità seguono il nuovo nome
	code, r = e.save(r["doc"].(map[string]any)["doc_id"].(string), "confirm", map[string]string{"surname": "Verdi"}, []string{"capacity"}, map[string]any{"overwrite": true})
	if code != 200 || r["doc"].(map[string]any)["file"] != "MARIO - VERDI - PA0123456.png" {
		t.Fatalf("seconda conferma: %d %v", code, r["doc"])
	}
	if e.exists("scans", "MARIO - BIANCHI - PA0123456.png") || e.exists("scans", "verita", "MARIO - BIANCHI - PA0123456.json") ||
		!e.exists("scans", "verita", "MARIO - VERDI - PA0123456.json") {
		t.Error("i file del nome precedente non devono restare")
	}
	// confermare di nuovo con gli stessi dati non cambia il nome (niente "(2)" a ogni conferma)
	code, r = e.save(r["doc"].(map[string]any)["doc_id"].(string), "confirm", nil, []string{"capacity"}, map[string]any{"overwrite": true})
	if code != 200 || r["doc"].(map[string]any)["file"] != "MARIO - VERDI - PA0123456.png" || r["renamed"] != nil {
		t.Errorf("terza conferma: %d %v", code, r["doc"].(map[string]any)["file"])
	}
}

func TestConfermaNonSovrascriveMaiUsaDue(t *testing.T) {
	e := newEnv(t, nil)
	scans := filepath.Join(e.base, "scans")
	_ = os.MkdirAll(scans, 0o700)
	// il nome definitivo esiste già (anche solo come JSON): si usa "(2)" e il vecchio non si tocca
	_ = os.WriteFile(filepath.Join(scans, "MARIO - ROSSI - P1234567.json"), []byte("vecchio"), 0o600)
	_ = os.WriteFile(filepath.Join(scans, "MARIO - ROSSI - P1234567 (2).pdf"), []byte("pdf"), 0o600)
	doc := e.scanDone(scanBody)["doc"].(map[string]any)
	if p := e.previewName(doc["doc_id"].(string), nil); p["name"] != "MARIO - ROSSI - P1234567 (3)" {
		t.Errorf("anteprima con due occupati: %v", p["name"])
	}
	code, r := e.save(doc["doc_id"].(string), "confirm", nil, []string{"capacity"}, nil)
	if code != 200 || r["doc"].(map[string]any)["file"] != "MARIO - ROSSI - P1234567 (3).png" {
		t.Fatalf("conferma: %d %v", code, r["doc"])
	}
	if b, _ := os.ReadFile(filepath.Join(scans, "MARIO - ROSSI - P1234567.json")); string(b) != "vecchio" {
		t.Error("file preesistente sovrascritto")
	}
	if !e.exists("scans", "verita", "MARIO - ROSSI - P1234567 (3).json") {
		t.Error("verità col nome definitivo mancante")
	}
}

func TestConfermaConMRZNonValidaRinominaDaiValoriControllati(t *testing.T) {
	// con MRZ non valida l'operatore ha comunque controllato i dati: la rinomina usa i valori del form
	e := newEnv(t, nil, "FAKE_EXTRACT=mrz_invalid")
	doc := e.scanDone(scanBody)["doc"].(map[string]any)
	code, r := e.save(doc["doc_id"].(string), "confirm", nil, nil, nil)
	if code != 200 || r["doc"].(map[string]any)["file"] != "MARIO - ROSSI - P1234567.png" {
		t.Errorf("%d %v", code, r["doc"])
	}
}

func TestConfermaSenzaNomeCognomeONumeroNonRinomina(t *testing.T) {
	e := newEnv(t, nil)
	doc := e.scanDone(scanBody)["doc"].(map[string]any)
	code, r := e.save(doc["doc_id"].(string), "confirm", map[string]string{"surname": ""}, []string{"capacity"}, nil)
	if code != 200 || r["renamed"] != nil || r["doc"].(map[string]any)["file"] != "tmp1.png" {
		t.Fatalf("%d %v", code, r)
	}
	said := false
	for _, m := range r["messages"].([]any) {
		if strings.Contains(m.(string), "Mancano nome, cognome o numero libretto") && strings.Contains(m.(string), "tmp1") {
			said = true
		}
	}
	if !said {
		t.Errorf("il motivo va detto: %v", r["messages"])
	}
	// verità e JSON si scrivono comunque, col nome scelto
	if !e.exists("scans", "verita", "tmp1.json") || !e.exists("scans", "tmp1.json") {
		t.Error("file di conferma mancanti")
	}
}

func TestApriImmagineSalvataEConfermaRinomina(t *testing.T) {
	// anche un'immagine aperta da scans\ si rinomina alla conferma (non prima)
	e := newEnv(t, nil)
	e.writeScan("mia.png")
	doc := e.open("mia.png")
	if doc["file"] != "mia.png" || !e.exists("scans", "mia.png") {
		t.Fatal("aprire non rinomina")
	}
	code, r := e.save(doc["doc_id"].(string), "confirm", nil, []string{"capacity"}, nil)
	if code != 200 || r["doc"].(map[string]any)["file"] != "MARIO - ROSSI - P1234567.png" {
		t.Errorf("%d %v", code, r["doc"])
	}
}

func TestConfermaRinominaConsiderandoLaVeritaEsistente(t *testing.T) {
	// la verità al nome definitivo esiste già: si chiede il permesso e non si cambia nulla prima
	e := newEnv(t, nil)
	scans := filepath.Join(e.base, "scans")
	_ = os.MkdirAll(filepath.Join(scans, "verita"), 0o700)
	_ = os.WriteFile(filepath.Join(scans, "verita", "MARIO - ROSSI - P1234567.json"), []byte("{}"), 0o600)
	doc := e.scanDone(scanBody)["doc"].(map[string]any)
	code, r := e.save(doc["doc_id"].(string), "confirm", nil, []string{"capacity"}, nil)
	if code != 409 || r["code"] != "esistono_file" {
		t.Fatalf("%d %v", code, r)
	}
	if !e.exists("scans", "tmp1.png") {
		t.Error("nulla va rinominato prima del consenso")
	}
	code, r = e.save(doc["doc_id"].(string), "confirm", nil, []string{"capacity"}, map[string]any{"overwrite": true})
	if code != 200 || r["doc"].(map[string]any)["file"] != "MARIO - ROSSI - P1234567.png" {
		t.Fatalf("con consenso: %d %v", code, r)
	}
}

func TestRenameAllTuttoONiente(t *testing.T) {
	dir := t.TempDir()
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewGray(image.Rect(0, 0, 2, 2)))
	_ = os.WriteFile(filepath.Join(dir, "old.png"), b.Bytes(), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "old.json"), []byte("{}"), 0o600)
	// il bersaglio del JSON è una cartella: la seconda rinomina fallisce e la prima va annullata
	_ = os.Mkdir(filepath.Join(dir, "new.json"), 0o700)
	if err := renameAll(dir, "old", "new"); err == nil {
		t.Fatal("doveva fallire")
	}
	for _, f := range []string{"old.png", "old.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s non rimesso a posto", f)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "new.png")); err == nil {
		t.Error("new.png non doveva restare")
	}
}

func TestFintoConNomiSpeciali(t *testing.T) {
	// il JSON del finto deve restare valido con virgolette e barre rovesciate nei nomi sintetici
	e := newEnv(t, nil, "FAKE_GIVEN=ÅSA \"X\"", "FAKE_SURNAME=D'ARC\\Z")
	e.writeScan("a.png")
	doc := e.open("a.png")
	if field(doc, "surname")["value"] != `D'ARC\Z` {
		t.Errorf("cognome: %v", field(doc, "surname")["value"])
	}
}
