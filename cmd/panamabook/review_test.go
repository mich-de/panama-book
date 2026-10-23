package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"scanlibretto/internal/memoria"
)

// Dati sintetici (finto libretto): con FAKE_EXTRACT=ok il campo capacity ha un avviso di lettura incerta, quindi e'
// "Da controllare"; certificate e' assente (non e' un errore).

func (e *env) save(docID, mode string, values map[string]string, reviewed []string, extra map[string]any) (int, map[string]any) {
	e.t.Helper()
	body := map[string]any{"doc_id": docID, "mode": mode, "values": values, "reviewed": reviewed}
	for k, v := range extra {
		body[k] = v
	}
	return e.do("POST", "/api/save", body)
}

func TestPulsanteControllatoEContatore(t *testing.T) {
	e := newEnv(t, nil)
	e.writeScan("a.png")
	doc := e.open("a.png")
	f := field(doc, "capacity")
	if f["status"] != "check" || f["flag"] != true || doc["to_check"].(float64) != 1 {
		t.Fatalf("capacity: %v, da controllare = %v", f, doc["to_check"])
	}
	if !strings.Contains(f["hint"].(string), "incerta") {
		t.Errorf("motivo: %v", f["hint"])
	}
	// un campo OK o assente non e' "da controllare"
	for _, k := range []string{"type", "level", "certificate", "place_of_birth"} {
		if field(doc, k)["flag"] == true {
			t.Errorf("%s non deve essere da controllare: %v", k, field(doc, k))
		}
	}
	// l'operatore segna "controllato" e salva (solo memoria)
	code, sv := e.save(doc["doc_id"].(string), "corrections", nil, []string{"capacity"}, nil)
	if code != 200 || sv["confirmed_saved"].(float64) != 1 {
		t.Fatalf("save: %d %v", code, sv)
	}
	d2 := sv["doc"].(map[string]any)
	if f := field(d2, "capacity"); f["status"] != "reviewed" || f["status_text"] != "Controllato" || f["reviewed"] != "user" || f["hint"] == "" {
		t.Errorf("dopo il controllo: %v (il motivo deve restare consultabile)", f)
	}
	if d2["to_check"].(float64) != 0 {
		t.Errorf("contatore = %v", d2["to_check"])
	}
	// una conferma non e' una correzione
	if m := d2["memory"].(map[string]any); m["known"].(float64) != 0 || m["confirmed"].(float64) != 1 {
		t.Errorf("memoria: %v", m)
	}

	// stessa lettura alla scansione successiva: gia' controllato, senza chiedere
	doc2 := e.open("a.png")
	f2 := field(doc2, "capacity")
	if f2["status"] != "reviewed" || f2["reviewed"] != "memory" || f2["status_text"] != "Controllato (da memoria)" || doc2["to_check"].(float64) != 0 {
		t.Errorf("conferma da memoria: %v", f2)
	}

	// valore diverso: torna "Da controllare" e dice cosa era stato confermato
	e2 := newEnvOn(t, e.base, "FAKE_CAPACITY=ABLE SEAMAN")
	doc3 := e2.open("a.png")
	f3 := field(doc3, "capacity")
	if f3["status"] != "check" || !strings.Contains(f3["hint"].(string), "l'ultima volta era stato confermato «ORDINARY SEAMEN»") || doc3["to_check"].(float64) != 1 {
		t.Errorf("valore cambiato: %v", f3)
	}

	// annulla ("rimetti da controllare"): non si manda il campo tra i controllati e la conferma ricordata sparisce
	_, sv = e.save(doc2["doc_id"].(string), "corrections", nil, []string{}, nil)
	if f := field(sv["doc"].(map[string]any), "capacity"); f["status"] != "check" {
		t.Errorf("dopo l'annullamento: %v", f)
	}
	if n := len(e.app.mem.Elenco()); n != 0 {
		t.Errorf("la conferma annullata doveva sparire dalla memoria: %d libretti", n)
	}
	doc4 := e.open("a.png")
	if field(doc4, "capacity")["status"] != "check" {
		t.Error("senza conferma ricordata il campo torna da controllare")
	}
}

func TestModificareValeCometControllato(t *testing.T) {
	e := newEnv(t, nil)
	e.writeScan("a.png")
	doc := e.open("a.png")
	// il campo e' modificato (non segnato controllato): la conferma non lo trova "aperto"
	code, sv := e.save(doc["doc_id"].(string), "confirm", map[string]string{"capacity": "ordinary seaman"}, nil, nil)
	if code != 200 {
		t.Fatalf("un campo modificato conta come controllato: %d %v", code, sv)
	}
	if f := field(sv["doc"].(map[string]any), "capacity"); f["flag"] == true {
		t.Errorf("campo modificato: %v", f)
	}
	b, _ := os.ReadFile(filepath.Join(e.base, "scans", "verita", "MARIO - ROSSI - P1234567.json"))
	var tr map[string]any
	_ = json.Unmarshal(b, &tr)
	if tr["capacity"] != "ORDINARY SEAMAN" {
		t.Errorf("verita': %v", tr["capacity"])
	}
}

func TestConfermaConCampiAperti(t *testing.T) {
	e := newEnv(t, nil)
	e.writeScan("a.png")
	doc := e.open("a.png")
	id := doc["doc_id"].(string)

	// campi ancora da controllare: avviso con l'elenco, nessun file scritto
	code, r := e.save(id, "confirm", nil, nil, nil)
	if code != 409 || r["code"] != "campi_aperti" {
		t.Fatalf("senza controllo: %d %v", code, r)
	}
	open := r["open"].([]any)
	if len(open) != 1 || open[0].(map[string]any)["key"] != "capacity" || !strings.Contains(open[0].(map[string]any)["label"].(string), "Capacity") {
		t.Errorf("elenco dei campi aperti: %v", open)
	}
	if _, err := os.Stat(filepath.Join(e.base, "scans", "verita")); err == nil {
		t.Error("nulla deve essere scritto prima della scelta")
	}
	if _, err := os.Stat(filepath.Join(e.base, "scans", "MARIO - ROSSI - P1234567.json")); err == nil {
		t.Error("JSON scritto senza scelta")
	}

	// "Conferma comunque": la verita' lascia vuoto il campo non controllato, con una nota
	code, r = e.save(id, "confirm", nil, nil, map[string]any{"accept_open": true})
	if code != 200 {
		t.Fatalf("conferma comunque: %d %v", code, r)
	}
	tb, _ := os.ReadFile(filepath.Join(e.base, "scans", "verita", "MARIO - ROSSI - P1234567.json"))
	var tr map[string]any
	if err := json.Unmarshal(tb, &tr); err != nil {
		t.Fatal(err)
	}
	if tr["capacity"] != "" || tr["type"] != "SI" {
		t.Errorf("verita': capacity=%v type=%v", tr["capacity"], tr["type"])
	}
	if n := tr["_note"].(map[string]any)["capacity"]; n == nil || !strings.Contains(n.(string), "non controllato") {
		t.Errorf("nota: %v", n)
	}
	jb, _ := os.ReadFile(filepath.Join(e.base, "scans", "MARIO - ROSSI - P1234567.json"))
	var saved map[string]any
	_ = json.Unmarshal(jb, &saved)
	rv := saved["review"].(map[string]any)
	if u := rv["unchecked"].([]any); len(u) != 1 || u[0] != "capacity" {
		t.Errorf("review.unchecked: %v", rv)
	}
	if len(rv["reviewed"].([]any)) != 0 {
		t.Errorf("review.reviewed: %v", rv["reviewed"])
	}

	// con il campo controllato non c'e' nessun avviso e la verita' ha il valore
	e2 := newEnv(t, nil)
	e2.writeScan("b.png")
	d2 := e2.open("b.png")
	code, r = e2.save(d2["doc_id"].(string), "confirm", nil, []string{"capacity"}, nil)
	if code != 200 {
		t.Fatalf("con il campo controllato: %d %v", code, r)
	}
	tb, _ = os.ReadFile(filepath.Join(e2.base, "scans", "verita", "MARIO - ROSSI - P1234567.json"))
	tr = nil
	_ = json.Unmarshal(tb, &tr)
	if tr["capacity"] != "ORDINARY SEAMEN" {
		t.Errorf("verita' del campo controllato: %v", tr["capacity"])
	}
	jb, _ = os.ReadFile(filepath.Join(e2.base, "scans", "MARIO - ROSSI - P1234567.json"))
	_ = json.Unmarshal(jb, &saved)
	rv = saved["review"].(map[string]any)
	rl := rv["reviewed"].([]any)
	if len(rl) != 1 || rl[0].(map[string]any)["field"] != "capacity" || rl[0].(map[string]any)["source"] != "user" {
		t.Errorf("review.reviewed: %v", rl)
	}
	// il blocco review non duplica i valori
	blk, _ := json.Marshal(rv)
	if bytes.Contains(blk, []byte("ORDINARY")) {
		t.Errorf("il blocco review contiene un valore: %s", blk)
	}
}

func TestControlloSenzaMRZValidaNonSiRicorda(t *testing.T) {
	e := newEnv(t, nil, "FAKE_EXTRACT=mrz_invalid")
	e.writeScan("a.png")
	doc := e.open("a.png")
	// con MRZ non valida i campi con avvisi della lettura restano da controllare come prima (qui il finto non ne ha): usiamo un formato anomalo
	code, sv := e.save(doc["doc_id"].(string), "corrections", map[string]string{"date_of_birth": "31-02-1990"}, nil, nil)
	if code != 200 {
		t.Fatal(sv)
	}
	if _, err := os.Stat(filepath.Join(e.base, "dati", "memoria.json")); err == nil {
		t.Error("nessuna memoria deve nascere senza MRZ valida")
	}
}

func TestMemoriaElencoConfermeEDimentica(t *testing.T) {
	e := newEnv(t, nil)
	e.writeScan("a.png")
	doc := e.open("a.png")
	e.save(doc["doc_id"].(string), "corrections", nil, []string{"capacity"}, nil)
	_, ml := e.do("GET", "/api/memory", nil)
	items := ml["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("elenco: %v", ml)
	}
	campi := items[0].(map[string]any)["campi"].([]any)
	if len(campi) != 1 || campi[0].(map[string]any)["tipo"] != "conferma" || campi[0].(map[string]any)["corretto"] != "ORDINARY SEAMEN" {
		t.Errorf("righe: %v", campi)
	}
	// dimentica la conferma: il documento aperto torna da controllare
	doc2 := e.open("a.png")
	if field(doc2, "capacity")["status"] != "reviewed" {
		t.Fatal("conferma non applicata")
	}
	key := items[0].(map[string]any)["key"]
	if c, _ := e.do("POST", "/api/memory/forget", map[string]any{"key": key, "field": "capacity", "kind": "conferma"}); c != 200 {
		t.Fatal(c)
	}
	_, dv := e.do("GET", "/api/doc/"+doc2["doc_id"].(string), nil)
	if field(dv, "capacity")["status"] != "check" {
		t.Errorf("dopo aver dimenticato la conferma: %v", field(dv, "capacity"))
	}
	if len(e.app.mem.Elenco()) != 0 {
		t.Error("libretto senza piu' nulla deve sparire")
	}
}

func TestProponeNomeLiberoProgressivo(t *testing.T) {
	e := newEnv(t, nil)
	_, st := e.do("GET", "/api/state", nil)
	if st["suggested"] != "libretto-001" {
		t.Errorf("primo nome: %v", st["suggested"])
	}
	e.writeScan("libretto-001.png")
	e.writeScan("libretto-007.jpg")
	e.writeScan("libretto-20261023-101010.png") // formato a data: non e' un progressivo
	e.writeScan("altro.png")
	_ = os.WriteFile(filepath.Join(e.base, "scans", "libretto-009.json"), []byte("{}"), 0o600)
	_, r := e.do("GET", "/api/suggest", nil)
	if r["name"] != "libretto-010" {
		t.Errorf("prossimo nome: %v", r["name"])
	}
}

func TestRicominciaDaCapo(t *testing.T) {
	e := newEnv(t, nil)
	p := e.writeScan("a.png")
	doc := e.open("a.png")
	e.save(doc["doc_id"].(string), "corrections", map[string]string{"capacity": "ORDINARY SEAMAN"}, nil, nil)
	_ = os.WriteFile(filepath.Join(e.base, "scans", "a.json"), []byte("{}"), 0o600)
	_, st := e.do("GET", "/api/state", nil)
	if st["last_doc"] == nil {
		t.Fatal("dopo una lettura il documento si ritrova al ricaricamento")
	}
	code, r := e.do("POST", "/api/reset", map[string]any{})
	if code != 200 || r["name"] == "" {
		t.Fatalf("reset: %d %v", code, r)
	}
	_, st = e.do("GET", "/api/state", nil)
	if st["last_doc"] != nil {
		t.Error("dopo 'Ricomincia da capo' non si ripropone il vecchio documento")
	}
	// niente e' stato cancellato: immagine, JSON, memoria
	for _, f := range []string{p, filepath.Join(e.base, "scans", "a.json"), filepath.Join(e.base, "dati", "memoria.json")} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("file sparito: %s", f)
		}
	}
	if len(e.app.mem.Elenco()) != 1 {
		t.Error("la memoria non va toccata")
	}
}

func TestRicominciaDuranteLaScansioneELaLettura(t *testing.T) {
	src := filepath.Join(t.TempDir(), "demo.png")
	b := bytes.NewBuffer(nil)
	_ = png.Encode(b, image.NewGray(image.Rect(0, 0, 4, 4)))
	_ = os.WriteFile(src, b.Bytes(), 0o600)
	e := newEnv(t, func(c *config) { c.Demo = src }, "FAKE_EXTRACT=slow")
	_, r := e.do("POST", "/api/scan", map[string]any{"name": "z", "driver": "twain", "dpi": 300, "len_mm": 97, "top_margin_mm": 2})
	time.Sleep(100 * time.Millisecond)
	if code, _ := e.do("POST", "/api/reset", map[string]any{}); code != 409 {
		t.Errorf("reset durante la scansione: %d (atteso 409)", code)
	}
	// finita la scansione parte la lettura (lenta): il reset la annulla
	id := r["job"].(string)
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, j := e.do("GET", "/api/jobs/"+id, nil)
		if j["phase"] == "reading" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("la lettura non e' partita: %v", j)
		}
		time.Sleep(30 * time.Millisecond)
	}
	if code, _ := e.do("POST", "/api/reset", map[string]any{}); code != 200 {
		t.Fatalf("reset durante la lettura: %d", code)
	}
	j := e.waitJob(id)
	if j["phase"] != "cancelled" {
		t.Errorf("la lettura doveva essere annullata: %v", j["phase"])
	}
	if _, err := os.Stat(filepath.Join(e.base, "scans", "z.png")); err != nil {
		t.Error("l'immagine scansionata non va cancellata")
	}
}

func TestReasonsConfermaMemoria(t *testing.T) {
	e := newEnv(t, nil)
	e.writeScan("a.png")
	doc := e.open("a.png")
	d := e.app.getDoc(doc["doc_id"].(string))
	def, _ := findDef("capacity")
	if len(d.reasons(def, "ORDINARY SEAMEN")) == 0 {
		t.Error("l'avviso del lettore e' un motivo")
	}
	d.Confirmed["level"] = memoria.Conferma{Valore: "SUPPORT"}
	defL, _ := findDef("level")
	if r := d.reasons(defL, "SUPPORT"); len(r) != 0 {
		t.Errorf("stesso valore confermato: %v", r)
	}
	if r := d.reasons(defL, "MANAGEMENT"); len(r) != 1 || !strings.Contains(r[0], "SUPPORT") {
		t.Errorf("valore diverso dal confermato: %v", r)
	}
}
