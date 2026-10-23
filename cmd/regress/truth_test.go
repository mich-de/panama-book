package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scanlibretto/internal/outsum"
)

func TestLoadTruth(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.json"), []byte(`{"type":"SI","_tipo":"pagina dati"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rotta.json"), []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	// il nome della verita' e' quello dell'immagine senza estensione, anche per le sottocartelle
	for _, rel := range []string{"scans/x.png", "scans/vecchi test WIA/x.png", "facsimili/panama/x.jpg"} {
		truth, ok, err := loadTruth(dir, rel)
		if err != nil || !ok || truth.Fields["type"] != "SI" {
			t.Errorf("%s: ok=%v err=%v", rel, ok, err)
		}
	}
	if _, ok, err := loadTruth(dir, "scans/senza-verita.png"); ok || err != nil {
		t.Errorf("verita' assente: ok=%v err=%v", ok, err)
	}
	if _, ok, err := loadTruth(dir, "scans/rotta.png"); ok || err == nil {
		t.Errorf("verita' rotta: ok=%v err=%v", ok, err)
	}
}

// La traduzione degli esiti di verify.Compare in simboli, e il confronto normalizzato (maiuscole, date ISO,
// trattini) che passa da verify.Equal.
func TestScoreTruth(t *testing.T) {
	s := outsum.Summary{Fields: map[string]string{
		"type":           "si",         // giusto (maiuscole)
		"country_code":   "PAN",        // giusto
		"nationality":    "ITALIAN",    // sbagliato
		"place_of_birth": "",           // mancante: la verita' ha un valore
		"date_of_issue":  "09-03-2026", // giusto (data ISO in verita')
		"date_of_expiry": "",           // vuoto da entrambe le parti
		"level":          "OPERATIONAL",
		"capacity":       "ORDINARY-SEAMAN",                     // giusto (trattino)
		"surname":        "ROSSI", "given_names": "MARIA ELENA", // dall'MRZ
	}}
	truthJSON := `{"type": "SI", "country_code": "PAN", "nationality": "GREEK", "place_of_birth": "COLON PAN",
		"date_of_issue": "2026-03-09", "date_of_expiry": "", "level": "", "capacity": "ordinary seaman",
		"_riga_nome": "MARIA ELENA ROSSI"}`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(truthJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	truth, ok, err := loadTruth(dir, "scans/a.png")
	if err != nil || !ok {
		t.Fatalf("verita': ok=%v err=%v", ok, err)
	}
	// ordine: type country_code nationality place_of_birth date_of_issue date_of_expiry date_of_birth level capacity ...
	got, name := scoreTruth(s, truth)
	if want := "GGSMG--XG-----"; got != want {
		t.Errorf("mappa %q, attesa %q", got, want)
	}
	if name != symRight {
		t.Errorf("nomi: %c, attesi G (parole uguali senza ordine)", name)
	}
	// nomi sbagliati, MRZ senza nomi, verita' senza riga del nome
	s.Fields["surname"] = "ROSSX"
	if _, name := scoreTruth(s, truth); name != symWrong {
		t.Errorf("nomi sbagliati: %c", name)
	}
	s.Fields["surname"], s.Fields["given_names"] = "", ""
	if _, name := scoreTruth(s, truth); name != symMissing {
		t.Errorf("MRZ senza nomi: %c", name)
	}
	senzaRiga, _ := os.ReadFile(filepath.Join(dir, "a.json"))
	if err := os.WriteFile(filepath.Join(dir, "b.json"), []byte(strings.Replace(string(senzaRiga), `"_riga_nome": "MARIA ELENA ROSSI"`, `"_tipo": "x"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	truthB, _, _ := loadTruth(dir, "scans/b.png")
	if _, name := scoreTruth(s, truthB); name != 0 {
		t.Errorf("verita' senza riga del nome: %c, atteso 0", name)
	}
}
