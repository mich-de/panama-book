package verify

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scanlibretto/internal/outsum"
)

func TestParseTruthPiatta(t *testing.T) {
	m := goodFields()
	b := truthJSON(t, with(m, "certificate", ""), map[string]any{
		"_note":      map[string]string{"certificate": "non stampato su questo titolare"},
		"_tipo":      "pagina",
		"_riga_nome": "ROSSI MARIO LUIGI",
		"altro":      "ignorato",
	})
	tr, err := ParseTruth(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Fields) != outsum.NFields || tr.Fields["type"] != "SI" || tr.Field("certificate") != "" {
		t.Errorf("campi: %v", tr.Fields)
	}
	if tr.Notes["certificate"] == "" || tr.Kind != "pagina" || tr.NameRow != "ROSSI MARIO LUIGI" || len(tr.Absent) != 0 {
		t.Errorf("metadati: %+v", tr)
	}
}

func TestParseTruthDentroFields(t *testing.T) {
	tr, err := ParseTruth([]byte(`{"fields": {"type": "SI", "country_code": "PAN"}, "level": "SUPPORT"}`))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Fields["type"] != "SI" || tr.Fields["country_code"] != "PAN" || tr.Fields["level"] != "SUPPORT" {
		t.Errorf("campi dentro fields e al livello superiore: %v", tr.Fields)
	}
	if len(tr.Absent) != outsum.NFields-3 {
		t.Errorf("chiavi mancanti: %v", tr.Absent)
	}
	if tr.Fields["capacity"] != "" {
		t.Errorf("una chiave assente vale vuoto: %q", tr.Fields["capacity"])
	}
}

func TestParseTruthNullEVuoto(t *testing.T) {
	tr, err := ParseTruth([]byte(`{"type": null, "country_code": ""}`))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Fields["type"] != "" || tr.Fields["country_code"] != "" {
		t.Errorf("null e stringa vuota valgono vuoto: %v", tr.Fields)
	}
	for _, k := range tr.Absent {
		if k == "type" || k == "country_code" {
			t.Errorf("%s e' presente nel file, non assente", k)
		}
	}
}

func TestParseTruthErrori(t *testing.T) {
	for _, in := range []string{``, `non json`, `[1,2]`, `{"type": 5}`, `{"fields": "x"}`, `{"capacity": ["SECRETVALUE"]}`} {
		_, err := ParseTruth([]byte(in))
		if err == nil || !errors.Is(err, ErrBadTruth) {
			t.Errorf("ParseTruth(%q) = %v, atteso ErrBadTruth", in, err)
			continue
		}
		if strings.Contains(err.Error(), "SECRETVALUE") {
			t.Errorf("l'errore contiene un valore: %v", err)
		}
	}
}

func TestParseOutput(t *testing.T) {
	if _, err := ParseOutput(outputJSON(t, goodFields(), true)); err != nil {
		t.Errorf("uscita valida: %v", err)
	}
	for _, in := range []string{``, `non json`, `{"fields": {}}`, `{"schema_version": 1}`, `{"cognome": "X"}`} {
		if _, err := ParseOutput([]byte(in)); err == nil {
			t.Errorf("ParseOutput(%q) dovrebbe fallire", in)
		}
	}
}

func TestVerifyEFailed(t *testing.T) {
	fixedNow(t)
	ok := Verify("a", summary(goodFields()), func() *Truth { t := truthOf(goodFields()); return &t }())
	if ok.Failed(false) || ok.Failed(true) {
		t.Errorf("uscita perfetta: %+v", ok.Counts())
	}
	// un avviso fa fallire solo con strict
	warn := Verify("b", summary(with(goodFields(), "place_of_birth", "ROMA  ITA")), func() *Truth { t := truthOf(with(goodFields(), "place_of_birth", "ROMA  ITA")); return &t }())
	if warn.Failed(false) || !warn.Failed(true) {
		t.Errorf("avviso: Failed(false)=%v Failed(true)=%v", warn.Failed(false), warn.Failed(true))
	}
	// un campo sbagliato fa fallire
	wrong := Verify("c", summary(with(goodFields(), "capacity", "MAS")), func() *Truth { t := truthOf(goodFields()); return &t }())
	if !wrong.Failed(false) {
		t.Error("campo sbagliato: dovrebbe fallire")
	}
	// IN_PIU e INFO non fanno fallire
	extra := Verify("d", noMRZ(with(goodFields(), "date_of_birth_mrz", "-", "date_of_expiry_mrz", "-", "seamans_book_mrz", "-", "regional_mrz", "-")),
		func() *Truth { t := truthOf(with(goodFields(), "certificate", "")); return &t }())
	if extra.Counts().Extra != 1 || extra.Failed(false) {
		t.Errorf("in piu' e note non fanno fallire: %+v failed=%v", extra.Counts(), extra.Failed(false))
	}
	// senza verita': solo coerenza
	bare := Verify("e", summary(with(goodFields(), "type", "IS")), nil)
	if bare.HasTruth || len(bare.Fields) != 0 || !bare.Failed(false) {
		t.Errorf("senza verita': %+v", bare)
	}
}

// writeDir scrive i file di una cartella di prova (nome -> contenuto).
func writeDir(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVerifyDir(t *testing.T) {
	fixedNow(t)
	root := t.TempDir()
	outs, truths := filepath.Join(root, "json"), filepath.Join(root, "verita")
	good := goodFields()
	writeDir(t, outs, map[string][]byte{
		"a.json":   outputJSON(t, good, true),                                   // perfetta
		"b.json":   outputJSON(t, with(good, "capacity", "ORDINARY"), true),     // un campo sbagliato (troncato)
		"c.json":   outputJSON(t, with(good, "level", "-", "type", "-"), false), // senza MRZ, due campi mancanti
		"d.json":   outputJSON(t, good, true),                                   // senza verita'
		"old.json": []byte(`{"tipo": "SI"}`),                                    // schema vecchio: saltato
		"note.txt": []byte("non e' un json"),                                    // ignorato
	})
	writeDir(t, truths, map[string][]byte{
		"a.json": truthJSON(t, good, nil),
		"b.json": truthJSON(t, good, nil),
		"c.json": truthJSON(t, good, nil),
	})
	rep, err := VerifyDir(outs, truths)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Items) != 4 || len(rep.Skipped) != 1 || rep.Skipped[0] != "old.json" {
		t.Fatalf("immagini %d, saltati %v", len(rep.Items), rep.Skipped)
	}
	if len(rep.NoTruth) != 1 || rep.NoTruth[0] != "d" {
		t.Errorf("senza verita': %v", rep.NoTruth)
	}
	names := []string{}
	for _, r := range rep.Items {
		names = append(names, r.Name)
	}
	if strings.Join(names, ",") != "a,b,c,d" {
		t.Errorf("ordine: %v", names)
	}
	if !rep.Failed(false) {
		t.Error("b e c hanno campi sbagliati o mancanti")
	}

	st := rep.Stats()
	if st.Images != 3 || st.NoData != 0 {
		t.Errorf("immagini con verita' leggibile: %d (senza pagina dati %d), attese 3 (0)", st.Images, st.NoData)
	}
	byField := map[string]FieldStats{}
	for _, f := range st.Fields {
		byField[f.Field] = f
	}
	if f := byField["capacity"]; f.OK != 2 || f.Wrong != 1 || f.Kinds[ErrTrunc] != 1 || f.Found != 3 {
		t.Errorf("capacity: %+v", f)
	}
	if f := byField["level"]; f.OK != 2 || f.Missing != 1 || f.Found != 2 {
		t.Errorf("level: %+v", f)
	}
	if f := byField["type"]; f.Missing != 1 || f.WithTruth() != 3 {
		t.Errorf("type: %+v", f)
	}
	if st.Total.OK != 3*14-3 || st.Total.Wrong != 1 || st.Total.Missing != 2 {
		t.Errorf("totali: %+v", st.Total)
	}
	if st.Findings["MRZ_NOT_FOUND"] != 1 {
		t.Errorf("incoerenze: %v", st.Findings)
	}

	// Filter: solo quelle con MRZ valido e con verita'
	with := rep.Filter(func(r Result) bool { return r.HasTruth && r.MRZValid() })
	if len(with.Items) != 2 {
		t.Errorf("con MRZ valido e verita': %d, attese 2", len(with.Items))
	}
}

func TestVerifyDirVeritaRottaEUnErrore(t *testing.T) {
	root := t.TempDir()
	outs, truths := filepath.Join(root, "json"), filepath.Join(root, "verita")
	writeDir(t, outs, map[string][]byte{"a.json": outputJSON(t, goodFields(), true)})
	writeDir(t, truths, map[string][]byte{"a.json": []byte(`{"type": 5}`)})
	if _, err := VerifyDir(outs, truths); !errors.Is(err, ErrBadTruth) {
		t.Errorf("verita' rotta: %v", err)
	}
	if _, err := VerifyDir(filepath.Join(root, "manca"), truths); err == nil {
		t.Error("cartella inesistente: atteso un errore")
	}
}

func TestVerifyDirSenzaCartellaVerita(t *testing.T) {
	fixedNow(t)
	root := t.TempDir()
	writeDir(t, root, map[string][]byte{"a.json": outputJSON(t, goodFields(), true)})
	rep, err := VerifyDir(root, "")
	if err != nil || len(rep.Items) != 1 || rep.Items[0].HasTruth || len(rep.NoTruth) != 0 {
		t.Errorf("solo coerenza: %+v %v", rep, err)
	}
}

func TestVerifyFile(t *testing.T) {
	fixedNow(t)
	root := t.TempDir()
	writeDir(t, root, map[string][]byte{
		"x.json": outputJSON(t, goodFields(), true),
		"t.json": truthJSON(t, goodFields(), nil),
	})
	res, err := VerifyFile(filepath.Join(root, "x.json"), filepath.Join(root, "t.json"))
	if err != nil || !res.HasTruth || res.Name != "x" || res.Failed(true) {
		t.Errorf("%+v %v", res, err)
	}
	if _, err := VerifyFile(filepath.Join(root, "manca.json"), ""); err == nil {
		t.Error("file inesistente")
	}
	if _, err := VerifyFile(filepath.Join(root, "x.json"), filepath.Join(root, "manca.json")); err == nil {
		t.Error("verita' inesistente")
	}
}

func TestRigaDelNomeDellMRZ(t *testing.T) {
	fixedNow(t)
	truth := func(row string) *Truth { t := truthOf(goodFields()); t.NameRow = row; return &t }
	ok := Verify("ok", summary(goodFields()), truth("MARIO LUIGI ROSSI"))
	wrong := Verify("sbagliata", summary(with(goodFields(), "surname", "ROSSL")), truth("MARIO LUIGI ROSSI"))
	missing := Verify("senza MRZ", noMRZ(with(goodFields(), "surname", "-", "given_names", "-")), truth("MARIO LUIGI ROSSI"))
	na := Verify("senza riga", summary(goodFields()), truth(""))
	if ok.Names != NameOK || wrong.Names != NameWrong || missing.Names != NameMissing || na.Names != NameNA {
		t.Fatalf("esiti: %v %v %v %v", ok.Names, wrong.Names, missing.Names, na.Names)
	}
	// un nome sbagliato fa fallire anche se i 14 campi sono giusti; un nome assente no (c'e' gia' MRZ_NOT_FOUND)
	if ok.Failed(false) || !wrong.Failed(false) {
		t.Errorf("Failed: ok=%v sbagliata=%v", ok.Failed(false), wrong.Failed(false))
	}
	if NameOK.String() != "OK" || NameWrong.String() != "SBAGLIATO" || NameMissing.String() != "MANCANTE" || NameNA.String() != "NON_VERIFICABILE" {
		t.Error("etichette di NameStatus")
	}
	st := StatsOf([]Result{ok, wrong, missing, na})
	if st.NamesOK != 1 || st.NamesWrong != 1 || st.NamesMissing != 1 {
		t.Errorf("conteggi nomi: %d %d %d", st.NamesOK, st.NamesWrong, st.NamesMissing)
	}
	var b strings.Builder
	WriteResult(&b, wrong)
	WriteStats(&b, st)
	out := b.String()
	if !strings.Contains(out, "riga del nome") || !strings.Contains(out, "giusti 1, sbagliati 1, senza MRZ 1") {
		t.Errorf("il report non riporta la riga del nome:\n%s", out)
	}
	for _, secret := range []string{"ROSSI", "ROSSL", "MARIO", "LUIGI"} {
		if strings.Contains(out, secret) {
			t.Errorf("il report contiene un nome (%q):\n%s", secret, out)
		}
	}
}

func TestStatsPaginaDatiAssenteEMRZAssente(t *testing.T) {
	fixedNow(t)
	empty := Truth{Fields: map[string]string{}}
	for _, k := range outsum.FieldKeys {
		empty.Fields[k] = ""
	}
	// copertina: nessun campo stampato, il programma non legge nulla
	cover := Verify("copertina", noMRZ(map[string]string{}), &empty)
	// pagina leggibile, senza MRZ, con la nascita e il numero libretto mancanti e il cargo sbagliato
	got := with(goodFields(), "date_of_birth", "-", "seamans_book", "-", "capacity", "SAMPLE", "date_of_birth_mrz", "-", "seamans_book_mrz", "-")
	page := Verify("pagina", noMRZ(got), func() *Truth { t := truthOf(goodFields()); return &t }())
	if cover.Readable() || !page.Readable() {
		t.Fatalf("Readable: copertina %v, pagina %v", cover.Readable(), page.Readable())
	}
	st := StatsOf([]Result{cover, page})
	if st.Images != 1 || st.NoData != 1 {
		t.Errorf("immagini %d, senza pagina dati %d", st.Images, st.NoData)
	}
	if st.Total.NotPrinted != outsum.NFields {
		t.Errorf("la copertina ha %d campi non stampati, attesi %d", st.Total.NotPrinted, outsum.NFields)
	}
	by := map[string]FieldStats{}
	for _, f := range st.Fields {
		by[f.Field] = f
	}
	if f := by["date_of_birth"]; f.Missing != 1 || f.MRZAbsent != 1 {
		t.Errorf("nascita mancante senza MRZ: %+v", f)
	}
	if f := by["seamans_book"]; f.MRZAbsent != 1 {
		t.Errorf("numero libretto senza MRZ: %+v", f)
	}
	if f := by["capacity"]; f.Wrong != 1 || f.MRZAbsent != 0 || f.MRZFixes != 0 || f.MRZNoFix != 0 {
		t.Errorf("il cargo non e' nell'MRZ: %+v", f)
	}
	var b strings.Builder
	WriteStats(&b, st)
	if !strings.Contains(b.String(), "1 immagini con pagina dati leggibile, 1 senza pagina dati") || !strings.Contains(b.String(), "MRZ assente 1") {
		t.Errorf("il testo non riporta le immagini senza pagina dati o l'MRZ assente:\n%s", b.String())
	}
}

func TestWriteSplitENessunValore(t *testing.T) {
	fixedNow(t)
	good := goodFields()
	items := []Result{
		Verify("con", summary(good), func() *Truth { t := truthOf(good); return &t }()),
		Verify("senza", noMRZ(with(good, "capacity", "ZQXJ")), func() *Truth { t := truthOf(good); return &t }()),
	}
	var b strings.Builder
	WriteDir(&b, DirReport{Items: items}, false)
	out := b.String()
	if !strings.Contains(out, "CON E SENZA MRZ VALIDO") || !strings.Contains(out, "ACCURATEZZA PER CAMPO") || !strings.Contains(out, "FAMIGLIE DI ERRORE") {
		t.Errorf("mancano le tabelle:\n%s", out)
	}
	for _, secret := range []string{"ZQXJ", "ROSSI", "1234567", "ITALIAN"} {
		if strings.Contains(out, secret) {
			t.Errorf("il report contiene un valore (%q):\n%s", secret, out)
		}
	}
}
