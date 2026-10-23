package libreta

import (
	"regexp"
	"strings"
	"testing"

	"scanlibretto/internal/mrz"
)

func TestVIZNationalityTable(t *testing.T) {
	reCode := regexp.MustCompile(`^[A-Z]{3}$`)
	reAdj := regexp.MustCompile(`^[A-Z][A-Z -]+$`)
	if len(vizNationalities) < 190 {
		t.Errorf("tabella incompleta: %d codici", len(vizNationalities))
	}
	for code, adjs := range vizNationalities {
		if !reCode.MatchString(code) || len(adjs) == 0 {
			t.Errorf("voce non valida: %q %v", code, adjs)
		}
		for _, a := range adjs {
			if !reAdj.MatchString(a) {
				t.Errorf("aggettivo non valido per %s: %q", code, a)
			}
		}
	}
	// i paesi frequenti tra i marittimi ci sono tutti
	for _, code := range []string{"PAN", "ITA", "PHL", "IND", "UKR", "MUS", "TUN", "IDN", "ROU", "HRV", "MNE", "GBR", "USA", "ZAF", "LKA"} {
		if len(vizNationalities[code]) == 0 {
			t.Errorf("manca il codice %s", code)
		}
	}
	// aggettivo -> codice, derivato dalla tabella
	for adj, code := range map[string]string{"PANAMANIAN": "PAN", "MAURITIAN": "MUS", "FILIPINO": "PHL", "PHILIPPINE": "PHL", "SRI LANKAN": "LKA", "BURMESE": "MMR"} {
		if vizDemonyms[adj] != code {
			t.Errorf("vizDemonyms[%s] = %q, atteso %s", adj, vizDemonyms[adj], code)
		}
	}
}

func TestVIZExpectedNationality(t *testing.T) {
	// lettura storpiata ma vicina all'aggettivo atteso per il codice MRZ
	if got, ok := vizExpectedNationality("MUS", []string{"MAIJRTT AN"}); !ok || got != "MAURITIAN" {
		t.Errorf("lettura vicina: %q %v", got, ok)
	}
	// tra piu' letture vince la piu' vicina
	if got, ok := vizExpectedNationality("TUN", []string{"XYZ", "TUNIS AN", "TUNISPAN"}); !ok || got != "TUNISIAN" {
		t.Errorf("piu' letture: %q %v", got, ok)
	}
	// testo lontano: nessuna sostituzione
	if _, ok := vizExpectedNationality("MUS", []string{"ROBERT ANTONIO MORALE"}); ok {
		t.Error("un testo lontano dall'aggettivo atteso non va sostituito")
	}
	// codice sconosciuto (o vuoto, o '<<<')
	for _, c := range []string{"", "<<<", "ZZZ"} {
		if _, ok := vizExpectedNationality(c, []string{"ITALIAN"}); ok {
			t.Errorf("codice %q sconosciuto", c)
		}
	}
	// piu' aggettivi per lo stesso paese
	if got, ok := vizExpectedNationality("PHL", []string{"PHILIPPINE"}); !ok || got != "PHILIPPINE" {
		t.Errorf("variante ammessa: %q %v", got, ok)
	}
}

func TestVIZFinalizeNationalityFromMRZ(t *testing.T) {
	mk := func(nat string) *Result {
		f := vizFullFields()
		f["nacionalidad"] = nat
		return &Result{MRZ: &mrz.Data{Nationality: "MUS", DocNumber: "PA0123401"}, Fields: f}
	}
	r := mk("MAIJRTT AN")
	finalizeVIZ(r)
	if r.Fields["nacionalidad"] != "MAURITIAN" {
		t.Errorf("aggettivo non riallineato al codice MRZ: %q", r.Fields["nacionalidad"])
	}
	// lettura lontana: resta il testo letto e il campo e' incerto
	r = mk("ROBERT ANTONIO")
	vizSetReadings(r, vizReadings{"nacionalidad": {cand(0, "nacionalidad", "ROBERT ANTONIO")}})
	finalizeVIZ(r)
	if r.Fields["nacionalidad"] != "ROBERT ANTONIO" {
		t.Errorf("testo lontano sostituito: %q", r.Fields["nacionalidad"])
	}
	if !strings.Contains(strings.Join(r.Warnings, "|"), "campo incerto: nacionalidad") {
		t.Errorf("campo incerto non segnalato: %v", r.Warnings)
	}
}

func TestVIZFinalizeUncertainDatesOnlyAfterFix(t *testing.T) {
	// una scadenza letta male e poi corretta dall'MRZ non deve lasciare l'avviso "scadenza non successiva all'emissione"
	m := vizMRZData()
	m.Checks = mrz.Checks{DocNumber: true, Birth: true, Expiry: true, Optional: true, Composite: true}
	f := vizFullFields()
	f["data_scadenza"] = "04-03-2021" // letta male: anteriore all'emissione
	r := &Result{MRZ: m, Fields: f}
	finalizeVIZ(r)
	joined := strings.Join(r.Warnings, "|")
	if strings.Contains(joined, "non successiva") {
		t.Errorf("incoerenza spuria dopo la correzione con l'MRZ: %v", r.Warnings)
	}
	if !strings.Contains(joined, "data di scadenza stampata diversa dall'MRZ") {
		t.Errorf("la differenza con l'MRZ va comunque segnalata: %v", r.Warnings)
	}
	if r.Fields["data_scadenza"] != "04-03-2031" {
		t.Errorf("scadenza non corretta: %q", r.Fields["data_scadenza"])
	}
}

func TestVIZTextVoteDroppedGlyphs(t *testing.T) {
	// I stretta persa dall'OCR: apostrofo o spazio al suo posto; gli apostrofi veri ("D'") restano
	vals := []string{"MAURIT US", "MAURIT'US", "MAURIT US", "MAURIT'US", "MAURITIUS", "MAURMT S"}
	got, _, ok := vizTextVote(vals, ones(len(vals)))
	if !ok || got != "MAURITIUS" {
		t.Errorf("glifo perso: %q", got)
	}
	if got := vizInnerApostrophe("D'IVOIRE"); got != "D'IVOIRE" {
		t.Errorf("apostrofo vero alterato: %q", got)
	}
	if got := vizInnerApostrophe("MAURIT'US"); got != "MAURITIUS" {
		t.Errorf("apostrofo interno: %q", got)
	}
	// lo spazio vero, presente in tutte le letture, resta
	vals = []string{"LA PAZ", "LA PAZ", "LA PAZ", "LA PAZ"}
	if got, _, _ := vizTextVote(vals, ones(len(vals))); got != "LA PAZ" {
		t.Errorf("spazio vero perso: %q", got)
	}
	// E persa come spazio in quasi tutte le varianti, letta in una sola
	vals = []string{"ST WARD", "ST WARD", "ST WARD", "STEWARD", "S WARD"}
	if got, _, _ := vizTextVote(vals, ones(len(vals))); got != "STEWARD" {
		t.Errorf("lettera contro spazio: %q", got)
	}
}

func TestVIZFinalizeNationalityNameLineAndFill(t *testing.T) {
	m := &mrz.Data{Nationality: "DEU", Surname: "ROSSI", GivenNames: "MARIA ELENA", Issuer: "PAN", DocNumber: "PA0123401"}
	// il ritaglio comprende la riga del nome: va scartata e l'aggettivo ricavato dal codice MRZ, segnalando l'incertezza
	f := vizFullFields()
	f["nacionalidad"] = "MARIA ELENA ROSSI"
	r := &Result{MRZ: m, Fields: f}
	finalizeVIZ(r)
	if r.Fields["nacionalidad"] != "GERMAN" {
		t.Errorf("riga del nome non scartata: %q", r.Fields["nacionalidad"])
	}
	if !strings.Contains(strings.Join(r.Warnings, "|"), "campo incerto: nacionalidad") {
		t.Errorf("aggettivo ricavato dall'MRZ non segnalato: %v", r.Warnings)
	}
	// il nome con una lettera storpiata e' ancora il nome
	if !vizIsMRZName("MARIA ELEMA ROSSI", m) || vizIsMRZName("GERMAN", m) {
		t.Error("vizIsMRZName")
	}
}

func TestVIZFinalizePaeseFromMRZIssuer(t *testing.T) {
	m := &mrz.Data{Issuer: "PAN", Nationality: "ITA", DocNumber: "PA0123401"}
	f := vizFullFields()
	delete(f, "paese")
	r := &Result{MRZ: m, Fields: f}
	finalizeVIZ(r)
	if r.Fields["paese"] != "PAN" {
		t.Errorf("paese non ricavato dall'emittente MRZ: %q", r.Fields["paese"])
	}
	// il rumore fuori vocabolario non vince
	b := vizBands[vizBandIndex("tipo")]
	out := &Result{Fields: map[string]string{}}
	vizVote(vizBand{key: "paese", kind: "short"}, []vizCand{cand(0, "paese", "DEL"), cand(1, "paese", "GUE")}, func(int) float64 { return 1 }, out)
	_ = b
	if out.Fields["paese"] != "" {
		t.Errorf("paese = %q: il rumore non deve vincere", out.Fields["paese"])
	}
}

func TestVIZIdentLoose(t *testing.T) {
	for in, want := range map[string]string{"C1ABC2DEF": "C1ABC2DEF", "N123456": "N123456", "c1abc2def.": "C1ABC2DEF", "ABCDEF": "", "123": "", "123456789012345": ""} {
		got, ok := vizIdentLoose(in)
		if (want == "") == ok || got != want {
			t.Errorf("vizIdentLoose(%q) = %q %v", in, got, ok)
		}
	}
}
