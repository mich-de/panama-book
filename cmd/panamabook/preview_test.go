package main

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var prevBody = map[string]any{"driver": "twain", "dpi": 300, "len_mm": 97, "top_margin_mm": 2}

// previewDone fa partire "Anteprima" e attende la fine; restituisce il lavoro finito.
func (e *env) previewDone() map[string]any {
	e.t.Helper()
	code, r := e.do("POST", "/api/preview", prevBody)
	if code != 202 {
		e.t.Fatalf("preview: %d %v", code, r)
	}
	j := e.waitJob(r["job"].(string))
	if j["phase"] != "done" {
		e.t.Fatalf("anteprima non riuscita: %v", j)
	}
	return j
}

func (e *env) scansEntries() []string {
	ents, _ := os.ReadDir(filepath.Join(e.base, "scans"))
	var out []string
	for _, x := range ents {
		out = append(out, x.Name())
	}
	return out
}

func (e *env) readPreview(file string) map[string]any {
	e.t.Helper()
	code, r := e.do("POST", "/api/open", map[string]string{"file": file, "dir": "preview"})
	if code != 202 {
		e.t.Fatalf("open preview: %d %v", code, r)
	}
	j := e.waitJob(r["job"].(string))
	if j["phase"] != "done" {
		e.t.Fatalf("lettura non riuscita: %v", j)
	}
	return j["doc"].(map[string]any)
}

func TestAnteprimaNonCreaFileInScansNeLeggeNulla(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args.txt")
	e := newEnv(t, nil, "FAKE_ARGS="+args)
	j := e.previewDone()
	pv := j["preview"].(map[string]any)
	if pv["file"] != "anteprima-001.png" || j["doc"] != nil {
		t.Fatalf("anteprima: %v", j)
	}
	if !e.exists("dati", "anteprima", "anteprima-001.png") {
		t.Error("l'anteprima deve stare in dati\\anteprima")
	}
	if len(e.scansEntries()) != 0 || e.exists("scans") {
		t.Errorf("niente in scans dopo Anteprima: %v", e.scansEntries())
	}
	b, _ := os.ReadFile(args)
	got := string(b)
	want := "scan -yes -o " + filepath.Join("dati", "anteprima", "anteprima-001.png") + " -driver twain -dpi 300 -len 97 -from 0 -rotate 180 -top-margin 2"
	if !strings.Contains(got, want) {
		t.Errorf("argomenti dello scan:\n%s\nvoluti: %s", got, want)
	}
	// a fine scansione parte da sola la lettura con la sola pagina dati; JSON in cache, niente in scans
	if !strings.Contains(got, "extract -i "+filepath.Join("dati", "anteprima", "anteprima-001.png")+" -page-out "+filepath.Join("dati", "anteprima", "anteprima-001-pagina.png")+" -top-margin 2 -force") {
		t.Errorf("manca la lettura automatica: %s", got)
	}
	if !e.exists("dati", "anteprima", "anteprima-001-lettura.json") || !e.exists("dati", "anteprima", "anteprima-001-pagina.png") || pv["has_page"] != true {
		t.Error("lettura in cache e pagina dati attese")
	}
	// l'immagine si vede (dir=preview) e non e' raggiungibile come file di scans
	get := func(path string) int {
		req, _ := http.NewRequest("GET", e.srv.URL+path, nil)
		req.Header.Set("X-PB-Token", e.app.token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if get("/api/image?dir=preview&file=anteprima-001.png") != 200 || get("/api/image?file=anteprima-001.png") != 404 {
		t.Error("immagine dell'anteprima")
	}
	// leggere i dati: ancora niente in scans, nessun JSON
	doc := e.readPreview("anteprima-001.png")
	if doc["dir"] != "preview" || doc["unsaved"] != true || doc["file"] != "anteprima-001.png" || !strings.Contains(doc["image_url"].(string), "dir=preview") {
		t.Errorf("documento: %v", doc)
	}
	if len(e.scansEntries()) != 0 {
		t.Errorf("niente in scans dopo Leggi: %v", e.scansEntries())
	}
	b, _ = os.ReadFile(args)
	if n := strings.Count(string(b), "extract -i"); n != 1 {
		t.Errorf("\"Leggi i dati\" deve usare la cache (una sola estrazione, a fine scansione): %d\n%s", n, b)
	}
	// salva correzioni e anteprima del nome: ancora niente in scans
	id := doc["doc_id"].(string)
	if c, r := e.save(id, "corrections", map[string]string{"capacity": "AB"}, []string{"capacity"}, nil); c != 200 {
		t.Fatalf("corrections: %d %v", c, r)
	}
	if p := e.previewName(id, nil); p["name"] != "MARIO - ROSSI - P1234567" || p["unsaved"] != true {
		t.Errorf("nome futuro: %v", p)
	}
	if e.exists("scans") {
		t.Error("scans non deve esistere prima della conferma")
	}
	// la striscia mostra l'anteprima, con il documento letto
	_, l := e.do("GET", "/api/previews", nil)
	items := l["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["doc_id"] != id || items[0].(map[string]any)["low"] != false {
		t.Errorf("striscia: %v", items)
	}
}

func TestSoloLaConfermaSalvaInScans(t *testing.T) {
	e := newEnv(t, nil)
	e.previewDone()
	doc := e.readPreview("anteprima-001.png")
	id := doc["doc_id"].(string)
	// PDF prima del salvataggio: in dati\anteprima, non in scans
	code, pr := e.do("POST", "/api/pdf", map[string]any{"file": "anteprima-001.png", "dir": "preview"})
	if code != 200 || pr["file"] != filepath.Join("dati", "anteprima", "anteprima-001.pdf") || pr["preview"] != true {
		t.Fatalf("pdf anteprima: %d %v", code, pr)
	}
	if !e.exists("dati", "anteprima", "anteprima-001.pdf") || e.exists("scans") {
		t.Error("il PDF dell'anteprima va in dati\\anteprima")
	}
	code, r := e.save(id, "confirm", nil, []string{"capacity"}, nil)
	if code != 200 || r["renamed"] != true {
		t.Fatalf("conferma: %d %v", code, r)
	}
	nd := r["doc"].(map[string]any)
	const base = "MARIO - ROSSI - P1234567"
	if nd["dir"] != "scans" || nd["unsaved"] != false || nd["file"] != base+".png" || !strings.Contains(nd["image_url"].(string), "file=") || strings.Contains(nd["image_url"].(string), "dir=preview") {
		t.Fatalf("documento dopo la conferma: %v", nd)
	}
	// solo ora esistono i file: immagine, PDF (con lo stesso nome base), JSON e verita'
	for _, f := range []string{base + ".png", base + ".pdf", base + ".json"} {
		if !e.exists("scans", f) {
			t.Errorf("manca scans\\%s", f)
		}
	}
	if !e.exists("scans", "verita", base+".json") {
		t.Error("verita' mancante")
	}
	// l'anteprima temporanea e' sparita (spostata, non copiata)
	if e.exists("dati", "anteprima", "anteprima-001.png") || e.exists("dati", "anteprima", "anteprima-001.pdf") {
		t.Error("l'anteprima doveva essere spostata")
	}
	_, l := e.do("GET", "/api/previews", nil)
	if len(l["items"].([]any)) != 0 {
		t.Error("l'anteprima salvata non e' piu' non salvata")
	}
	// una seconda conferma non cambia nome
	code, r = e.save(id, "confirm", nil, []string{"capacity"}, map[string]any{"overwrite": true})
	if code != 200 || r["doc"].(map[string]any)["file"] != base+".png" {
		t.Errorf("riconferma: %d %v", code, r["doc"])
	}
	// il PDF dopo il salvataggio va in scans con lo stesso nome base
	if code, pr := e.do("POST", "/api/pdf", map[string]any{"file": base + ".png", "overwrite": true}); code != 200 || pr["file"] != filepath.Join("scans", base+".pdf") {
		t.Errorf("pdf dopo il salvataggio: %d %v", code, pr)
	}
}

func TestSalvataggioConNomeEsistenteUsaDue(t *testing.T) {
	e := newEnv(t, nil)
	scans := filepath.Join(e.base, "scans")
	_ = os.MkdirAll(scans, 0o700)
	prev := []byte("immagine dell'utente: da non toccare")
	_ = os.WriteFile(filepath.Join(scans, "MARIO - ROSSI - P1234567.png"), prev, 0o600)
	e.previewDone()
	doc := e.readPreview("anteprima-001.png")
	code, r := e.save(doc["doc_id"].(string), "confirm", nil, []string{"capacity"}, nil)
	if code != 200 || r["doc"].(map[string]any)["file"] != "MARIO - ROSSI - P1234567 (2).png" {
		t.Fatalf("%d %v", code, r["doc"])
	}
	if b, _ := os.ReadFile(filepath.Join(scans, "MARIO - ROSSI - P1234567.png")); !bytes.Equal(b, prev) {
		t.Error("file esistente sovrascritto")
	}
}

func TestSalvataggioSenzaDatiUsaIlNomeDiRiserva(t *testing.T) {
	e := newEnv(t, nil, "FAKE_EXTRACT=empty")
	e.previewDone()
	doc := e.readPreview("anteprima-001.png")
	id := doc["doc_id"].(string)
	if p := e.do2("POST", "/api/preview-name", map[string]any{"doc_id": id, "fallback_name": "mio-nome"}); p["ok"] != false || p["fallback"] != "mio-nome" {
		t.Errorf("anteprima del nome senza dati: %v", p)
	}
	code, r := e.save(id, "confirm", nil, nil, map[string]any{"fallback_name": "mio-nome"})
	if code != 200 || r["doc"].(map[string]any)["file"] != "mio-nome.png" {
		t.Fatalf("%d %v", code, r["doc"])
	}
	if !e.exists("scans", "mio-nome.png") || !e.exists("scans", "mio-nome.json") || !e.exists("scans", "verita", "mio-nome.json") {
		t.Errorf("file: %v", e.scansEntries())
	}
	// un nome di riserva non valido o gia' usato: libretto-NNN / (2), mai sovrascrivere
	doc = e.readPreview(e.previewDone()["preview"].(map[string]any)["file"].(string))
	code, r = e.save(doc["doc_id"].(string), "confirm", nil, nil, map[string]any{"fallback_name": "mio-nome"})
	if code != 200 || r["doc"].(map[string]any)["file"] != "mio-nome (2).png" {
		t.Errorf("nome di riserva occupato: %d %v", code, r["doc"])
	}
	doc = e.readPreview(e.previewDone()["preview"].(map[string]any)["file"].(string))
	code, r = e.save(doc["doc_id"].(string), "confirm", nil, nil, map[string]any{"fallback_name": "../evil"})
	if code != 200 || r["doc"].(map[string]any)["file"] != "libretto-001.png" {
		t.Errorf("nome di riserva non valido: %d %v", code, r["doc"])
	}
}

func (e *env) do2(method, path string, body any) map[string]any {
	e.t.Helper()
	_, r := e.do(method, path, body)
	return r
}

func TestScartaSoloLAnteprimaDellApp(t *testing.T) {
	e := newEnv(t, nil)
	e.previewDone()
	e.previewDone()
	// file dell'utente e file estranei: mai toccati
	e.writeScan("mia.png")
	_ = os.WriteFile(filepath.Join(e.base, "dati", "anteprima", "nota.txt"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(e.base, "dati", "anteprima", "anteprima-001.pdf"), []byte("pdf"), 0o600)
	for _, f := range []string{"../../scans/mia.png", "mia.png", "nota.txt", "anteprima-001.pdf", "anteprima-9.png/", "..\\x.png", "", "anteprima-099.png"} {
		if c, _ := e.do("POST", "/api/preview/discard", map[string]string{"file": f}); c != 404 {
			t.Errorf("scarto di %q: %d (atteso 404)", f, c)
		}
	}
	if !e.exists("scans", "mia.png") || !e.exists("dati", "anteprima", "nota.txt") {
		t.Fatal("file non dell'app toccati")
	}
	doc := e.readPreview("anteprima-001.png")
	if c, _ := e.do("POST", "/api/preview/discard", map[string]string{"file": "anteprima-001.png"}); c != 200 {
		t.Fatalf("scarto: %d", c)
	}
	if e.exists("dati", "anteprima", "anteprima-001.png") || e.exists("dati", "anteprima", "anteprima-001.pdf") {
		t.Error("anteprima e PDF di anteprima vanno cancellati")
	}
	if !e.exists("dati", "anteprima", "anteprima-002.png") || !e.exists("scans", "mia.png") || !e.exists("dati", "anteprima", "nota.txt") {
		t.Error("lo scarto ha toccato altri file")
	}
	// il documento letto da quell'anteprima non esiste piu'
	if c, _ := e.do("GET", "/api/doc/"+doc["doc_id"].(string), nil); c != 404 {
		t.Errorf("documento dell'anteprima scartata: %d", c)
	}
	_, st := e.do("GET", "/api/state", nil)
	if st["last_doc"] != nil {
		t.Error("l'ultimo documento era quello scartato")
	}
	// nuova anteprima: numero progressivo senza riusare quelli esistenti
	if j := e.previewDone(); j["preview"].(map[string]any)["file"] != "anteprima-003.png" {
		t.Errorf("progressivo: %v", j["preview"])
	}
}

func TestStrisciaDelleUltimeAnteprime(t *testing.T) {
	e := newEnv(t, nil)
	for i := 0; i < 6; i++ {
		e.previewDone()
		time.Sleep(15 * time.Millisecond)
	}
	e.readPreview("anteprima-004.png")
	_, l := e.do("GET", "/api/previews", nil)
	items := l["items"].([]any)
	if len(items) != 6 {
		t.Fatalf("anteprime = %d", len(items))
	}
	if items[0].(map[string]any)["file"] != "anteprima-006.png" || items[5].(map[string]any)["file"] != "anteprima-001.png" {
		t.Errorf("ordine: piu' recente per prima: %v %v", items[0].(map[string]any)["file"], items[5].(map[string]any)["file"])
	}
	for _, it := range items {
		m := it.(map[string]any)
		withDoc := m["file"] == "anteprima-004.png"
		if (m["doc_id"] != nil) != withDoc || m["width"].(float64) == 0 {
			t.Errorf("voce: %v", m)
		}
	}
	// riavvio: le anteprime sono file, restano
	app2, err := newApp(config{Base: e.base, Exe: fakeExe})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	ents, _ := os.ReadDir(app2.previewDir())
	for _, en := range ents {
		if rePreviewPNG.MatchString(en.Name()) {
			n++
		}
	}
	if n != 6 {
		t.Errorf("dopo il riavvio: %d anteprime", n)
	}
}

func TestRicominciaConAnteprimeNonSalvate(t *testing.T) {
	e := newEnv(t, nil)
	e.previewDone()
	e.previewDone()
	if c, _ := e.do("POST", "/api/reset", map[string]any{}); c != 200 {
		t.Fatalf("reset: %d", c)
	}
	// ricominciare non cancella le anteprime: restano nella striscia
	_, l := e.do("GET", "/api/previews", nil)
	if len(l["items"].([]any)) != 2 || !e.exists("dati", "anteprima", "anteprima-001.png") {
		t.Error("le anteprime non salvate devono restare")
	}
}

func TestPuliziaAnteprimeVecchie(t *testing.T) {
	e := newEnv(t, nil)
	dir := e.app.previewDir()
	_ = os.MkdirAll(dir, 0o700)
	old := time.Now().Add(-8 * 24 * time.Hour)
	fresh := time.Now().Add(-6 * 24 * time.Hour)
	mk := func(name string, mt time.Time) {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, []byte("x"), 0o600)
		_ = os.Chtimes(p, mt, mt)
	}
	mk("anteprima-001.png", old)
	mk("anteprima-001.pdf", old)
	mk("anteprima-002.png", fresh)
	mk("altro.png", old) // non e' un file dell'app
	mk("nota.txt", old)  // idem
	mk("anteprima-x.png", old)
	_ = os.WriteFile(filepath.Join(e.base, "scans.png"), []byte("x"), 0o600)
	e.writeScan("vecchia.png")
	_ = os.Chtimes(filepath.Join(e.base, "scans", "vecchia.png"), old, old)
	if n := e.app.cleanOldPreviews(previewMaxAge); n != 2 {
		t.Errorf("eliminati %d, attesi 2", n)
	}
	for _, f := range []string{"anteprima-001.png", "anteprima-001.pdf"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("%s doveva essere eliminata", f)
		}
	}
	for _, f := range []string{"anteprima-002.png", "altro.png", "nota.txt", "anteprima-x.png"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s non doveva essere toccata", f)
		}
	}
	if !e.exists("scans", "vecchia.png") || !e.exists("scans.png") {
		t.Error("la pulizia tocca solo dati\\anteprima")
	}
}

func TestImmagineBassaEIlSuggerimento(t *testing.T) {
	e := newEnv(t, nil, "FAKE_SIZE=1513x590")
	pv := e.previewDone()["preview"].(map[string]any)
	if pv["low"] != true || pv["width"].(float64) != 1513 || pv["height"].(float64) != 590 {
		t.Errorf("immagine bassa: %v", pv)
	}
	e2 := newEnv(t, nil, "FAKE_SIZE=1475x1146")
	if pv := e2.previewDone()["preview"].(map[string]any); pv["low"] != false {
		t.Errorf("immagine normale: %v", pv)
	}
}

func TestAnteprimaErroriEDemo(t *testing.T) {
	// errori dello scanner: nessun file residuo, nessuna anteprima
	for _, c := range []struct{ mode, want string }{{"paper", "Nessun libretto"}, {"jam", "Inceppamento"}} {
		e := newEnv(t, nil, "FAKE_SCAN="+c.mode)
		_, r := e.do("POST", "/api/preview", prevBody)
		j := e.waitJob(r["job"].(string))
		if j["phase"] != "error" || !strings.Contains(j["error"].(string), c.want) {
			t.Errorf("%s: %v", c.mode, j)
		}
		if ents, _ := os.ReadDir(e.app.previewDir()); len(ents) != 0 {
			t.Errorf("%s: file residui %v", c.mode, ents)
		}
	}
	// parametri non validi
	e := newEnv(t, nil)
	for _, m := range []map[string]any{{"driver": "x", "dpi": 300, "len_mm": 97, "top_margin_mm": 2}, {"driver": "twain", "dpi": 9000, "len_mm": 97, "top_margin_mm": 2}} {
		if c, _ := e.do("POST", "/api/preview", m); c != 400 {
			t.Errorf("parametri non validi: %d", c)
		}
	}
	// demo: copia l'immagine nella cartella temporanea, non in scans, e durante la "scansione" non si annulla
	src := filepath.Join(t.TempDir(), "demo.png")
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewGray(image.Rect(0, 0, 40, 30)))
	_ = os.WriteFile(src, b.Bytes(), 0o600)
	ed := newEnv(t, func(c *config) { c.Demo = src })
	_, r := ed.do("POST", "/api/preview", prevBody)
	time.Sleep(100 * time.Millisecond)
	if c, _ := ed.do("POST", "/api/jobs/"+r["job"].(string)+"/cancel", map[string]any{}); c != 409 {
		t.Errorf("annulla durante la scansione: %d", c)
	}
	j := ed.waitJob(r["job"].(string))
	if j["phase"] != "done" || j["demo"] != true {
		t.Fatalf("demo: %v", j)
	}
	got, _ := os.ReadFile(filepath.Join(ed.base, "dati", "anteprima", "anteprima-001.png"))
	if !bytes.Equal(got, b.Bytes()) || ed.exists("scans") {
		t.Error("la demo copia l'immagine nella cartella temporanea e non in scans")
	}
}

func TestLetturaDiUnAnteprimaAnnullabile(t *testing.T) {
	e0 := newEnv(t, nil)
	e0.previewDone()
	_ = os.Remove(filepath.Join(e0.base, "dati", "anteprima", "anteprima-001-lettura.json")) // senza cache la lettura parte davvero
	e := newEnvOn(t, e0.base, "FAKE_EXTRACT=slow")
	_, r := e.do("POST", "/api/open", map[string]string{"file": "anteprima-001.png", "dir": "preview"})
	id := r["job"].(string)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, j := e.do("GET", "/api/jobs/"+id, nil)
		if j["phase"] == "reading" && j["cancelable"] == true {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("lettura non partita: %v", j)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if c, _ := e.do("POST", "/api/jobs/"+id+"/cancel", map[string]any{}); c != 200 {
		t.Fatal(c)
	}
	if j := e.waitJob(id); j["phase"] != "cancelled" {
		t.Errorf("fase: %v", j["phase"])
	}
	if !e0.exists("dati", "anteprima", "anteprima-001.png") || e0.exists("scans") {
		t.Error("annullare la lettura non tocca l'anteprima")
	}
}

func TestApriImmagineSalvataContinuaAFunzionare(t *testing.T) {
	e := newEnv(t, nil)
	p := e.writeScan("vecchia.png")
	before, _ := os.ReadFile(p)
	doc := e.open("vecchia.png")
	if doc["dir"] != "scans" || doc["unsaved"] != false || doc["file"] != "vecchia.png" {
		t.Errorf("%v", doc)
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) || len(e.scansEntries()) != 1 {
		t.Error("aprire un'immagine salvata non sposta ne' cambia nulla")
	}
	// un percorso verso l'anteprima con dir scans non esiste, e viceversa
	e.previewDone()
	if c, _ := e.do("POST", "/api/open", map[string]string{"file": "anteprima-001.png"}); c != 404 {
		t.Errorf("anteprima aperta come file di scans: %d", c)
	}
	if c, _ := e.do("POST", "/api/open", map[string]string{"file": "vecchia.png", "dir": "preview"}); c != 404 {
		t.Errorf("file di scans aperto come anteprima: %d", c)
	}
	_ = url.QueryEscape
}
