package mrz

import "testing"

// MRZ sintetico (persona e numeri inventati), check digit calcolati con CheckDigit.
const (
	synL1 = "ISPANROSSI<<MARIO<<<<<<<<<<<<<<<<<<<<<<<<<<<"
	synL2 = "P1234567<1ITA9001011M3012316PANAMA<<<<<<<<44"
)

func TestCheckDigit(t *testing.T) {
	// esempio ufficiale ICAO 9303: numero documento L898902C3 -> 6
	if got := CheckDigit("L898902C3"); got != '6' {
		t.Fatalf("L898902C3: atteso 6, ottenuto %c", got)
	}
	if got := CheckDigit("740812"); got != '2' { // data ICAO
		t.Fatalf("740812: atteso 2, ottenuto %c", got)
	}
}

func TestParseSintetico(t *testing.T) {
	d, err := Parse(synL1, synL2)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Checks.AllOK() {
		t.Fatalf("check digit non validi: %+v", d.Checks)
	}
	want := map[string]string{
		"doc_code": "IS", "issuer": "PAN", "surname": "ROSSI", "given": "MARIO",
		"num": "P1234567", "nat": "ITA", "birth": "1990-01-01", "sex": "M", "expiry": "2030-12-31", "opt": "PANAMA",
	}
	got := map[string]string{
		"doc_code": d.DocCode, "issuer": d.Issuer, "surname": d.Surname, "given": d.GivenNames,
		"num": d.DocNumber, "nat": d.Nationality, "birth": d.BirthDate, "sex": d.Sex, "expiry": d.ExpiryDate, "opt": d.Optional,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: atteso %q, ottenuto %q", k, w, got[k])
		}
	}
}

func TestRepairErroriOCR(t *testing.T) {
	// l'OCR scambia: O al posto di 0 nella data, I al posto di 1, spazi e filler «
	noisy1 := "ISPANROSSI«« MARIO««««««««««««««««««««««««««««"
	noisy2 := "P123456 7<1ITA9OO1O11M3O1231 6PANAMA<<<<<<<<44"
	d, notes, err := Repair(noisy1, noisy2)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Checks.AllOK() {
		t.Fatalf("dopo la riparazione i check dovrebbero tornare: %+v (correzioni: %v)", d.Checks, notes)
	}
	if d.BirthDate != "1990-01-01" || d.ExpiryDate != "2030-12-31" {
		t.Errorf("date errate: %s %s", d.BirthDate, d.ExpiryDate)
	}
	if len(notes) == 0 {
		t.Error("attese correzioni riportate")
	}
}

func TestCheckDigitSegnalaCifraSbagliata(t *testing.T) {
	// una cifra del numero documento errata (3 al posto di 2) non e' un errore tipico dell'OCR:
	// Repair non deve "aggiustarla" di nascosto, il check digit deve restare non valido.
	bad := synL2[:2] + "3" + synL2[3:]
	d, _, err := Repair(synL1, bad)
	if err != nil {
		t.Fatal(err)
	}
	if d.Checks.DocNumber {
		t.Errorf("il numero errato %q risulta valido", d.DocNumber)
	}
}

func TestRiparaCifraConfondibile(t *testing.T) {
	// nella data di nascita 900101 l'OCR legge un 1 come 7 (900701): data valida, ma il check digit non torna.
	// Repair deve recuperare la cifra giusta e dichiararlo.
	bad := synL2[:16] + "7" + synL2[17:]
	d, notes, err := Repair(synL1, bad)
	if err != nil {
		t.Fatal(err)
	}
	if d.BirthDate != "1990-01-01" || !d.Checks.AllOK() {
		t.Fatalf("non recuperata: nascita=%s checks=%+v", d.BirthDate, d.Checks)
	}
	if len(notes) == 0 {
		t.Error("la correzione deve essere riportata")
	}
}

func TestParseRigheCorte(t *testing.T) {
	if _, err := Parse("ISPAN", "P1"); err == nil {
		t.Fatal("attesi errori per righe corte")
	}
}
