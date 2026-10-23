package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"scanlibretto/internal/libreta"
	"scanlibretto/internal/mrz"
)

// Il JSON in uscita usa solo nomi inglesi, uguali alle etichette stampate sul documento.
func TestToOutputNomiInglesi(t *testing.T) {
	res := &libreta.Result{
		Rotation: 180,
		Fields: map[string]string{
			"tipo": "SI", "paese": "PAN", "nacionalidad": "X", "luogo_nascita": "X", "data_emissione": "X",
			"data_emissione_iso": "X", "data_scadenza": "X", "data_nascita": "X", "livello": "X", "cargo": "X",
			"numero_libretto": "X", "identificazione": "X", "refrendo": "X", "id_marino": "X", "regionale": "X",
			"cognome": "X", "nomi": "X", "nazionalita_iso": "X", "sesso": "X", "data_nascita_mrz": "X",
			"data_scadenza_mrz": "X", "numero_libretto_mrz": "X", "regionale_mrz": "X",
		},
		Warnings:       []string{"MRZ non trovato: i dati stampati non sono verificabili"},
		MRZCorrections: []string{"riga2 pos 14: S→5 (check digit)"},
	}
	out := toOutput(res)
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"type", "country_code", "nationality", "place_of_birth", "date_of_issue", "date_of_issue_iso",
		"date_of_expiry", "date_of_birth", "level", "capacity", "seamans_book", "id_number", "certificate",
		"seafarers_id", "regional", "surname", "given_names", "nationality_iso", "sex", "date_of_birth_mrz",
		"date_of_expiry_mrz", "seamans_book_mrz", "regional_mrz"}
	for _, k := range want {
		if _, ok := out.Fields[k]; !ok {
			t.Errorf("manca il campo %q", k)
		}
	}
	if len(out.Fields) != len(want) {
		t.Errorf("campi in uscita %d, attesi %d: %v", len(out.Fields), len(want), out.Fields)
	}
	for k := range out.Fields {
		if !regexp.MustCompile(`^[a-z_]+$`).MatchString(k) {
			t.Errorf("nome campo non valido: %q", k)
		}
	}
	for _, italiano := range []string{"tipo", "paese", "cargo", "livello", "cognome", "nomi", "sesso", "regionale", "identificazione", "libretto", "nascita", "scadenza", "emissione", "riga"} {
		if strings.Contains(string(b), italiano) {
			t.Errorf("il JSON contiene la parola italiana %q: %s", italiano, b)
		}
	}
	if out.SchemaVersion != 1 || out.RotationDeg != 180 {
		t.Errorf("schema/rotazione: %+v", out)
	}
	if got := out.MRZCorrections[0]; got != "line2 pos 14: S->5 (check digit)" {
		t.Errorf("correzione: %q", got)
	}
}

// Il campo che la zona visiva chiama internamente "refrendo" esce con il nome dell'etichetta stampata, "Nº Título /
// Certificate": `certificate`, non piu' `endorsement`. Vale anche per gli avvisi che lo nominano.
func TestRefrendoEsceComeCertificate(t *testing.T) {
	if got := fieldName("refrendo"); got != "certificate" {
		t.Errorf("fieldName(refrendo) = %q", got)
	}
	out := toOutput(&libreta.Result{Fields: map[string]string{"refrendo": "123456789", "identificazione": "X"}})
	if out.Fields["certificate"] != "123456789" {
		t.Errorf("campo certificate mancante: %v", out.Fields)
	}
	if _, vecchio := out.Fields["endorsement"]; vecchio {
		t.Errorf("la vecchia chiave endorsement e' ancora in uscita: %v", out.Fields)
	}
	w := toWarning("campi stampati non letti: refrendo, regionale")
	if w.Code != "PRINTED_FIELDS_NOT_READ" || strings.Join(w.Fields, ",") != "certificate,regional" {
		t.Errorf("campi non letti: %+v", w)
	}
	if w := toWarning("campo incerto: refrendo (letture scarse o discordanti tra le varianti)"); w.Code != "UNCERTAIN_FIELD" || w.Field != "certificate" {
		t.Errorf("campo incerto: %+v", w)
	}
}

// Gli avvisi della seconda lettura dell'MRZ (OCR contro lettore a template) hanno codici stabili, senza valori.
func TestToWarningMRZIncerto(t *testing.T) {
	casi := map[string]warning{
		"riga 1 MRZ incerta: OCR e lettore a template differiscono": {Code: "MRZ_LINE1_UNCERTAIN"},
		"campo MRZ incerto: nazionalita_iso":                        {Code: "MRZ_FIELD_UNCERTAIN", Field: "nationality_iso"},
		"campo MRZ incerto: sesso":                                  {Code: "MRZ_FIELD_UNCERTAIN", Field: "sex"},
		"nome stampato diverso dall'MRZ":                            {Code: "MRZ_NAME_DIFFERS_FROM_PRINTED"},
		"campo incerto: id_marino (letture scarse)":                 {Code: "UNCERTAIN_FIELD", Field: "seafarers_id"}, // campo stampato: resta com'era
	}
	for testo, want := range casi {
		if got := toWarning(testo); got.Code != want.Code || got.Field != want.Field {
			t.Errorf("%q -> %+v, atteso %+v", testo, got, want)
		}
	}
}

// Le correzioni MRZ escono in inglese, anche le frasi del lettore a template e della seconda lettura.
func TestCorrectionInglese(t *testing.T) {
	casi := map[string]string{
		"riga2 pos 14: S→5 (check digit)":                                                                                   "line2 pos 14: S->5 (check digit)",
		"riga1 pos 22: lettura incerta (O)":                                                                                 "line1 pos 22: uncertain reading (O)",
		"riga2 pos 7: O→0 (vincoli MRZ del lettore a template)":                                                             "line2 pos 7: O->0 (MRZ constraints of the template reader)",
		"riga1 pos 3: W riconosciuta solo da un glifo sintetico (mai vista su una scansione): da verificare":                "line1 pos 3: W recognised only from a synthetic glyph (never seen on a scan): to be checked",
		"riga1: cognome e nomi presi dal lettore a template (OCR e lettore differivano, nessuna cella diversa era incerta)": "line1: surname and given names taken from the template reader (OCR and reader differed, none of the differing cells was uncertain)",
		"riga2: nazionalita' presa dal lettore a template (OCR e lettore differivano, nessuna cella diversa era incerta)":   "line2: nationality taken from the template reader (OCR and reader differed, none of the differing cells was uncertain)",
		"riga2: sesso preso dal lettore a template (OCR e lettore differivano, nessuna cella diversa era incerta)":          "line2: sex taken from the template reader (OCR and reader differed, none of the differing cells was uncertain)",
	}
	for in, want := range casi {
		got := correction(in)
		if got != want {
			t.Errorf("%q ->\n  %q\natteso\n  %q", in, got, want)
		}
		for _, italiano := range []string{"riga", "lettura", "incerta", "vincoli", "lettore", "cognome", "nomi ", "nazionalita", "sesso", "cella", "presa", "preso", "glifo"} {
			if strings.Contains(got, italiano) {
				t.Errorf("la correzione contiene la parola italiana %q: %q", italiano, got)
			}
		}
	}
}

func TestToWarningCodici(t *testing.T) {
	casi := map[string]warning{
		"MRZ non trovato: i dati stampati non sono verificabili":                  {Code: "MRZ_NOT_FOUND"},
		"check digit MRZ non tutti validi: {DocNumber:false}":                     {Code: "MRZ_CHECK_DIGITS_INVALID"},
		"campo incerto: id_marino (letture scarse o discordanti tra le varianti)": {Code: "UNCERTAIN_FIELD", Field: "seafarers_id"},
		"data di nascita stampata diversa dall'MRZ":                               {Code: "PRINTED_FIELD_DIFFERS_FROM_MRZ", Field: "date_of_birth"},
		"regionale stampato diverso dall'MRZ":                                     {Code: "PRINTED_FIELD_DIFFERS_FROM_MRZ", Field: "regional"},
		"nazionalita' stampata diversa dall'MRZ":                                  {Code: "PRINTED_FIELD_DIFFERS_FROM_MRZ", Field: "nationality"},
		"testo mai visto":                                                         {Code: "UNKNOWN_WARNING"},
	}
	for testo, want := range casi {
		got := toWarning(testo)
		if got.Code != want.Code || got.Field != want.Field {
			t.Errorf("%q -> %+v, atteso %+v", testo, got, want)
		}
	}
	got := toWarning("campi stampati non letti: paese, tipo, cargo")
	if got.Code != "PRINTED_FIELDS_NOT_READ" || strings.Join(got.Fields, ",") != "capacity,country_code,type" {
		t.Errorf("campi non letti: %+v", got)
	}
}

// Nei libretti di Panama l'MRZ elenca prima i nomi e poi i cognomi: l'uscita li mette nei campi giusti.
func TestToOutputOrdineNomiPanama(t *testing.T) {
	// riga 1 sintetica: ISPAN + MARIO<ANTONIO<<ROSSI<VERDI; il parser (ICAO) legge cognome = primo gruppo
	d := &mrz.Data{DocCode: "IS", Issuer: "PAN", Surname: "MARIO ANTONIO", GivenNames: "ROSSI VERDI"}
	res := &libreta.Result{MRZ: d, Fields: map[string]string{"cognome": "MARIO ANTONIO", "nomi": "ROSSI VERDI"}}
	out := toOutput(res)
	if out.Fields["given_names"] != "MARIO ANTONIO" || out.Fields["surname"] != "ROSSI VERDI" {
		t.Errorf("campi: surname=%q given_names=%q", out.Fields["surname"], out.Fields["given_names"])
	}
	if out.MRZ.Surname != "ROSSI VERDI" || out.MRZ.GivenNames != "MARIO ANTONIO" {
		t.Errorf("blocco mrz: %+v", out.MRZ)
	}
	if d.Surname != "MARIO ANTONIO" {
		t.Error("il dato interno non va modificato")
	}
	// passaporto comune (codice P<) o un solo gruppo di nomi: nessuna inversione
	for _, m := range []*mrz.Data{
		{DocCode: "P<", Issuer: "PAN", Surname: "ROSSI", GivenNames: "MARIO"},
		{DocCode: "IS", Issuer: "PAN", Surname: "ROSSI"},
	} {
		o := toOutput(&libreta.Result{MRZ: m, Fields: map[string]string{"cognome": m.Surname, "nomi": m.GivenNames}})
		if o.Fields["surname"] != m.Surname || o.MRZ.Surname != m.Surname {
			t.Errorf("non andava invertito: %+v -> %+v", m, o.Fields)
		}
	}
}
