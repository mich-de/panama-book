package filename

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanCaratteriSpeciali(t *testing.T) {
	casi := map[string]string{
		"JOSE<MARIA":          "JOSE MARIA",
		"NUÑEZ":               "NUNEZ",
		"O'BRIEN":             "O BRIEN",
		"DE LA CRUZ-PEÑA":     "DE LA CRUZ PENA",
		`A/B\C:D*E?F"G<H>I|J`: "A B C D E F G H I J",
		"  MARÍA   JOSÉ  ":    "MARIA JOSE",
		"STRAßE":              "STRASSE",
		"ΑΒΓ":                 "",
		"CON.":                "CON",
	}
	for in, want := range casi {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, atteso %q", in, got, want)
		}
	}
}

func TestBase(t *testing.T) {
	got, ok := Base(Parts{Given: "ANA<MARIA", Surname: "GARCÍA<LÓPEZ", Book: "PA0123456"})
	if !ok || got != "ANA MARIA - GARCIA LOPEZ - PA0123456" {
		t.Errorf("Base = %q, %v", got, ok)
	}
	for _, p := range []Parts{{"", "X", "PA1"}, {"X", "", "PA1"}, {"X", "Y", ""}, {"<<", "Y", "PA1"}} {
		if _, ok := Base(p); ok {
			t.Errorf("Base(%+v) doveva rifiutare", p)
		}
	}
	long := strings.Repeat("ABCDEFGHIJ ", 20)
	b, _ := Base(Parts{Given: long, Surname: long, Book: "PA1"})
	if len(b) > 3*maxPartLen+6 || strings.HasSuffix(b, " ") {
		t.Errorf("nome troppo lungo o con spazio finale: %d %q", len(b), b)
	}
	for _, c := range `\/:*?"<>|` {
		if strings.ContainsRune(b, c) {
			t.Errorf("carattere vietato %q in %q", c, b)
		}
	}
}

func TestUnique(t *testing.T) {
	dir := t.TempDir()
	if got := Unique(dir, "A - B - PA1", ".png", ".json"); got != "A - B - PA1" {
		t.Errorf("libero: %q", got)
	}
	_ = os.WriteFile(filepath.Join(dir, "A - B - PA1.json"), nil, 0o600)
	if got := Unique(dir, "A - B - PA1", ".png", ".json"); got != "A - B - PA1 (2)" {
		t.Errorf("con json esistente: %q", got)
	}
	_ = os.WriteFile(filepath.Join(dir, "A - B - PA1 (2).png"), nil, 0o600)
	if got := Unique(dir, "A - B - PA1", ".png", ".json"); got != "A - B - PA1 (3)" {
		t.Errorf("terzo: %q", got)
	}
}
