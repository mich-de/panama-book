package verify

import (
	"strings"
	"testing"

	"scanlibretto/internal/outsum"
)

func TestCompareStati(t *testing.T) {
	truth := goodFields()
	got := with(truth,
		"place_of_birth", "ROMX ITA", // sbagliato
		"capacity", "-", // mancante
		"level", "-", // la verita' sotto e' vuota: non stampato
		"certificate", "999999999", // la verita' sotto e' vuota ma il programma ha letto: in piu'
	)
	tr := truthOf(with(truth, "level", "", "certificate", ""))
	rs := Compare(summary(got), tr)
	if len(rs) != outsum.NFields {
		t.Fatalf("attesi %d esiti, trovati %d", outsum.NFields, len(rs))
	}
	for i, k := range outsum.FieldKeys {
		if rs[i].Field != k {
			t.Errorf("esito %d e' %s, atteso %s (ordine di outsum.FieldKeys)", i, rs[i].Field, k)
		}
	}
	want := map[string]Status{
		"type": StatusOK, "place_of_birth": StatusWrong, "capacity": StatusMissing,
		"level": StatusNotPrinted, "certificate": StatusExtra, "date_of_birth": StatusOK,
	}
	for f, st := range want {
		if got := statusOf(rs, f).Status; got != st {
			t.Errorf("%s: %s, atteso %s", f, got, st)
		}
	}
	c := Count(rs)
	if c.OK != 10 || c.Wrong != 1 || c.Missing != 1 || c.NotPrinted != 1 || c.Extra != 1 || c.WithTruth() != 12 {
		t.Errorf("conteggi: %+v", c)
	}
}

func TestCompareNonStampatoSeVuotoEVuoto(t *testing.T) {
	tr := truthOf(with(goodFields(), "level", ""))
	rs := Compare(summary(with(goodFields(), "level", "-")), tr)
	if got := statusOf(rs, "level").Status; got != StatusNotPrinted {
		t.Errorf("level: %s, atteso NON_STAMPATO", got)
	}
}

func TestStatusString(t *testing.T) {
	want := map[Status]string{StatusOK: "OK", StatusWrong: "SBAGLIATO", StatusMissing: "MANCANTE", StatusNotPrinted: "NON_STAMPATO", StatusExtra: "IN_PIU"}
	for s, w := range want {
		if s.String() != w {
			t.Errorf("Status(%d) = %q, atteso %q", s, s.String(), w)
		}
	}
}

func TestDiffFamiglie(t *testing.T) {
	cases := []struct {
		name, field, got, want string
		kind                   ErrKind
		detail                 string // sottostringa attesa nel dettaglio ("" = non controllato)
	}{
		{"troncato", "capacity", "SAM", "SAMPLE TITLE II/9", ErrTrunc, "3 caratteri invece di 14"},
		{"troncato con parole", "capacity", "SAMPLE", "SAMPLE TITLE II/9", ErrTrunc, "1 parola invece di 3"},
		{"coda in piu'", "place_of_birth", "ROMA ITA XX", "ROMA ITA", ErrTail, "2 caratteri in piu'"},
		{"una sostituzione", "id_number", "K12S456", "K123456", ErrSubst, "1 carattere sostituito"},
		{"due sostituzioni", "id_number", "K12SS56", "K123456", ErrSubst, "2 caratteri sostituiti"},
		{"scambio", "id_number", "K123546", "K123456", ErrSwap, ""},
		{"non correlato", "capacity", "HOTEL STAFF", "SAMPLE TITLE II/9", ErrUnrelated, ""},
		{"coda di una lettera", "capacity", "AABBCCDDEEFFGGHHZ", "AABBCCDDEEFFGGHH", ErrTail, "1 carattere in piu"},
		{"lettere doppie", "capacity", "SAAMPLE TTITLE II/9", "SAMPLE TITLE II/9", ErrLength, "differenze"},
		{"solo simboli", "capacity", "---", "SAMPLE", ErrUnrelated, "nessuna lettera"},
		{"data: anno", "date_of_issue", "05-03-2023", "05-03-2026", ErrDate, "anno"},
		{"data: giorno e mese scambiati", "date_of_issue", "03-05-2026", "05-03-2026", ErrDate, "scambiati"},
		{"data: non valida", "date_of_issue", "XX-03-2026", "05-03-2026", ErrDate, "non e' una data valida"},
		{"data: mese", "date_of_birth", "01-02-1990", "01-01-1990", ErrDate, "mese"},
	}
	for _, c := range cases {
		cl := Diff(c.field, c.got, c.want)
		if cl.Kind != c.kind {
			t.Errorf("%s: famiglia %q, attesa %q (dettaglio %q)", c.name, cl.Kind, c.kind, cl.Detail)
			continue
		}
		if c.detail != "" && !strings.Contains(cl.Detail, c.detail) {
			t.Errorf("%s: dettaglio %q, atteso che contenga %q", c.name, cl.Detail, c.detail)
		}
	}
	if cl := Diff("capacity", "same value", "SAME  VALUE"); cl.Kind != "" {
		t.Errorf("valori uguali: famiglia %q, attesa vuota", cl.Kind)
	}
}

func TestClassificaTestoDellaRigaDelNome(t *testing.T) {
	// con il nome letto dall'MRZ
	got := with(goodFields(), "nationality", "ROSSI MARIO LUIGI")
	rs := Compare(summary(got), truthOf(goodFields()))
	if r := statusOf(rs, "nationality"); r.Status != StatusWrong || r.Class.Kind != ErrNameRow {
		t.Errorf("nationality con il testo della riga del nome: %+v", r)
	}
	// parola storpiata dall'OCR (una lettera su cinque)
	got = with(goodFields(), "nationality", "ROSSL")
	if r := statusOf(Compare(summary(got), truthOf(goodFields())), "nationality"); r.Class.Kind != ErrNameRow {
		t.Errorf("nome storpiato: %+v", r)
	}
	// senza MRZ: il nome arriva dalla verita' (_riga_nome)
	g := with(goodFields(), "nationality", "ROSSI MARIO", "surname", "-", "given_names", "-")
	tr := truthOf(goodFields())
	tr.NameRow = "ROSSI MARIO LUIGI"
	if r := statusOf(Compare(noMRZ(g), tr), "nationality"); r.Class.Kind != ErrNameRow {
		t.Errorf("senza MRZ con _riga_nome: %+v", r)
	}
	// una verita' che contiene essa stessa la parola non e' una lettura della riga del nome
	if r := Diff("place_of_birth", "SAN ROSSI", "SAN ROSSINO"); r.Kind == ErrNameRow {
		t.Errorf("senza contesto non si deve parlare di riga del nome: %+v", r)
	}
}

func TestClassificaValoreDiUnAltroCampo(t *testing.T) {
	got := with(goodFields(), "place_of_birth", "ITALIAN")
	r := statusOf(Compare(summary(got), truthOf(goodFields())), "place_of_birth")
	if r.Status != StatusWrong || r.Class.Kind != ErrOtherField || r.Class.Detail != "nationality" {
		t.Errorf("place_of_birth con il valore di nationality: %+v", r)
	}
	// una data nell'altro campo data
	got = with(goodFields(), "date_of_birth", "05-03-2026")
	r = statusOf(Compare(summary(got), truthOf(goodFields())), "date_of_birth")
	if r.Class.Kind != ErrDate {
		t.Errorf("le date si classificano come date: %+v", r)
	}
}

func TestMRZFix(t *testing.T) {
	truth := goodFields()
	tr := truthOf(truth)

	// data di nascita sbagliata: l'MRZ valido la darebbe giusta
	r := statusOf(Compare(summary(with(truth, "date_of_birth", "07-01-1990")), tr), "date_of_birth")
	if r.Status != StatusWrong || r.MRZ != MRZFixes {
		t.Errorf("nascita sbagliata con MRZ valido: %+v", r)
	}
	// stesso errore ma il check digit della nascita non torna: l'MRZ non e' affidabile
	s := summary(with(truth, "date_of_birth", "07-01-1990"))
	s.Checks[1] = false
	if r := statusOf(Compare(s, tr), "date_of_birth"); r.MRZ != MRZAbsent {
		t.Errorf("check digit della nascita non valido: %+v", r)
	}
	// senza MRZ
	if r := statusOf(Compare(noMRZ(with(truth, "date_of_birth", "07-01-1990")), tr), "date_of_birth"); r.MRZ != MRZAbsent {
		t.Errorf("senza MRZ: %+v", r)
	}
	// tipo e paese: costanti della riga 1 dell'MRZ (IS PAN)
	if r := statusOf(Compare(summary(with(truth, "type", "S1", "country_code", "PAH")), tr), "type"); r.Status != StatusWrong || r.MRZ != MRZFixes {
		t.Errorf("tipo sbagliato con MRZ: %+v", r)
	}
	if r := statusOf(Compare(noMRZ(with(truth, "country_code", "-")), tr), "country_code"); r.Status != StatusMissing || r.MRZ != MRZAbsent {
		t.Errorf("paese mancante senza MRZ: %+v", r)
	}
	// campo che nell'MRZ non c'e'
	if r := statusOf(Compare(summary(with(truth, "capacity", "MAS")), tr), "capacity"); r.Status != StatusWrong || r.MRZ != MRZNA {
		t.Errorf("capacity non e' nell'MRZ: %+v", r)
	}
	// numero libretto mancante: l'MRZ lo da'
	if r := statusOf(Compare(summary(with(truth, "seamans_book", "-")), tr), "seamans_book"); r.Status != StatusMissing || r.MRZ != MRZFixes {
		t.Errorf("numero libretto mancante: %+v", r)
	}
	// nazionalita' sbagliata: il codice ISO la ricostruisce con la tabella
	if r := statusOf(Compare(summary(with(truth, "nationality", "ITALLAN")), tr), "nationality"); r.MRZ != MRZFixes {
		t.Errorf("nazionalita' con codice ISO: %+v", r)
	}
	// ma solo se il check digit finale e' valido
	s = summary(with(truth, "nationality", "ITALLAN"))
	s.Checks[4] = false
	if r := statusOf(Compare(s, tr), "nationality"); r.MRZ != MRZAbsent {
		t.Errorf("nazionalita' con check digit finale non valido: %+v", r)
	}
	// regionale: l'MRZ ha solo il prefisso ("PANAMA" per "PANAMA DR")
	if r := statusOf(Compare(summary(with(truth, "regional", "PANAMA D")), tr), "regional"); r.Status != StatusWrong || r.MRZ != MRZNoFix {
		t.Errorf("regionale (solo prefisso nell'MRZ): %+v", r)
	}
	// regionale che nell'MRZ e' uguale alla verita' (il campo opzionale e' il nome intero)
	t2 := with(truth, "regional", "NAPOLI", "regional_mrz", "NAPOLI")
	if r := statusOf(Compare(summary(with(t2, "regional", "NAPOLX")), truthOf(t2)), "regional"); r.MRZ != MRZFixes {
		t.Errorf("regionale uguale all'MRZ: %+v", r)
	}
}

func TestIlReportNonContieneValori(t *testing.T) {
	const (
		secretGot  = "ZQXJGOT7"
		secretWant = "ZQXJWANT9"
	)
	truth := with(goodFields(), "capacity", secretWant, "id_number", secretWant, "nationality", "ZQXJLAND")
	got := with(truth, "capacity", secretGot, "id_number", "-", "place_of_birth", secretGot, "nationality", secretWant)
	res := Verify("immagine-sintetica", summary(got), func() *Truth { t := truthOf(truth); t.NameRow = secretWant; return &t }())
	var b strings.Builder
	WriteResult(&b, res)
	WriteDir(&b, DirReport{Items: []Result{res}}, false)
	WriteDir(&b, DirReport{Items: []Result{res}}, true)
	out := b.String()
	for _, secret := range []string{"ZQXJ", "GOT7", "WANT9", "ROSSI", "ITALIAN", "ROMA", "1234567"} {
		if strings.Contains(out, secret) {
			t.Errorf("il report contiene un valore (%q):\n%s", secret, out)
		}
	}
	if !strings.Contains(out, "SBAGLIATO") || !strings.Contains(out, "MANCANTE") {
		t.Errorf("il report non riporta gli esiti:\n%s", out)
	}
}
