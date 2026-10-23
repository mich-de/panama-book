package memoria

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"scanlibretto/internal/mrz"
)

// Dati sintetici: nessun valore reale.
func mrzProva() *mrz.Data {
	return &mrz.Data{
		DocNumber: "PA1234567", BirthDate: "1990-01-01",
		Checks: mrz.Checks{DocNumber: true, Birth: true, Expiry: true, Optional: true, Composite: true},
	}
}

func TestChiaveRichiedeMRZValida(t *testing.T) {
	k, err := Chiave(mrzProva())
	if err != nil || k != "PA1234567|1990-01-01" {
		t.Fatalf("Chiave = %q, %v", k, err)
	}
	if _, err := Chiave(nil); !errors.Is(err, ErrMRZNonValida) {
		t.Errorf("MRZ nil: err = %v", err)
	}
	for i := 0; i < 5; i++ {
		m := mrzProva()
		switch i {
		case 0:
			m.Checks.DocNumber = false
		case 1:
			m.Checks.Birth = false
		case 2:
			m.Checks.Expiry = false
		case 3:
			m.Checks.Optional = false
		case 4:
			m.Checks.Composite = false
		}
		if _, err := Chiave(m); !errors.Is(err, ErrMRZNonValida) {
			t.Errorf("check %d falso: err = %v", i, err)
		}
	}
	m := mrzProva()
	m.DocNumber = "<<<<<<<<<"
	if _, err := Chiave(m); err == nil {
		t.Error("numero vuoto accettato")
	}
}

func TestSalvaRileggiDimentica(t *testing.T) {
	p := filepath.Join(t.TempDir(), "dati", "memoria.json")
	a, err := Apri(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("il file non deve esistere finche' non si salva")
	}
	k, _ := Chiave(mrzProva())
	if err := a.Salva(k, "ROSSI MARIO", "PA1234567", map[string]Correzione{
		"capacity": {Corretto: "ORDINARY SEAMAN", Letto: "ORD SEAMAN"},
		"level":    {Corretto: "SUPPORT", Letto: "SUPPOR"},
	}); err != nil {
		t.Fatal(err)
	}
	b, err := Apri(p)
	if err != nil {
		t.Fatal(err)
	}
	c := b.Campi(k)
	if len(c) != 2 || c["capacity"].Corretto != "ORDINARY SEAMAN" || c["capacity"].Letto != "ORD SEAMAN" || c["capacity"].Data.IsZero() {
		t.Fatalf("campi riletti: %+v", c)
	}
	if l := b.Elenco(); len(l) != 1 || l[0].Etichetta != "ROSSI MARIO" || l[0].Numero != "PA1234567" {
		t.Fatalf("elenco: %+v", l)
	}
	if len(b.Campi("PA7654321|1990-01-01")) != 0 {
		t.Error("campi di un altro libretto")
	}
	if err := b.DimenticaCampo(k, "level"); err != nil {
		t.Fatal(err)
	}
	if c := b.Campi(k); len(c) != 1 {
		t.Fatalf("dopo DimenticaCampo: %+v", c)
	}
	if err := b.DimenticaCampo(k, "capacity"); err != nil {
		t.Fatal(err)
	}
	if len(b.Elenco()) != 0 {
		t.Error("un libretto senza campi deve sparire dall'elenco")
	}
	if err := b.Salva(k, "X", "", map[string]Correzione{"sex": {Corretto: "M", Letto: "F"}}); err != nil {
		t.Fatal(err)
	}
	if err := b.DimenticaLibretto(k); err != nil {
		t.Fatal(err)
	}
	c2, _ := Apri(p)
	if len(c2.Elenco()) != 0 {
		t.Error("DimenticaLibretto non scritto su disco")
	}
}

func TestSostituisceCampoEDataAutomatica(t *testing.T) {
	a, _ := Apri(filepath.Join(t.TempDir(), "m.json"))
	k := "PA1|2000-01-01"
	_ = a.Salva(k, "", "", map[string]Correzione{"capacity": {Corretto: "A", Letto: "B"}})
	_ = a.Salva(k, "", "", map[string]Correzione{"capacity": {Corretto: "C", Letto: "B"}})
	c := a.Campi(k)["capacity"]
	if c.Corretto != "C" || c.Data.IsZero() {
		t.Errorf("campo non sostituito: %+v", c)
	}
}

func TestScritturaAtomicaSenzaResidui(t *testing.T) {
	dir := t.TempDir()
	a, _ := Apri(filepath.Join(dir, "memoria.json"))
	for i := 0; i < 3; i++ {
		if err := a.Salva("K|1", "", "", map[string]Correzione{"sex": {Corretto: "M", Letto: "F"}}); err != nil {
			t.Fatal(err)
		}
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 || ents[0].Name() != "memoria.json" {
		t.Errorf("file residui: %v", ents)
	}
}

func TestFileRottoEVersioneFutura(t *testing.T) {
	p := filepath.Join(t.TempDir(), "m.json")
	_ = os.WriteFile(p, []byte("{non json"), 0o600)
	if _, err := Apri(p); err == nil {
		t.Error("file rotto accettato")
	}
	_ = os.WriteFile(p, []byte(`{"versione":99,"libretti":{}}`), 0o600)
	if _, err := Apri(p); err == nil {
		t.Error("versione futura accettata")
	}
	_ = os.WriteFile(p, []byte(""), 0o600)
	if a, err := Apri(p); err != nil || len(a.Elenco()) != 0 {
		t.Errorf("file vuoto: %v", err)
	}
}

func TestSalvaSenzaChiaveONulla(t *testing.T) {
	a, _ := Apri(filepath.Join(t.TempDir(), "m.json"))
	if err := a.Salva("", "", "", map[string]Correzione{"sex": {Corretto: "M"}}); !errors.Is(err, ErrMRZNonValida) {
		t.Errorf("chiave vuota: %v", err)
	}
	if err := a.Salva("K|1", "", "", nil); err != nil {
		t.Errorf("niente da salvare: %v", err)
	}
}

func TestConfermeSalvaRimuoviPersistono(t *testing.T) {
	p := filepath.Join(t.TempDir(), "m.json")
	a, _ := Apri(p)
	k := "PA1234567|1990-01-01"
	if err := a.SalvaConferme(k, "ROSSI MARIO", "PA1234567", map[string]string{"capacity": "ORDINARY SEAMAN", "level": "SUPPORT"}); err != nil {
		t.Fatal(err)
	}
	b, _ := Apri(p)
	c := b.Confermati(k)
	if len(c) != 2 || c["capacity"].Valore != "ORDINARY SEAMAN" || c["capacity"].Data.IsZero() {
		t.Fatalf("conferme: %+v", c)
	}
	if len(b.Campi(k)) != 0 {
		t.Error("una conferma non e' una correzione")
	}
	if l := b.Elenco(); len(l) != 1 || len(l[0].Confermati) != 2 {
		t.Fatalf("elenco: %+v", l)
	}
	// una correzione dello stesso campo sostituisce la conferma
	if err := b.Salva(k, "", "", map[string]Correzione{"capacity": {Corretto: "AB", Letto: "ORDINARY SEAMAN"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Confermati(k)["capacity"]; ok {
		t.Error("la conferma doveva sparire con la correzione")
	}
	// DimenticaCampo non toglie le conferme e il libretto resta finche' c'e' qualcosa
	if err := b.DimenticaCampo(k, "capacity"); err != nil {
		t.Fatal(err)
	}
	if len(b.Elenco()) != 1 || len(b.Confermati(k)) != 1 {
		t.Fatalf("dopo DimenticaCampo: %+v", b.Elenco())
	}
	if err := b.RimuoviConferme(k, "level", "inesistente"); err != nil {
		t.Fatal(err)
	}
	if len(b.Elenco()) != 0 {
		t.Error("senza correzioni e senza conferme il libretto deve sparire")
	}
	if err := b.RimuoviConferme("altro|1", "x"); err != nil {
		t.Errorf("rimozione su libretto ignoto: %v", err)
	}
}
