package main

import (
	"strings"
	"testing"

	"scanlibretto/internal/memoria"
	"scanlibretto/internal/mrz"
)

func TestParseExtractAliasCertificate(t *testing.T) {
	o, err := parseExtract([]byte(`{"fields": {"endorsement": "123456789", "type": "SI"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if v := readValues(o)["certificate"]; v != "123456789" {
		t.Errorf("alias: %q", v)
	}
	o, _ = parseExtract([]byte(`avviso\n{"fields": {"certificate": "111", "endorsement": "222"}}`))
	if v := readValues(o)["certificate"]; v != "111" {
		t.Errorf("la chiave nuova deve prevalere: %q", v)
	}
	if _, err := parseExtract([]byte("non json")); err == nil {
		t.Error("JSON rotto accettato")
	}
	if _, err := parseExtract([]byte(`{"schema_version": 1}`)); err == nil {
		t.Error("senza campi accettato")
	}
}

func TestReadValuesDaMRZ(t *testing.T) {
	o := &extractOut{Fields: map[string]string{}, MRZ: &mrz.Data{Surname: "ROSSI", GivenNames: "MARIO", Sex: "M"}}
	v := readValues(o)
	if v["surname"] != "ROSSI" || v["given_names"] != "MARIO" || v["sex"] != "M" {
		t.Errorf("valori dall'MRZ: %v", v)
	}
}

func TestAvvisiTradottiInItaliano(t *testing.T) {
	codes := []string{"MRZ_NOT_FOUND", "MRZ_CHECK_DIGITS_INVALID", "PRINTED_FIELDS_NOT_READ", "MRZ_LINE1_UNCERTAIN", "MRZ_FIELD_UNCERTAIN",
		"UNCERTAIN_FIELD", "PRINTED_FIELD_DIFFERS_FROM_MRZ", "DATE_OF_EXPIRY_NOT_AFTER_DATE_OF_ISSUE", "DATE_OF_BIRTH_NOT_BEFORE_DATE_OF_ISSUE", "UNKNOWN_WARNING", "CODICE_NUOVO"}
	for _, c := range codes {
		v := warnText(warning{Code: c, Field: "capacity", Fields: []string{"certificate"}})
		if v.Text == "" || strings.ContainsAny(v.Text, "_") && c != "CODICE_NUOVO" {
			t.Errorf("%s: testo %q", c, v.Text)
		}
		if v.Severity == "" {
			t.Errorf("%s: gravita' mancante", c)
		}
	}
	if v := warnText(warning{Code: "PRINTED_FIELDS_NOT_READ", Fields: []string{"certificate"}}); !strings.Contains(v.Text, "Certificate") {
		t.Errorf("etichetta stampata mancante: %q", v.Text)
	}
	if warnText(warning{Code: "MRZ_NOT_FOUND"}).Severity != "error" {
		t.Error("MRZ_NOT_FOUND e' un errore")
	}
}

func TestStatoDeiCampi(t *testing.T) {
	m := &mrz.Data{DocNumber: "P1234567<", BirthDate: "1990-01-01", Checks: mrz.Checks{DocNumber: true, Birth: true, Expiry: true, Optional: true, Composite: true}}
	o := &extractOut{
		MRZ:    m,
		Fields: map[string]string{"type": "SI", "capacity": "X", "date_of_birth": "31-02-1990", "level": "SUPPORT", "regional": "PANAMA", "sex": "M"},
		Warnings: []warning{{Code: "UNCERTAIN_FIELD", Field: "capacity"}, {Code: "PRINTED_FIELDS_NOT_READ", Fields: []string{"certificate"}},
			{Code: "MRZ_FIELD_UNCERTAIN", Field: "sex"}},
	}
	mem, _ := memoria.Apri(t.TempDir() + "/m.json")
	d := newDoc("1", "a.png", nil, o, mem)
	v := d.view(mem, false)
	st := map[string]FieldView{}
	for _, f := range v.Fields {
		st[f.Key] = f
	}
	want := map[string]string{"type": "ok", "capacity": "check", "date_of_birth": "check", "level": "ok", "certificate": "empty", "sex": "check", "place_of_birth": "empty"}
	for k, w := range want {
		if st[k].Status != w {
			t.Errorf("%s: stato %q, atteso %q (%s)", k, st[k].Status, w, st[k].Hint)
		}
	}
	if !strings.Contains(st["date_of_birth"].Hint, "inesistente") {
		t.Errorf("data impossibile non spiegata: %q", st["date_of_birth"].Hint)
	}
	if st["certificate"].StatusText != "Non letto" {
		t.Errorf("campo assente non deve sembrare un errore: %q", st["certificate"].StatusText)
	}
	if st["surname"].Source == "" || st["type"].Source != "" {
		t.Error("l'origine MRZ va indicata solo sui campi MRZ")
	}
	if len(v.MRZ.Checks) != 5 || !v.MRZ.Valid {
		t.Errorf("MRZ: %+v", v.MRZ)
	}
}
