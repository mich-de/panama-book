package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Dati INVENTATI: nessun dato di persone reali.

var fieldKeys = []string{"type", "country_code", "nationality", "place_of_birth", "date_of_issue", "date_of_expiry", "date_of_birth",
	"level", "capacity", "seamans_book", "id_number", "certificate", "seafarers_id", "regional"}

func good() map[string]string {
	return map[string]string{
		"type": "SI", "country_code": "PAN", "nationality": "ITALIAN", "place_of_birth": "ROMA ITA",
		"date_of_issue": "05-03-2026", "date_of_expiry": "04-03-2031", "date_of_birth": "01-01-1990",
		"level": "SUPPORT", "capacity": "ORDINARY SEAMAN", "seamans_book": "PA1234567", "id_number": "AB1234567",
		"certificate": "123456789", "seafarers_id": "987654321", "regional": "PANAMA DR",
		"date_of_issue_iso": "2026-03-05", "date_of_expiry_iso": "2031-03-04", "date_of_birth_iso": "1990-01-01",
		"surname": "ROSSI", "given_names": "MARIO LUIGI", "nationality_iso": "ITA", "sex": "M",
		"date_of_birth_mrz": "1990-01-01", "date_of_expiry_mrz": "2031-03-04", "seamans_book_mrz": "PA1234567",
		"regional_mrz": "PANAMA",
	}
}

func with(m map[string]string, kv ...string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i]] = kv[i+1]
	}
	return out
}

func outputJSON(t *testing.T, fields map[string]string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"schema_version": 1, "rotation_deg": 0, "fields": fields,
		"mrz": map[string]any{"checks": map[string]bool{"doc_number": true, "birth": true, "expiry": true, "optional": true, "composite": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func truthJSON(t *testing.T, m map[string]string) []byte {
	t.Helper()
	out := map[string]any{}
	for _, k := range fieldKeys {
		out[k] = m[k]
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// exec lancia run con lo standard input dato e restituisce codice, standard output e standard error.
func exec(args []string, stdin string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestFileConVerita(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "x.json"), outputJSON(t, good()))
	write(t, filepath.Join(d, "t.json"), truthJSON(t, good()))
	code, out, errs := exec([]string{"-json", filepath.Join(d, "x.json"), "-truth", filepath.Join(d, "t.json")}, "")
	if code != exitOK {
		t.Fatalf("uscita %d\n%s\n%s", code, out, errs)
	}
	if !strings.Contains(out, "14 giusti su 14") || strings.Contains(out, "SBAGLIATO") {
		t.Errorf("uscita perfetta:\n%s", out)
	}
}

// rename restituisce una copia di m con la chiave from rinominata in to.
func rename(m map[string]string, from, to string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	if v, ok := out[from]; ok {
		delete(out, from)
		out[to] = v
	}
	return out
}

func TestVecchioNomeEndorsementEAncoraAccettato(t *testing.T) {
	d := t.TempDir()
	// un libretto.exe vecchio scrive `endorsement`, e le verita' vecchie lo hanno ancora
	old := rename(good(), "certificate", "endorsement")
	write(t, filepath.Join(d, "json", "x.json"), outputJSON(t, old))
	write(t, filepath.Join(d, "verita", "x.json"), truthJSON(t, old))
	for _, args := range [][]string{
		{"-json", filepath.Join(d, "json", "x.json"), "-truth", filepath.Join(d, "verita", "x.json")},
		{"-dir", filepath.Join(d, "json"), "-truth", filepath.Join(d, "verita")},
	} {
		code, out, errs := exec(args, "")
		if code != exitOK {
			t.Fatalf("%v: uscita %d\n%s\n%s", args, code, out, errs)
		}
		if !strings.Contains(out, "certificate") || strings.Contains(out, "endorsement") {
			t.Errorf("%v: l'uscita deve usare il nome attuale certificate:\n%s", args, out)
		}
	}
	// uscita vecchia con verita' nuova, e il contrario
	write(t, filepath.Join(d, "verita", "n.json"), truthJSON(t, good()))
	write(t, filepath.Join(d, "json", "n.json"), outputJSON(t, old))
	write(t, filepath.Join(d, "verita2", "n.json"), truthJSON(t, old))
	write(t, filepath.Join(d, "json2", "n.json"), outputJSON(t, good()))
	if code, out, _ := exec([]string{"-json", filepath.Join(d, "json", "n.json"), "-truth", filepath.Join(d, "verita", "n.json")}, ""); code != exitOK {
		t.Errorf("uscita vecchia, verita' nuova: %d\n%s", code, out)
	}
	if code, out, _ := exec([]string{"-json", filepath.Join(d, "json2", "n.json"), "-truth", filepath.Join(d, "verita2", "n.json")}, ""); code != exitOK {
		t.Errorf("uscita nuova, verita' vecchia: %d\n%s", code, out)
	}
}

func TestCertificatoAssenteNonFaFallire(t *testing.T) {
	d := t.TempDir()
	none := with(good(), "certificate", "")
	write(t, filepath.Join(d, "x.json"), outputJSON(t, none))
	write(t, filepath.Join(d, "t.json"), truthJSON(t, none))
	code, out, _ := exec([]string{"-json", filepath.Join(d, "x.json"), "-truth", filepath.Join(d, "t.json"), "-strict"}, "")
	if code != exitOK || !strings.Contains(out, "13 giusti su 13") || !strings.Contains(out, "NON_STAMPATO") {
		t.Errorf("certificato assente: uscita %d\n%s", code, out)
	}
}

func TestFileConUnCampoSbagliato(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "x.json"), outputJSON(t, with(good(), "capacity", "ZQXJSEAMAN", "id_number", "")))
	write(t, filepath.Join(d, "t.json"), truthJSON(t, good()))
	// argomento posizionale al posto di -json
	code, out, _ := exec([]string{filepath.Join(d, "x.json"), "-truth", filepath.Join(d, "t.json")}, "")
	if code != exitFail {
		t.Fatalf("uscita %d, attesa %d\n%s", code, exitFail, out)
	}
	for _, want := range []string{"capacity", "SBAGLIATO", "id_number", "MANCANTE", "12 giusti su 14"} {
		if !strings.Contains(out, want) {
			t.Errorf("manca %q:\n%s", want, out)
		}
	}
	// nessun valore, ne' letto ne' vero
	for _, secret := range []string{"ZQXJ", "SEAMAN", "ROSSI", "1234567", "ITALIAN", "ROMA"} {
		if strings.Contains(out, secret) {
			t.Errorf("l'uscita contiene un valore (%q):\n%s", secret, out)
		}
	}
}

func TestFileConCartellaDiVerita(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "json", "img.json"), outputJSON(t, good()))
	write(t, filepath.Join(d, "verita", "img.json"), truthJSON(t, good()))
	code, out, errs := exec([]string{"-json", filepath.Join(d, "json", "img.json"), "-truth", filepath.Join(d, "verita")}, "")
	if code != exitOK || !strings.Contains(out, "14 giusti su 14") {
		t.Errorf("uscita %d\n%s\n%s", code, out, errs)
	}
}

func TestFileSoloCoerenza(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "x.json"), outputJSON(t, with(good(), "type", "IS", "nationality", "GERMAN")))
	code, out, _ := exec([]string{"-json", filepath.Join(d, "x.json")}, "")
	if code != exitFail {
		t.Errorf("errori di coerenza: uscita %d\n%s", code, out)
	}
	for _, want := range []string{"nessuna verita'", "FORMAT_TYPE", "NATIONALITY_ISO_MISMATCH"} {
		if !strings.Contains(out, want) {
			t.Errorf("manca %q:\n%s", want, out)
		}
	}
	// il file coerente esce con 0
	write(t, filepath.Join(d, "ok.json"), outputJSON(t, good()))
	if code, out, _ := exec([]string{"-json", filepath.Join(d, "ok.json")}, ""); code != exitOK {
		t.Errorf("file coerente: uscita %d\n%s", code, out)
	}
}

func TestStrictFaFallireGliAvvisi(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "x.json"), outputJSON(t, with(good(), "place_of_birth", "ROMA  ITA")))
	if code, out, _ := exec([]string{"-json", filepath.Join(d, "x.json")}, ""); code != exitOK {
		t.Errorf("un avviso non fa fallire: uscita %d\n%s", code, out)
	}
	if code, out, _ := exec([]string{"-json", filepath.Join(d, "x.json"), "-strict"}, ""); code != exitFail {
		t.Errorf("con -strict l'avviso fa fallire: uscita %d\n%s", code, out)
	}
}

func TestStandardInput(t *testing.T) {
	code, out, errs := exec([]string{"-json", "-"}, string(outputJSON(t, good())))
	if code != exitOK || !strings.Contains(out, "standard input") {
		t.Errorf("uscita %d\n%s\n%s", code, out, errs)
	}
	if code, _, errs := exec([]string{"-json", "-"}, "non json"); code != exitUsage || errs == "" {
		t.Errorf("JSON non valido da stdin: uscita %d, errori %q", code, errs)
	}
}

func TestAiuto(t *testing.T) {
	code, _, errs := exec([]string{"-h"}, "")
	if code != exitOK || !strings.Contains(errs, "verifyjson -dir") {
		t.Errorf("-h: uscita %d, l'aiuto va su standard error:\n%s", code, errs)
	}
}

func TestUsoErrato(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "x.json"), outputJSON(t, good()))
	cases := [][]string{
		{},                                     // niente
		{"-json", "a.json", "-dir", d},         // si escludono
		{"a.json", "b.json"},                   // argomenti in piu'
		{"-json", filepath.Join(d, "no.json")}, // file inesistente
		{"-nonesiste"},                         // flag sconosciuto
		{"-json", filepath.Join(d, "x.json"), "-truth", filepath.Join(d, "no.json")}, // verita' inesistente
		{"-dir", filepath.Join(d, "nonesiste")},
		{"-dir", d, "-only", "("}, // espressione regolare non valida
	}
	for _, args := range cases {
		if code, _, _ := exec(args, ""); code != exitUsage {
			t.Errorf("run(%v) = %d, atteso %d", args, code, exitUsage)
		}
	}
	// un JSON che non e' un'uscita di libretto
	write(t, filepath.Join(d, "vecchio.json"), []byte(`{"tipo":"SI"}`))
	if code, _, errs := exec([]string{"-json", filepath.Join(d, "vecchio.json")}, ""); code != exitUsage || !strings.Contains(errs, "schema_version") {
		t.Errorf("schema vecchio: uscita %d, errori %q", code, errs)
	}
}

func TestDirConFiltri(t *testing.T) {
	d := t.TempDir()
	outs, truths := filepath.Join(d, "json"), filepath.Join(d, "verita")
	write(t, filepath.Join(outs, "uno.json"), outputJSON(t, good()))
	write(t, filepath.Join(outs, "uno-grezza.json"), outputJSON(t, with(good(), "capacity", "ZQXJ")))
	write(t, filepath.Join(truths, "uno.json"), truthJSON(t, good()))
	write(t, filepath.Join(truths, "uno-grezza.json"), truthJSON(t, good()))

	code, out, errs := exec([]string{"-dir", outs, "-truth", truths}, "")
	if code != exitFail || !strings.Contains(out, "ESITO: FALLITO") || !strings.Contains(out, "SBAGLIATO: capacity") {
		t.Errorf("tutte: uscita %d\n%s\n%s", code, out, errs)
	}
	if !strings.Contains(out, "ACCURATEZZA PER CAMPO (2 immagini") {
		t.Errorf("tabella:\n%s", out)
	}
	// escludendo la grezza resta una sola immagine, perfetta
	code, out, _ = exec([]string{"-dir", outs, "-truth", truths, "-exclude", "grezza"}, "")
	if code != exitOK || !strings.Contains(out, "ESITO: OK") || !strings.Contains(out, "(1 immagini") {
		t.Errorf("-exclude: uscita %d\n%s", code, out)
	}
	code, out, _ = exec([]string{"-dir", outs, "-truth", truths, "-only", "^uno$"}, "")
	if code != exitOK || !strings.Contains(out, "(1 immagini") {
		t.Errorf("-only: uscita %d\n%s", code, out)
	}
	// filtri che non lasciano nulla
	if code, _, _ := exec([]string{"-dir", outs, "-truth", truths, "-only", "^nessuna$"}, ""); code != exitUsage {
		t.Errorf("-only senza risultati: uscita %d", code)
	}
	// dettaglio per immagine
	code, out, _ = exec([]string{"-dir", outs, "-truth", truths, "-v"}, "")
	if code != exitFail || !strings.Contains(out, "uno: rotazione 0") || !strings.Contains(out, "esito:") {
		t.Errorf("-v: uscita %d\n%s", code, out)
	}
	for _, secret := range []string{"ZQXJ", "ROSSI", "1234567", "ITALIAN"} {
		if strings.Contains(out, secret) {
			t.Errorf("l'uscita contiene un valore (%q)", secret)
		}
	}
	// la verita' di -dir dev'essere una cartella
	if code, _, _ := exec([]string{"-dir", outs, "-truth", filepath.Join(truths, "uno.json")}, ""); code != exitUsage {
		t.Errorf("-truth file con -dir: uscita %d", code)
	}
}

func TestDirVeritaPredefinita(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "json", "uno.json"), outputJSON(t, with(good(), "capacity", "MAS")))
	write(t, filepath.Join(d, "scans", "verita", "uno.json"), truthJSON(t, good()))
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	code, out, _ := exec([]string{"-dir", "json"}, "")
	if code != exitFail || !strings.Contains(out, "giusti 13/14") {
		t.Errorf("verita' predefinita scans\\verita: uscita %d\n%s", code, out)
	}
}
