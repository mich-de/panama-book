package verify

import "testing"

func TestNorm(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  Master II/2 ", "MASTERII2"},
		{"1-234-5678", "12345678"},
		{"PANAMÁ", "PANAMA"},
		{"muñoz-núñez", "MUNOZNUNEZ"},
		{"Straße", "STRASSE"},
		{"Æther", "AETHER"},
		{"", ""},
		{"<<--//..", ""},
		{"a b\tc", "ABC"},
	}
	for _, c := range cases {
		if got := Norm(c.in); got != c.want {
			t.Errorf("Norm(%q) = %q, atteso %q", c.in, got, c.want)
		}
	}
}

func TestParseDate(t *testing.T) {
	ok := []string{"09-03-2026", "2026-03-09", "09/03/2026", "09.03.2026", "9-3-2026", " 09-03-2026 ", "2026/3/9"}
	for _, s := range ok {
		d, good := parseDate(s)
		if !good || d != (ymd{2026, 3, 9}) {
			t.Errorf("parseDate(%q) = %v, %v", s, d, good)
		}
	}
	bad := []string{"", "31-02-2026", "2026-13-01", "09-03-26", "a-b-c", "1-2", "09-03-2026-1", "00-01-2026", "29-02-2025", "2026-00-10"}
	for _, s := range bad {
		if _, good := parseDate(s); good {
			t.Errorf("parseDate(%q) dovrebbe fallire", s)
		}
	}
	if _, good := parseDate("29-02-2024"); !good {
		t.Error("29-02-2024 e' una data valida (anno bisestile)")
	}
}

func TestEqual(t *testing.T) {
	cases := []struct {
		field, got, want string
		eq               bool
	}{
		{"capacity", "ordinary  seaman", "ORDINARY SEAMAN", true},
		{"capacity", "MASTER II/2", "MASTER II 2", true},
		{"id_number", "1-234-5678", "12345678", true},
		{"place_of_birth", "SAN JOSÉ", "SAN JOSE", true},
		{"nationality", "ITALIAN", "ITALLAN", false},
		{"capacity", "MAS", "MASTER II/2", false},
		{"id_number", "K12S456", "K123456", false},
		{"date_of_birth", "1990-01-01", "01-01-1990", true},
		{"date_of_issue", "05/03/2026", "05-03-2026", true},
		{"date_of_expiry", "04-03-2031", "04-03-2030", false},
		{"date_of_birth", "03-05-2026", "05-03-2026", false}, // giorno e mese scambiati
		{"date_of_birth", "31-02-1990", "31-02-1990", true},  // non sono date: ripiega sul testo
		{"id_number", "2026-03-09", "09-03-2026", false},     // un campo che non e' una data non si confronta come data
		{"regional", "", "", true},
		{"regional", "PANAMA", "", false},
	}
	for _, c := range cases {
		if got := Equal(c.field, c.got, c.want); got != c.eq {
			t.Errorf("Equal(%s, %q, %q) = %v, atteso %v", c.field, c.got, c.want, got, c.eq)
		}
	}
}
