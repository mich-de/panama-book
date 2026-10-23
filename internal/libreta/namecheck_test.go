package libreta

import (
	"testing"

	"scanlibretto/internal/mrz"
)

func TestNamesDifferIgnoraOrdineEAccenti(t *testing.T) {
	cases := []struct {
		printed, sur, giv string
		want              bool
	}{
		{"MARIO ROSSI", "ROSSI", "MARIO", false}, // ordine Panama: nomi prima del cognome
		{"ROSSI MARIO", "ROSSI", "MARIO", false}, // ordine ICAO
		{"JOSE  MARIA GOMEZ", "GOMEZ", "JOSE MARIA", false},
		{"JOSÉ NUÑEZ", "NUNEZ", "JOSE", false},  // accenti ignorati
		{"MARIO R0SSI", "ROSSI", "MARIO", true}, // una cifra al posto della lettera: parole diverse
		{"MARIO ROSSI", "ROSSI", "MARIA", true}, // una lettera diversa
		{"MARIO", "ROSSI", "MARIO", true},       // parola mancante
		{"MARIO ROSSI BIANCHI", "ROSSI", "MARIO", true},
	}
	for _, c := range cases {
		if got := namesDiffer(c.printed, c.sur, c.giv); got != c.want {
			t.Errorf("namesDiffer(%q, %q, %q) = %v, atteso %v", c.printed, c.sur, c.giv, got, c.want)
		}
	}
}

func TestMRZNamesTruncated(t *testing.T) {
	short := "IPPANROSSI<<MARIO" + "<<<<<<<<<<<<<<<<<<<<<<<<<<<"
	long := "IPPAN" + "ABCDEFGHIJKLMNOPQRSTU<<VWXYZABCDEFGHIJK"
	if len(short) != 44 || len(long) != 44 {
		t.Fatalf("righe di prova non lunghe 44: %d, %d", len(short), len(long))
	}
	if mrzNamesTruncated(short) {
		t.Error("nome corto riconosciuto come troncato")
	}
	if !mrzNamesTruncated(long) {
		t.Error("campo nomi tutto occupato non riconosciuto come troncato")
	}
	if mrzNamesTruncated("") {
		t.Error("riga assente riconosciuta come troncata")
	}
}

// resultWithName prepara un Result con MRZ ROSSI/MARIO e il nome stampato dato, con la confidenza data.
func resultWithName(printed string, conf float64) *Result {
	r := &Result{Fields: map[string]string{}, MRZ: &mrz.Data{
		Surname: "ROSSI", GivenNames: "MARIO",
		Line1: "IPPANROSSI<<MARIO" + "<<<<<<<<<<<<<<<<<<<<<<<<<<<",
	}}
	vizSetPrintedName(r, printed, conf)
	return r
}

func hasNameWarning(r *Result) bool {
	for _, w := range r.Warnings {
		if w == warnNameDiffers {
			return true
		}
	}
	return false
}

func TestCheckPrintedName(t *testing.T) {
	// nome stampato diverso, lettura sicura: avviso
	if r := resultWithName("MARIO R0SSI", 0.95); !func() bool { checkPrintedName(r); return hasNameWarning(r) }() {
		t.Error("nome diverso con confidenza alta: avviso atteso")
	}
	// nome stampato uguale (ordine Panama): nessun avviso
	r := resultWithName("MARIO ROSSI", 0.95)
	checkPrintedName(r)
	if hasNameWarning(r) {
		t.Error("nome uguale: nessun avviso atteso")
	}
	// lettura debole: nessun avviso anche se diverso
	r = resultWithName("MARIO R0SSI", printedNameMinConf-0.05)
	checkPrintedName(r)
	if hasNameWarning(r) {
		t.Error("confidenza bassa: nessun avviso atteso")
	}
	// nome non letto
	r = &Result{Fields: map[string]string{}, MRZ: &mrz.Data{Surname: "ROSSI", GivenNames: "MARIO"}}
	vizSetPrintedName(r, "", 0)
	checkPrintedName(r)
	if hasNameWarning(r) {
		t.Error("nome non letto: nessun avviso atteso")
	}
	// MRZ assente: nessun avviso e nessun panico
	r = &Result{Fields: map[string]string{}}
	checkPrintedName(r)
	if len(r.Warnings) != 0 {
		t.Errorf("MRZ assente: nessun avviso atteso, trovati %v", r.Warnings)
	}
	// campo nomi pieno: possibile troncamento, nessun avviso
	r = resultWithName("MARIO R0SSI", 0.95)
	r.MRZ.Line1 = "IPPAN" + "ABCDEFGHIJKLMNOPQRSTU<<VWXYZABCDEFGHIJK"
	checkPrintedName(r)
	if hasNameWarning(r) {
		t.Error("nomi possibilmente troncati: nessun avviso atteso")
	}
}
