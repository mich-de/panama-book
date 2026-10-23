package mrz

import (
	"strings"
	"testing"
	"time"
)

// mkLine2 compone una riga 2 sintetica con tutti i check digit calcolati; optCD, se diverso da 0, sostituisce il
// check digit del campo opzionale (il composito viene ricalcolato di conseguenza).
func mkLine2(optional string, optCD byte) string {
	doc, birth, expiry := "P1234567<", "900101", "301231"
	ocd := optCD
	if ocd == 0 {
		ocd = CheckDigit(optional)
	}
	l := doc + string(CheckDigit(doc)) + "ITA" + birth + string(CheckDigit(birth)) + "M" +
		expiry + string(CheckDigit(expiry)) + optional + string(ocd)
	comp := CheckDigit(l[0:10] + l[13:20] + l[21:43])
	return l + string(comp)
}

// Due sostituzioni diverse (pos 1 e pos 7) fanno entrambe tornare il check digit del numero documento e il
// composito: la correzione e' ambigua e Repair non deve applicarne nessuna.
func TestRepairCorrezioneAmbiguaNonApplicata(t *testing.T) {
	bad := synL2[:1] + "7" + synL2[2:] // P7234567<1...: '7' al posto di '1'
	// precondizione: ciascuna delle due correzioni, da sola, farebbe tornare tutto
	for _, pos := range []int{1, 7} {
		fixed := bad[:pos] + "1" + bad[pos+1:]
		d, err := Parse(synL1, fixed)
		if err != nil || !d.Checks.DocNumber || !d.Checks.Composite {
			t.Fatalf("precondizione del test: la correzione in pos %d dovrebbe validare (%+v, %v)", pos, d.Checks, err)
		}
	}
	d, notes, err := Repair(synL1, bad)
	if err != nil {
		t.Fatal(err)
	}
	if d.Line2 != bad {
		t.Errorf("riga 2 modificata nonostante l'ambiguita': %q", d.Line2)
	}
	if d.Checks.DocNumber {
		t.Error("check del numero documento valido: la correzione ambigua e' stata applicata")
	}
	for _, n := range notes {
		if strings.Contains(n, "check digit") {
			t.Errorf("correzione riportata: %s", n)
		}
	}
}

// Una riga 2 che non ha 44 caratteri non si tocca: correggere O->0 sposterebbe o falserebbe i campi.
func TestNormalizeRiga2LunghezzaDiversaIntatta(t *testing.T) {
	for _, l2 := range []string{
		synL2[:43],                    // troppo corta
		synL2 + "<",                   // troppo lunga
		"P1234567<1ITA9OO1O11M3O1231", // corta e piena di O al posto di 0
	} {
		_, got, notes := Normalize(synL1, l2)
		l2c := strings.ReplaceAll(l2, " ", "")
		if got != l2c {
			t.Errorf("riga di %d car modificata: %q -> %q", len(l2), l2, got)
		}
		for _, n := range notes {
			if strings.HasPrefix(n, "riga2") {
				t.Errorf("correzione sulla riga 2 di lunghezza %d: %s", len(l2), n)
			}
		}
	}
}

// Campo opzionale tutto '<': il check digit puo' essere '<' o '0' (ICAO), non altro.
func TestCampoOpzionaleVuoto(t *testing.T) {
	empty := strings.Repeat("<", 14)
	for _, tc := range []struct {
		cd   byte
		want bool
	}{{'<', true}, {'0', true}, {'1', false}, {'5', false}} {
		d, err := Parse(synL1, mkLine2(empty, tc.cd))
		if err != nil {
			t.Fatal(err)
		}
		if d.Checks.Optional != tc.want {
			t.Errorf("check opzionale %q: atteso %v, ottenuto %v", tc.cd, tc.want, d.Checks.Optional)
		}
		if tc.want && !d.Checks.AllOK() {
			t.Errorf("check opzionale %q: tutti i check dovrebbero tornare: %+v", tc.cd, d.Checks)
		}
		if d.Optional != "" {
			t.Errorf("campo opzionale vuoto atteso, ottenuto %q", d.Optional)
		}
	}
	// un campo non vuoto richiede il check digit vero (un '<' non va bene)
	d, _ := Parse(synL1, mkLine2("PANAMA<<<<<<<<<", '<'))
	if d.Checks.Optional {
		t.Error("campo opzionale non vuoto con check '<': non deve validare")
	}
}

// Il secolo della data di nascita dipende dall'anno corrente: con un orologio fisso il risultato e' deterministico.
func TestDateSecoloConOrologioFisso(t *testing.T) {
	old := now
	t.Cleanup(func() { now = old })
	now = func() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) }
	for _, tc := range []struct {
		yymmdd string
		birth  bool
		want   string
	}{
		{"250101", true, "2025-01-01"}, // passato: 2000
		{"260101", true, "2026-01-01"}, // anno corrente: 2000
		{"270101", true, "1927-01-01"}, // futuro: secolo scorso
		{"990101", true, "1999-01-01"},
		{"270101", false, "2027-01-01"}, // la scadenza e' sempre nel 2000
		{"990101", false, "2099-01-01"},
		{"900230", true, "900230"}, // data impossibile: stringa grezza
		{"9001", true, "9001"},     // lunghezza errata: stringa grezza
	} {
		if got := date(tc.yymmdd, tc.birth); got != tc.want {
			t.Errorf("date(%q, birth=%v) = %q, atteso %q", tc.yymmdd, tc.birth, got, tc.want)
		}
	}
	// spostando l'orologio cambia il secolo di uno stesso anno
	now = func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }
	if got := date("270101", true); got != "2027-01-01" {
		t.Errorf("con orologio 2030: atteso 2027-01-01, ottenuto %q", got)
	}
}
