package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"scanlibretto/internal/memoria"
)

// Documento come la scansione fallita dell'utente: solo l'informativa (1513x590), nessuna MRZ, nessun campo letto.
func TestDocumentoSenzaMRZETuttiICampiVuoti(t *testing.T) {
	e := newEnv(t, nil, "FAKE_EXTRACT=empty", "FAKE_SIZE=1513x590")
	j := e.scanDone(map[string]any{"name": "libretto-001", "driver": "twain", "dpi": 300, "len_mm": 97, "top_margin_mm": 2})
	doc := j["doc"].(map[string]any)
	id := doc["doc_id"].(string)
	if len(doc["fields"].([]any)) != len(fieldDefs) {
		t.Fatalf("campi = %d", len(doc["fields"].([]any)))
	}
	for _, f := range doc["fields"].([]any) {
		m := f.(map[string]any)
		if m["value"] != "" || m["status"] != "empty" || m["flag"] == true {
			t.Errorf("campo vuoto atteso: %v", m)
		}
	}
	if doc["to_check"].(float64) != 0 || doc["mrz"].(map[string]any)["found"] != false || doc["memory"].(map[string]any)["usable"] != false {
		t.Errorf("riepilogo: %v %v", doc["to_check"], doc["memory"])
	}
	codes := map[string]bool{}
	for _, w := range doc["warnings"].([]any) {
		codes[w.(map[string]any)["code"].(string)] = true
		if w.(map[string]any)["text"] == "" {
			t.Error("avviso senza testo")
		}
	}
	if !codes["MRZ_NOT_FOUND"] || !codes["PRINTED_FIELDS_NOT_READ"] {
		t.Errorf("avvisi: %v", codes)
	}
	// anteprima del nome: mancano i dati
	if p := e.previewName(id, nil); p["ok"] != false || p["current"] != "libretto-001" {
		t.Errorf("anteprima: %v", p)
	}
	// salva correzioni: niente memoria (MRZ non valida), nessun errore
	code, r := e.save(id, "corrections", map[string]string{"capacity": "AB"}, nil, nil)
	if code != 200 || r["memory_skipped"] != true {
		t.Fatalf("corrections: %d %v", code, r)
	}
	// conferma: nessuna rinomina (mancano nome, cognome, numero), si scrivono JSON e verità col nome scelto
	code, r = e.save(id, "confirm", nil, nil, nil)
	if code != 200 || r["renamed"] != nil || r["doc"].(map[string]any)["file"] != "libretto-001.png" {
		t.Fatalf("confirm: %d %v", code, r)
	}
	if !e.exists("scans", "libretto-001.json") || !e.exists("scans", "verita", "libretto-001.json") || !e.exists("scans", "libretto-001.png") {
		t.Error("file di conferma mancanti")
	}
	// con i dati scritti a mano dall'operatore si rinomina
	code, r = e.save(id, "confirm", map[string]string{"given_names": "anna", "surname": "verdi", "seamans_book": "pa0000001"}, nil, map[string]any{"overwrite": true})
	if code != 200 || r["doc"].(map[string]any)["file"] != "ANNA - VERDI - PA0000001.png" {
		t.Fatalf("confirm a mano: %d %v", code, r["doc"])
	}
	// PDF su quel documento
	if code, pr := e.do("POST", "/api/pdf", map[string]any{"file": "ANNA - VERDI - PA0000001.png"}); code != 200 {
		t.Errorf("pdf: %d %v", code, pr)
	}
	// rilettura, apertura e ricomincia
	if d2 := e.open("ANNA - VERDI - PA0000001.png"); d2["to_check"].(float64) != 0 {
		t.Errorf("rilettura: %v", d2["to_check"])
	}
	if c, _ := e.do("POST", "/api/reset", map[string]any{}); c != 200 {
		t.Errorf("reset: %d", c)
	}
	// forget-field e memoria su un documento senza chiave: errore gestito, non panic
	if c, _ := e.do("POST", "/api/memory/forget-field", map[string]any{"doc_id": id, "field": "capacity"}); c != 409 {
		t.Errorf("forget-field senza chiave: %d", c)
	}
}

// JSON dell'estrazione "strani": non devono mai far crollare il processo.
func TestEstrazioneConJSONStrani(t *testing.T) {
	cases := []string{
		`{"fields": {}}`,
		`{"fields": {"type": null}}`,
		`{"fields": {"capacity": "X"}, "mrz": {}, "warnings": [{"code": "UNCERTAIN_FIELD"}, {"code": "PRINTED_FIELDS_NOT_READ"}, {"code": ""}]}`,
		`{"fields": {}, "mrz": {"checks": {"doc_number": true, "birth": true, "expiry": true, "optional": true, "composite": true}}}`, // "valida" ma senza numero
		`{"fields": {"surname": "A"}, "warnings": [{"code": "MRZ_FIELD_UNCERTAIN", "field": "nonesiste"}, {"code": "UNCERTAIN_FIELD", "field": "zzz"}]}`,
	}
	for i, c := range cases {
		o, err := parseExtract([]byte(c))
		if err != nil {
			t.Errorf("caso %d: %v", i, err)
			continue
		}
		mem := openTestMem(t)
		d := newDoc("x", "a.png", []byte(c), o, mem)
		dv := d.view(mem, false)
		if len(dv.Fields) != len(fieldDefs) {
			t.Errorf("caso %d: campi %d", i, len(dv.Fields))
		}
		if _, err := buildSavedJSON([]byte(c), map[string]string{}, nil, nil, nil); err != nil {
			t.Errorf("caso %d: JSON salvato: %v", i, err)
		}
		_ = buildTruth(map[string]string{}, nil, []string{"capacity"})
	}
}

// Stress: molte goroutine leggono e scrivono insieme sullo stato condiviso (documenti, lavori, memoria, rinomina).
// Senza cgo non c'e' -race: se manca un lucchetto il runtime si ferma con "concurrent map read and map write".
func TestStatoCondivisoConcorrente(t *testing.T) {
	e := newEnv(t, nil)
	e.writeScan("a.png")
	doc := e.open("a.png")
	id := doc["doc_id"].(string)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var bad atomic.Int32
	hit := func(method, path string, body any) {
		code, _ := e.do(method, path, body)
		if code >= 500 {
			bad.Add(1)
		}
	}
	work := []func(i int){
		func(i int) { hit("GET", "/api/state", nil) },
		func(i int) { hit("GET", "/api/doc/"+id, nil) },
		func(i int) { hit("GET", "/api/memory", nil) },
		func(i int) { hit("GET", "/api/scans", nil) },
		func(i int) { hit("POST", "/api/heartbeat", map[string]any{}) },
		func(i int) {
			hit("POST", "/api/preview-name", map[string]any{"doc_id": id, "values": map[string]string{"surname": strings.Repeat("A", i%7+1)}})
		},
		func(i int) {
			hit("POST", "/api/save", map[string]any{"doc_id": id, "mode": "corrections", "values": map[string]string{"capacity": "V" + strings.Repeat("X", i%5)}, "reviewed": []string{"capacity"}})
		},
		func(i int) {
			hit("POST", "/api/memory/forget-field", map[string]any{"doc_id": id, "field": "capacity"})
		},
		func(i int) { hit("POST", "/api/open", map[string]string{"file": "a.png"}) },
		func(i int) {
			_, r := e.do("GET", "/api/state", nil)
			if b, _ := r["busy_job"].(string); b != "" {
				hit("GET", "/api/jobs/"+b, nil)
			}
		},
	}
	for w := range work {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				work[w](i)
			}
		}(w)
	}
	// chi usa il documento mentre si rinomina: la conferma cambia nome ai file a meta' del lavoro degli altri
	for i := 0; i < 6; i++ {
		time.Sleep(150 * time.Millisecond)
		code, _ := e.save(id, "confirm", map[string]string{"surname": "R" + strings.Repeat("O", i)}, []string{"capacity"}, map[string]any{"overwrite": true, "accept_open": true})
		if code >= 500 {
			bad.Add(1)
		}
	}
	close(stop)
	wg.Wait()
	e.app.cancelActive()
	if bad.Load() != 0 {
		t.Errorf("%d risposte 5xx", bad.Load())
	}
}

func TestPanicInUnLavoroDiventaErroreVisibile(t *testing.T) {
	e := newEnv(t, nil)
	j, err := e.app.startJob("prova", false, func(ctx context.Context, j *Job) { panic("guasto di prova") })
	if err != nil {
		t.Fatal(err)
	}
	st := e.waitJob(j.ID)
	if st["phase"] != "error" || !strings.Contains(st["error"].(string), "Errore interno") {
		t.Errorf("il panic deve diventare un errore visibile: %v", st)
	}
	// il programma e il server sono vivi e si puo' lanciare un altro lavoro
	if code, _ := e.do("GET", "/api/state", nil); code != 200 {
		t.Errorf("server non risponde dopo un panic: %d", code)
	}
	if _, err := e.app.startJob("prova2", false, func(ctx context.Context, j *Job) { j.finish(phaseDone, "ok", "", "") }); err != nil {
		t.Errorf("nuovo lavoro dopo un panic: %v", err)
	}
}

func TestPanicInUnGestoreNonChiudeIlProgramma(t *testing.T) {
	e := newEnv(t, nil)
	lg := openLog(filepath.Join(e.base, "dati", "panamabook.log"))
	e.app.lg = lg
	h := e.app.guard(panicHandler{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/state?t="+e.app.token, nil)
	req.Host = strings.TrimPrefix(e.srv.URL, "http://")
	h.ServeHTTP(rec, req)
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "Errore interno") {
		t.Errorf("risposta: %d %s", rec.Code, rec.Body.String())
	}
	lg.close()
	b, _ := os.ReadFile(filepath.Join(e.base, "dati", "panamabook.log"))
	if !strings.Contains(string(b), "PANIC nel gestore") || !strings.Contains(string(b), "guasto di prova") {
		t.Errorf("il registro deve avere il panic e lo stack: %s", b)
	}
}

func TestRegistroEventi(t *testing.T) {
	e := newEnv(t, nil)
	e.app.lg = openLog(filepath.Join(e.base, "dati", "panamabook.log"))
	defer e.app.lg.close()
	e.writeScan("a.png")
	e.open("a.png")
	e.do("POST", "/api/scan", map[string]any{"name": "../x", "driver": "twain", "dpi": 300, "len_mm": 97, "top_margin_mm": 2}) // 400, finisce nel registro
	e.do("POST", "/api/heartbeat", map[string]any{})
	e.do("GET", "/api/state", nil)
	b, _ := os.ReadFile(filepath.Join(e.base, "dati", "panamabook.log"))
	log := string(b)
	for _, want := range []string{"lavoro ", "avviato", "sottoprocesso fakelibretto.exe extract avviato", "terminato: codice 0", "finito fase done", "API POST /api/open -> 202", "API POST /api/scan -> 400", "finestra collegata"} {
		if !strings.Contains(log, want) {
			t.Errorf("manca nel registro %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "MARIO") || strings.Contains(log, "ROSSI") || strings.Contains(log, "P1234567") {
		t.Error("il registro non deve contenere valori dei campi")
	}
	if strings.Contains(log, "/api/heartbeat ->") {
		t.Error("gli heartbeat riusciti non vanno nel registro")
	}
}

type panicHandler struct{}

func (panicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { panic("guasto di prova") }

func openTestMem(t *testing.T) *memoria.Archivio {
	t.Helper()
	m, err := memoria.Apri(filepath.Join(t.TempDir(), "m.json"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}
