package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"scanlibretto/internal/verify"
)

// fakeExe e' il finto libretto.exe (cmd/panamabook/fakelibretto) compilato una volta per tutti i test. I test NON usano
// mai il vero libretto.exe: nessuno scanner viene toccato, i dati sono sintetici.
var fakeExe string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pbfake")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakeExe = filepath.Join(dir, "fakelibretto.exe")
	cmd := exec.Command("go", "build", "-o", fakeExe, "./fakelibretto")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, "compilazione del finto libretto:", err, string(out))
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type env struct {
	t    *testing.T
	app  *App
	srv  *httptest.Server
	base string
}

// newEnv avvia l'app su una cartella temporanea con il finto libretto. extra sono variabili d'ambiente del finto (FAKE_*).
func newEnv(t *testing.T, mut func(*config), extra ...string) *env {
	t.Helper()
	base := t.TempDir()
	cfg := config{Base: base, Exe: fakeExe, ExtraEnv: extra, NoWindow: true, PDFFallback: filepath.Join(base, "_out", "libretto_pdf.exe")}
	if mut != nil {
		mut(&cfg)
	}
	app, err := newApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	app.demoDelay = 400 * time.Millisecond
	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	app.setAddr(u.Port())
	return &env{t: t, app: app, srv: srv, base: base}
}

func (e *env) do(method, path string, body any) (int, map[string]any) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	req.Header.Set("X-PB-Token", e.app.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

// waitJob attende la fine del lavoro e ne restituisce lo stato.
func (e *env) waitJob(id string) map[string]any {
	e.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		code, j := e.do("GET", "/api/jobs/"+id, nil)
		if code != 200 {
			e.t.Fatalf("job: codice %d", code)
		}
		if j["done"] == true {
			return j
		}
		time.Sleep(30 * time.Millisecond)
	}
	e.t.Fatal("lavoro non finito in tempo")
	return nil
}

func (e *env) writeScan(name string) string {
	e.t.Helper()
	dir := filepath.Join(e.base, "scans")
	_ = os.MkdirAll(dir, 0o700)
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	for x := 0; x < 20; x++ {
		img.Set(x, 3, color.RGBA{A: 255})
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		e.t.Fatal(err)
	}
	return p
}

// open apre un'immagine di scans e attende la lettura; restituisce il documento.
func (e *env) open(file string) map[string]any {
	e.t.Helper()
	code, r := e.do("POST", "/api/open", map[string]string{"file": file})
	if code != 202 {
		e.t.Fatalf("open: codice %d %v", code, r)
	}
	j := e.waitJob(r["job"].(string))
	if j["phase"] != "done" {
		e.t.Fatalf("lettura non riuscita: %v", j)
	}
	return j["doc"].(map[string]any)
}

func field(doc map[string]any, key string) map[string]any {
	for _, f := range doc["fields"].([]any) {
		m := f.(map[string]any)
		if m["key"] == key {
			return m
		}
	}
	return nil
}

func TestSicurezzaTokenHostOrigin(t *testing.T) {
	e := newEnv(t, nil)
	get := func(path string, mut func(*http.Request)) int {
		req, _ := http.NewRequest("GET", e.srv.URL+path, nil)
		if mut != nil {
			mut(req)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := get("/api/state", nil); c != 401 {
		t.Errorf("senza token: %d", c)
	}
	if c := get("/api/state", func(r *http.Request) { r.Header.Set("X-PB-Token", "sbagliato") }); c != 401 {
		t.Errorf("token errato: %d", c)
	}
	if c := get("/api/state", func(r *http.Request) { r.Header.Set("X-PB-Token", e.app.token) }); c != 200 {
		t.Errorf("token giusto: %d", c)
	}
	if c := get("/api/state?t="+e.app.token, nil); c != 200 {
		t.Errorf("token in query: %d", c)
	}
	if c := get("/api/state?t="+e.app.token, func(r *http.Request) { r.Host = "evil.example:80" }); c != 403 {
		t.Errorf("host estraneo: %d", c)
	}
	if c := get("/api/state?t="+e.app.token, func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") }); c != 403 {
		t.Errorf("origin estraneo: %d", c)
	}
	if c := get("/api/state?t="+e.app.token, func(r *http.Request) { r.Header.Set("Origin", "http://127.0.0.1:1") }); c != 403 {
		t.Errorf("origin con porta sbagliata: %d", c)
	}
	if c := get("/api/state?t="+e.app.token, func(r *http.Request) { r.Header.Set("Origin", e.srv.URL) }); c != 200 {
		t.Errorf("origin giusto: %d", c)
	}
	if c := get("/api/state?t="+e.app.token, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }); c != 403 {
		t.Errorf("cross-site: %d", c)
	}
	// la pagina statica non richiede il token (non contiene nulla di segreto) ma controlla l'host
	if c := get("/", nil); c != 200 {
		t.Errorf("pagina: %d", c)
	}
	if c := get("/", func(r *http.Request) { r.Host = "evil.example" }); c != 403 {
		t.Errorf("pagina con host estraneo: %d", c)
	}
	// POST senza token
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/heartbeat", strings.NewReader("{}"))
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 401 {
		t.Errorf("POST senza token: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestScansioneConFintoScanner(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args.txt")
	e := newEnv(t, nil, "FAKE_ARGS="+args)
	code, r := e.do("POST", "/api/scan", map[string]any{"name": "prova1", "driver": "twain", "dpi": 300, "len_mm": 97, "top_margin_mm": 2})
	if code != 202 {
		t.Fatalf("scan: %d %v", code, r)
	}
	j := e.waitJob(r["job"].(string))
	if j["phase"] != "done" {
		t.Fatalf("job: %v", j)
	}
	// dopo la lettura i file restano col nome scelto: la rinomina avviene solo con "Conferma e salva"
	doc := j["doc"].(map[string]any)
	if doc["file"] != "prova1.png" {
		t.Errorf("file = %v", doc["file"])
	}
	if _, err := os.Stat(filepath.Join(e.base, "scans", "prova1.png")); err != nil {
		t.Errorf("scansione non scritta: %v", err)
	}
	if n := len(doc["fields"].([]any)); n != len(fieldDefs) {
		t.Errorf("campi = %d, attesi %d", n, len(fieldDefs))
	}
	b, _ := os.ReadFile(args)
	got := string(b)
	want := "-yes -o " + filepath.Join("scans", "prova1.png") + " -driver twain -dpi 300 -len 97 -from 0 -rotate 180 -top-margin 2"
	if !strings.Contains(got, want) {
		t.Errorf("argomenti dello scan:\n%s\nvoluti: %s", got, want)
	}
	if !strings.Contains(got, "extract -i "+filepath.Join("scans", "prova1.png")) {
		t.Errorf("manca l'estrazione: %s", got)
	}
	// stesso nome: rifiutato
	if code, _ := e.do("POST", "/api/scan", map[string]any{"name": "prova1", "driver": "twain", "dpi": 300, "len_mm": 97, "top_margin_mm": 2}); code != 409 {
		t.Errorf("nome duplicato: %d", code)
	}
}

func TestImpostazioniNonValide(t *testing.T) {
	e := newEnv(t, nil)
	ok := map[string]any{"name": "a", "driver": "twain", "dpi": 300, "len_mm": 97, "top_margin_mm": 2}
	cases := map[string]func(m map[string]any){
		"nome con ..":      func(m map[string]any) { m["name"] = "a..b" },
		"nome con barra":   func(m map[string]any) { m["name"] = "../x" },
		"nome con spazio":  func(m map[string]any) { m["name"] = "a b" },
		"nome vuoto":       func(m map[string]any) { m["name"] = "" },
		"driver ignoto":    func(m map[string]any) { m["driver"] = "x; calc" },
		"dpi enorme":       func(m map[string]any) { m["dpi"] = 5000 },
		"lunghezza minima": func(m map[string]any) { m["len_mm"] = 5 },
		"margine enorme":   func(m map[string]any) { m["top_margin_mm"] = 99 },
	}
	for name, mut := range cases {
		m := map[string]any{}
		for k, v := range ok {
			m[k] = v
		}
		mut(m)
		if code, _ := e.do("POST", "/api/scan", m); code != 400 {
			t.Errorf("%s: codice %d, atteso 400", name, code)
		}
	}
	if _, err := os.Stat(filepath.Join(e.base, "scans")); err == nil {
		ents, _ := os.ReadDir(filepath.Join(e.base, "scans"))
		if len(ents) != 0 {
			t.Errorf("file creati con impostazioni non valide: %v", ents)
		}
	}
}

func TestErroriDelloScanner(t *testing.T) {
	for _, c := range []struct{ mode, want string }{{"paper", "Nessun libretto"}, {"jam", "Inceppamento"}} {
		e := newEnv(t, nil, "FAKE_SCAN="+c.mode)
		_, r := e.do("POST", "/api/scan", map[string]any{"name": "x", "driver": "twain", "dpi": 300, "len_mm": 97, "top_margin_mm": 2})
		j := e.waitJob(r["job"].(string))
		if j["phase"] != "error" || !strings.Contains(j["error"].(string), c.want) {
			t.Errorf("%s: %v", c.mode, j)
		}
	}
}

func TestDemoNonUsaLoScanner(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args.txt")
	src := filepath.Join(t.TempDir(), "demo.png")
	b := bytes.NewBuffer(nil)
	_ = png.Encode(b, image.NewGray(image.Rect(0, 0, 8, 8)))
	_ = os.WriteFile(src, b.Bytes(), 0o600)
	e := newEnv(t, func(c *config) { c.Demo = src }, "FAKE_ARGS="+args)
	_, st := e.do("GET", "/api/state", nil)
	if st["demo"] != true {
		t.Errorf("stato: %v", st)
	}
	_, r := e.do("POST", "/api/scan", map[string]any{"name": "d1", "driver": "twain", "dpi": 300, "len_mm": 97, "top_margin_mm": 2})

	// durante la scansione (anche finta) niente annulla
	time.Sleep(100 * time.Millisecond)
	_, jv := e.do("GET", "/api/jobs/"+r["job"].(string), nil)
	if jv["phase"] != "scanning" || jv["cancelable"] != false || jv["demo"] != true {
		t.Errorf("durante la scansione: %v", jv)
	}
	if code, _ := e.do("POST", "/api/jobs/"+r["job"].(string)+"/cancel", map[string]any{}); code != 409 {
		t.Errorf("annulla durante la scansione: %d (atteso 409)", code)
	}
	// un secondo lavoro e' rifiutato
	if code, _ := e.do("POST", "/api/scan", map[string]any{"name": "d2", "driver": "twain", "dpi": 300, "len_mm": 97, "top_margin_mm": 2}); code != 409 {
		t.Errorf("secondo lavoro: %d", code)
	}
	j := e.waitJob(r["job"].(string))
	if j["phase"] != "done" {
		t.Fatalf("demo: %v", j)
	}
	got, _ := os.ReadFile(filepath.Join(e.base, "scans", j["doc"].(map[string]any)["file"].(string)))
	if !bytes.Equal(got, b.Bytes()) {
		t.Error("l'immagine demo non e' stata copiata")
	}
	a, _ := os.ReadFile(args)
	if strings.Contains(string(a), "scan ") || !strings.Contains(string(a), "extract") {
		t.Errorf("in demo non si deve lanciare scan: %q", a)
	}
}

func TestAnnullaLaLettura(t *testing.T) {
	e := newEnv(t, nil, "FAKE_EXTRACT=slow")
	e.writeScan("lenta.png")
	_, r := e.do("POST", "/api/open", map[string]string{"file": "lenta.png"})
	id := r["job"].(string)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, j := e.do("GET", "/api/jobs/"+id, nil)
		if j["phase"] == "reading" && j["cancelable"] == true {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("la lettura non e' partita: %v", j)
		}
		time.Sleep(20 * time.Millisecond)
	}
	start := time.Now()
	if code, _ := e.do("POST", "/api/jobs/"+id+"/cancel", map[string]any{}); code != 200 {
		t.Fatalf("annulla: %d", code)
	}
	j := e.waitJob(id)
	if j["phase"] != "cancelled" {
		t.Errorf("fase = %v", j["phase"])
	}
	if time.Since(start) > 10*time.Second {
		t.Error("il sottoprocesso non e' stato terminato")
	}
	// dopo l'annullamento si puo' partire di nuovo
	if code, _ := e.do("POST", "/api/open", map[string]string{"file": "lenta.png"}); code != 202 {
		t.Errorf("nuovo lavoro dopo annullamento: %d", code)
	}
	e.app.cancelActive()
}

func TestEstrazioneFallita(t *testing.T) {
	e := newEnv(t, nil, "FAKE_EXTRACT=fail")
	e.writeScan("a.png")
	_, r := e.do("POST", "/api/open", map[string]string{"file": "a.png"})
	j := e.waitJob(r["job"].(string))
	if j["phase"] != "error" || !strings.Contains(j["error"].(string), "OCR non disponibile") {
		t.Errorf("job: %v", j)
	}
}

func TestImmagineSoloDaScans(t *testing.T) {
	e := newEnv(t, nil)
	e.writeScan("ok.png")
	_ = os.WriteFile(filepath.Join(e.base, "segreto.png"), []byte("x"), 0o600)
	_ = os.MkdirAll(filepath.Join(e.base, "scans", "sub"), 0o700)
	_ = os.WriteFile(filepath.Join(e.base, "scans", "sub", "b.png"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(e.base, "scans", "nota.txt"), []byte("x"), 0o600)
	for _, f := range []string{"../segreto.png", "..\\segreto.png", "sub/b.png", "sub\\b.png", "nota.txt", "", "ok.png/", "C:\\x.png", ".png"} {
		code, _ := e.do("GET", "/api/image?file="+url.QueryEscape(f), nil)
		if code != 404 {
			t.Errorf("file %q: %d, atteso 404", f, code)
		}
		if code, _ := e.do("POST", "/api/open", map[string]string{"file": f}); code != 404 {
			t.Errorf("open %q: %d, atteso 404", f, code)
		}
	}
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/image?file=ok.png", nil)
	req.Header.Set("X-PB-Token", e.app.token)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" {
		t.Errorf("immagine valida: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	resp.Body.Close()
	_, l := e.do("GET", "/api/scans", nil)
	if items := l["items"].([]any); len(items) != 1 {
		t.Errorf("elenco: %v", items)
	}
}

func TestMemoriaImparaEApplica(t *testing.T) {
	e := newEnv(t, nil)
	e.writeScan("a.png")
	doc := e.open("a.png")
	cap1 := field(doc, "capacity")
	if cap1["value"] != "ORDINARY SEAMEN" || cap1["status"] != "check" {
		t.Fatalf("capacity iniziale: %v", cap1)
	}
	if !doc["memory"].(map[string]any)["usable"].(bool) {
		t.Fatal("la memoria dovrebbe essere usabile con MRZ valida")
	}
	if st := field(doc, "type")["status"]; st != "ok" {
		t.Errorf("type: %v", st)
	}
	if st := field(doc, "certificate")["status"]; st != "empty" {
		t.Errorf("certificate assente deve essere 'empty': %v", st)
	}
	// l'operatore corregge e salva
	code, sv := e.do("POST", "/api/save", map[string]any{"doc_id": doc["doc_id"], "mode": "corrections", "values": map[string]string{"capacity": "ordinary  seaman"}})
	if code != 200 || sv["memory_saved"].(float64) != 1 {
		t.Fatalf("save: %d %v", code, sv)
	}
	if st := field(sv["doc"].(map[string]any), "capacity"); st["value"] != "ORDINARY SEAMAN" || st["status"] != "memory" {
		t.Errorf("dopo il salvataggio: %v", st)
	}
	if fi, err := os.Stat(filepath.Join(e.base, "dati", "memoria.json")); err != nil {
		t.Fatalf("memoria non scritta: %v", err)
	} else if fi.Size() == 0 {
		t.Error("memoria vuota")
	}
	// "corrections" non scrive ne' verita' ne' JSON
	if _, err := os.Stat(filepath.Join(e.base, "scans", "verita")); err == nil {
		t.Error("salva correzioni non deve scrivere la verita'")
	}

	// la scansione successiva dello stesso libretto applica la memoria
	doc2 := e.open("a.png")
	c2 := field(doc2, "capacity")
	if c2["value"] != "ORDINARY SEAMAN" || c2["status"] != "memory" || c2["mem_read"] != "ORDINARY SEAMEN" || c2["read"] != "ORDINARY SEAMEN" {
		t.Errorf("memoria non applicata: %v", c2)
	}
	if m := doc2["memory"].(map[string]any); m["applied"].(float64) != 1 || m["known"].(float64) != 1 {
		t.Errorf("riepilogo memoria: %v", m)
	}
	if field(doc2, "level")["status"] != "ok" {
		t.Error("un campo non corretto non deve cambiare")
	}

	// persistenza: una nuova istanza sullo stesso disco ricorda
	app2, err := newApp(config{Base: e.base, Exe: fakeExe})
	if err != nil || len(app2.mem.Elenco()) != 1 {
		t.Fatalf("nuova istanza: %v", err)
	}

	// "questo era giusto, dimentica"
	code, fg := e.do("POST", "/api/memory/forget-field", map[string]any{"doc_id": doc2["doc_id"], "field": "capacity"})
	if code != 200 {
		t.Fatalf("forget-field: %d %v", code, fg)
	}
	if f := field(fg, "capacity"); f["value"] != "ORDINARY SEAMEN" || f["status"] == "memory" {
		t.Errorf("dopo forget: %v", f)
	}
	doc3 := e.open("a.png")
	if field(doc3, "capacity")["status"] == "memory" {
		t.Error("la memoria dimenticata e' stata riapplicata")
	}
	_, ml := e.do("GET", "/api/memory", nil)
	if len(ml["items"].([]any)) != 0 {
		t.Errorf("memoria non vuota: %v", ml)
	}
}

func TestMemoriaNonSiApplicaSenzaMRZValida(t *testing.T) {
	// prima si impara con MRZ valida, poi la stessa lettura con un check digit sbagliato non deve usare la memoria
	e := newEnv(t, nil)
	e.writeScan("a.png")
	doc := e.open("a.png")
	e.do("POST", "/api/save", map[string]any{"doc_id": doc["doc_id"], "mode": "corrections", "values": map[string]string{"capacity": "ORDINARY SEAMAN"}})

	bad := newEnvOn(t, e.base, "FAKE_EXTRACT=mrz_invalid")
	d2 := bad.open("a.png")
	m := d2["memory"].(map[string]any)
	if m["usable"].(bool) || m["reason"] == "" || m["applied"].(float64) != 0 {
		t.Errorf("memoria con MRZ non valida: %v", m)
	}
	if field(d2, "capacity")["value"] != "ORDINARY SEAMEN" {
		t.Error("memoria applicata con MRZ non valida")
	}
	// e non si salva
	_, sv := bad.do("POST", "/api/save", map[string]any{"doc_id": d2["doc_id"], "mode": "corrections", "values": map[string]string{"level": "OPERATIONAL"}})
	if sv["memory_skipped"] != true || sv["memory_saved"].(float64) != 0 {
		t.Errorf("salvataggio con MRZ non valida: %v", sv)
	}
	if n := len(bad.app.mem.Elenco()); n != 1 {
		t.Errorf("libretti in memoria = %d, atteso 1 (quello di prima)", n)
	}
	for _, w := range d2["warnings"].([]any) {
		if w.(map[string]any)["code"] == "MRZ_CHECK_DIGITS_INVALID" && w.(map[string]any)["severity"] != "error" {
			t.Error("MRZ_CHECK_DIGITS_INVALID deve essere un errore")
		}
	}

	nomrz := newEnvOn(t, e.base, "FAKE_EXTRACT=no_mrz")
	d3 := nomrz.open("a.png")
	if d3["memory"].(map[string]any)["usable"].(bool) || d3["mrz"].(map[string]any)["found"].(bool) {
		t.Errorf("senza MRZ: %v", d3["memory"])
	}
}

// newEnvOn avvia un'altra app sulla stessa cartella di lavoro (stessa memoria su disco) con un altro finto.
func newEnvOn(t *testing.T, base string, extra ...string) *env {
	t.Helper()
	e := newEnv(t, func(c *config) { c.Base = base }, extra...)
	return e
}

func TestConfermaScriveVeritaEJSON(t *testing.T) {
	e := newEnv(t, nil, "FAKE_EXTRACT=legacy")
	e.writeScan("c1.png")
	doc := e.open("c1.png")
	// alias in lettura: endorsement -> certificate
	if f := field(doc, "certificate"); f["value"] != "123456789" {
		t.Fatalf("alias endorsement non letto: %v", f)
	}
	body := map[string]any{"doc_id": doc["doc_id"], "mode": "confirm", "values": map[string]string{"capacity": "ORDINARY SEAMAN", "date_of_issue": "02-02-2026"}}
	code, sv := e.do("POST", "/api/save", body)
	if code != 200 {
		t.Fatalf("confirm: %d %v", code, sv)
	}
	// JSON salvato
	b, err := os.ReadFile(filepath.Join(e.base, "scans", "MARIO - ROSSI - P1234567.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	fields := saved["fields"].(map[string]any)
	if fields["capacity"] != "ORDINARY SEAMAN" || fields["certificate"] != "123456789" || fields["endorsement"] != nil {
		t.Errorf("campi salvati: %v", fields)
	}
	if fields["date_of_issue_iso"] != "2026-02-02" {
		t.Errorf("data iso non aggiornata: %v", fields["date_of_issue_iso"])
	}
	cs := saved["corrections"].([]any)
	if len(cs) != 2 {
		t.Fatalf("corrections = %v", cs)
	}
	for _, c := range cs {
		if c.(map[string]any)["source"] != "user" {
			t.Errorf("fonte: %v", c)
		}
	}
	if saved["mrz"] == nil || saved["warnings"] == nil {
		t.Error("il JSON salvato deve conservare mrz e warnings")
	}
	// verita' leggibile dal verificatore
	tb, err := os.ReadFile(filepath.Join(e.base, "scans", "verita", "MARIO - ROSSI - P1234567.json"))
	if err != nil {
		t.Fatal(err)
	}
	tr, err := verify.ParseTruth(tb)
	if err != nil {
		t.Fatalf("verita' non valida: %v", err)
	}
	if tr.Fields["capacity"] != "ORDINARY SEAMAN" || tr.Fields["certificate"] != "123456789" || tr.Fields["date_of_issue"] != "02-02-2026" || tr.Fields["type"] != "SI" {
		t.Errorf("verita': %v", tr.Fields)
	}
	if len(tr.Absent) != 0 {
		t.Errorf("chiavi mancanti nella verita': %v", tr.Absent)
	}
	if tr.NameRow != "MARIO ROSSI" {
		t.Errorf("riga nome = %q", tr.NameRow)
	}
	if tr.Notes["capacity"] == "" {
		t.Error("manca la nota sul campo corretto")
	}

	// seconda conferma: i file esistono, serve il consenso
	code, sv = e.do("POST", "/api/save", body)
	if code != 409 || sv["code"] != "esistono_file" || len(sv["existing"].([]any)) != 2 {
		t.Fatalf("senza consenso: %d %v", code, sv)
	}
	if after, _ := os.ReadFile(filepath.Join(e.base, "scans", "verita", "MARIO - ROSSI - P1234567.json")); !bytes.Equal(after, tb) {
		t.Error("la verita' e' stata toccata senza consenso")
	}
	body["overwrite"] = true
	if code, sv = e.do("POST", "/api/save", body); code != 200 {
		t.Fatalf("con consenso: %d %v", code, sv)
	}
	if sv["truth_backup"] == nil {
		t.Fatal("manca la copia della verita' precedente")
	}
	if prev, err := os.ReadFile(filepath.Join(e.base, filepath.FromSlash(strings.ReplaceAll(sv["truth_backup"].(string), "\\", "/")))); err != nil || !bytes.Equal(prev, tb) {
		t.Errorf("copia della verita' precedente: %v", err)
	}
}

func TestSalvaConValoriStrani(t *testing.T) {
	e := newEnv(t, nil)
	e.writeScan("a.png")
	doc := e.open("a.png")
	long := strings.Repeat("x", 500)
	code, sv := e.do("POST", "/api/save", map[string]any{"doc_id": doc["doc_id"], "mode": "corrections", "values": map[string]string{"capacity": long, "campo_inventato": "1"}})
	if code != 200 {
		t.Fatalf("%d %v", code, sv)
	}
	if v := field(sv["doc"].(map[string]any), "capacity")["value"].(string); len(v) != 120 {
		t.Errorf("valore non limitato: %d", len(v))
	}
	if c, _ := e.do("POST", "/api/save", map[string]any{"doc_id": "x", "mode": "confirm"}); c != 404 {
		t.Errorf("doc sconosciuto: %d", c)
	}
	if c, _ := e.do("POST", "/api/save", map[string]any{"doc_id": doc["doc_id"], "mode": "boh"}); c != 400 {
		t.Errorf("modo ignoto: %d", c)
	}
	if _, err := os.Stat(filepath.Join(e.base, "scans", "a.json")); err == nil {
		t.Error("corrections non deve scrivere il JSON")
	}
}

func TestEsportaPDF(t *testing.T) {
	e := newEnv(t, nil)
	e.writeScan("p1.png")
	code, r := e.do("POST", "/api/pdf", map[string]any{"file": "p1.png"})
	if code != 200 || r["limit_ok"] != true || r["file"] != filepath.Join("scans", "p1.pdf") {
		t.Fatalf("pdf: %d %v", code, r)
	}
	if _, err := os.Stat(filepath.Join(e.base, "scans", "p1.pdf")); err != nil {
		t.Error("PDF non scritto")
	}
	// non si sovrascrive senza consenso
	code, r = e.do("POST", "/api/pdf", map[string]any{"file": "p1.png"})
	if code != 409 || r["code"] != "esiste" {
		t.Errorf("senza consenso: %d %v", code, r)
	}
	if code, _ = e.do("POST", "/api/pdf", map[string]any{"file": "p1.png", "overwrite": true}); code != 200 {
		t.Errorf("con consenso: %d", code)
	}
	if code, _ = e.do("POST", "/api/pdf", map[string]any{"file": "../x.png"}); code != 404 {
		t.Errorf("percorso: %d", code)
	}
}

func TestPDFTroppoGrandeEFallback(t *testing.T) {
	e := newEnv(t, nil, "FAKE_PDF_SIZE=1100000")
	e.writeScan("g.png")
	_, r := e.do("POST", "/api/pdf", map[string]any{"file": "g.png"})
	if r["limit_ok"] != false {
		t.Errorf("un PDF oltre 1 MB deve essere segnalato: %v", r)
	}

	// libretto.exe vecchio (senza 'libretto pdf'): ripiego su _out\libretto_pdf.exe
	old := filepath.Join(t.TempDir(), "libretto_nopdf.exe")
	b, _ := os.ReadFile(fakeExe)
	_ = os.WriteFile(old, b, 0o755)
	e2 := newEnv(t, func(c *config) { c.Exe = old })
	fb := filepath.Join(e2.base, "_out", "libretto_pdf.exe")
	_ = os.MkdirAll(filepath.Dir(fb), 0o755)
	e2.writeScan("g.png")
	if code, r := e2.do("POST", "/api/pdf", map[string]any{"file": "g.png"}); code != 424 {
		t.Errorf("senza ripiego: %d %v", code, r)
	}
	_ = os.WriteFile(fb, b, 0o755)
	if code, r := e2.do("POST", "/api/pdf", map[string]any{"file": "g.png"}); code != 200 {
		t.Errorf("con ripiego: %d %v", code, r)
	}
	if e2.app.pdfPath != fb {
		t.Errorf("eseguibile scelto: %s", e2.app.pdfPath)
	}
}

func TestUscitaEHeartbeat(t *testing.T) {
	e := newEnv(t, nil)
	start := time.Now()
	exit := func(at time.Time) bool { x, _ := e.app.shouldExit(at, start); return x }
	if exit(start.Add(time.Minute)) {
		t.Error("non deve uscire: la finestra ha ancora tempo per collegarsi")
	}
	if x, why := e.app.shouldExit(start.Add(4*time.Minute), start); !x || why == "" {
		t.Errorf("deve uscire (e dire perche'): la finestra non si e' mai collegata: %v %q", x, why)
	}
	e.do("POST", "/api/heartbeat", map[string]any{})
	now := time.Now()
	if exit(now.Add(50 * time.Second)) {
		t.Error("mai uscire con un heartbeat negli ultimi 60 s")
	}
	if exit(now.Add(170 * time.Second)) {
		t.Error("una pagina in secondo piano rallenta i timer: 170 s senza heartbeat non bastano")
	}
	if x, why := e.app.shouldExit(now.Add(190*time.Second), start); !x || !strings.Contains(why, "heartbeat assente") {
		t.Errorf("heartbeat scaduto: %v %q", x, why)
	}

	// senza un Edge nostro "bye" accelera l'uscita, ma solo se dopo 15 s non e' tornato nessun heartbeat
	e.do("POST", "/api/bye", map[string]any{})
	if exit(time.Now().Add(10 * time.Second)) {
		t.Error("troppo presto dopo bye (si ricarica la pagina)")
	}
	if x, why := e.app.shouldExit(time.Now().Add(20*time.Second), start); !x || !strings.Contains(why, "bye") {
		t.Errorf("dopo bye senza heartbeat nuovo deve uscire: %v %q", x, why)
	}
	e.do("POST", "/api/heartbeat", map[string]any{})
	if exit(time.Now().Add(20 * time.Second)) {
		t.Error("dopo un ricaricamento (heartbeat nuovo) non deve uscire")
	}

	// con un Edge lanciato da noi la chiusura e' la fine del suo processo: "bye" non conta
	e.app.haveEdge.Store(true)
	e.do("POST", "/api/bye", map[string]any{})
	if exit(time.Now().Add(20 * time.Second)) {
		t.Error("con Edge nostro bye non deve spegnere il server")
	}
}

func TestNonEsceDuranteLaScansione(t *testing.T) {
	src := filepath.Join(t.TempDir(), "demo.png")
	b := bytes.NewBuffer(nil)
	_ = png.Encode(b, image.NewGray(image.Rect(0, 0, 4, 4)))
	_ = os.WriteFile(src, b.Bytes(), 0o600)
	e := newEnv(t, func(c *config) { c.Demo = src })
	_, r := e.do("POST", "/api/scan", map[string]any{"name": "z", "driver": "twain", "dpi": 300, "len_mm": 97, "top_margin_mm": 2})
	time.Sleep(80 * time.Millisecond)
	if !e.app.scanningNow() {
		t.Fatal("la scansione dovrebbe essere in corso")
	}
	if x, _ := e.app.shouldExit(time.Now().Add(time.Hour), time.Now().Add(-time.Hour)); x {
		t.Error("non deve uscire con un lavoro in corso")
	}
	e.waitJob(r["job"].(string))
}
