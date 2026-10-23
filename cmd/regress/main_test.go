package main

// Prove di tutto il comando con un "libretto finto": il binario di test stesso, che quando trova la variabile
// REGRESS_FAKE_LIBRETTO=1 si comporta come `libretto extract -i FILE` (niente OCR, niente scanner).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"scanlibretto/internal/outsum"
)

const (
	fakeEnv = "REGRESS_FAKE_LIBRETTO" // "1": il binario di test fa da libretto finto
	modeEnv = "REGRESS_FAKE_MODE"     // base | peggio (ok diventa mediocre) | meglio (mediocre diventa ok)
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeEnv) == "1" {
		os.Exit(fakeLibretto(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeLibretto imita `libretto extract -i FILE`. L'esito dipende dall'inizio del nome del file (senza cifre):
// ok (MRZ 5/5, 14 campi), mediocre (MRZ 3/5, 9 campi), nomrz (niente MRZ, 2 campi), rotto (errore), garbage
// (uscita che non e' JSON), lento (non finisce mai). Accetta solo "extract -i FILE": ogni altro uso e' un errore.
func fakeLibretto(args []string) int {
	if len(args) != 3 || args[0] != "extract" || args[1] != "-i" {
		fmt.Fprintln(os.Stderr, "libretto finto: uso non previsto:", args)
		return 2
	}
	if _, err := os.Stat(args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "errore: lettura immagine")
		return 1
	}
	kind := strings.TrimRight(strings.SplitN(filepath.Base(args[2]), ".", 2)[0], "0123456789")
	switch mode := os.Getenv(modeEnv); {
	case kind == "ok" && mode == "peggio":
		kind = "mediocre"
	case kind == "mediocre" && mode == "meglio":
		kind = "ok"
	}
	switch kind {
	case "ok":
		emitFake(true, 5, 14)
	case "mediocre":
		emitFake(true, 3, 9)
	case "nomrz":
		emitFake(false, 0, 2)
	case "rotto":
		fmt.Fprintln(os.Stderr, "errore: guasto finto")
		return 1
	case "garbage":
		fmt.Println("questo non e' JSON")
	case "lento":
		time.Sleep(time.Minute)
	}
	return 0
}

// fakeValue e' il valore finto di un campo; contiene "VAL-" per controllare che non finisca mai nei rapporti.
func fakeValue(k string) string { return "VAL-" + strings.ToUpper(k) }

func emitFake(withMRZ bool, checks, fields int) {
	f := map[string]string{}
	for i := 0; i < fields; i++ {
		f[outsum.FieldKeys[i]] = fakeValue(outsum.FieldKeys[i])
	}
	out := map[string]any{"schema_version": 1, "rotation_deg": 0, "fields": f,
		"warnings": []map[string]string{{"code": "UNKNOWN_WARNING"}}}
	if withMRZ {
		f["surname"], f["given_names"] = "ROSSI", "MARIO" // dall'MRZ; con la modalita' "nomisbagliati" il cognome e' storpiato
		if os.Getenv(modeEnv) == "nomisbagliati" {
			f["surname"] = "ROSSX"
		}
		c := map[string]bool{}
		for i := 0; i < outsum.NChecks; i++ {
			c[outsum.CheckKeys[i]] = i < checks
		}
		out["rotation_deg"] = 180
		out["mrz"] = map[string]any{"checks": c}
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
}

// corpusFinto crea una radice con immagini finte (il contenuto non conta) e la restituisce.
func corpusFinto(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	creaFile(t, root, "scans/ok1.png", "scans/mediocre1.png", "scans/nomrz1.png", "scans/rotto1.png", "scans/garbage1.png",
		"scans/ok1-pagina.png", "facsimili/panama/ok2.jpg")
	return root
}

// eseguiRegress lancia il comando con il libretto finto e restituisce codice di uscita, stdout e stderr.
func eseguiRegress(t *testing.T, mode, root string, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv(fakeEnv, "1")
	t.Setenv(modeEnv, mode)
	var o, e bytes.Buffer
	all := append([]string{"-exe", os.Args[0], "-q", "-root", root}, args...)
	code := run(context.Background(), all, &o, &e)
	return code, o.String(), e.String()
}

func righe(t *testing.T, tsv string) map[string]Row {
	t.Helper()
	rows, err := readTable(strings.NewReader(tsv))
	if err != nil {
		t.Fatalf("tabella non leggibile: %v\n%s", err, tsv)
	}
	m := map[string]Row{}
	for _, r := range rows {
		m[r.File] = r
	}
	return m
}

// albero legge tutti i file sotto root: percorso -> contenuto. Serve a verificare che l'harness non scriva nulla.
func albero(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		out[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRunTabellaEImmaginiIntatte(t *testing.T) {
	root := corpusFinto(t)
	creaFile(t, root, "scans/nomrz2.png", "scans/lento-non-in-corpus.txt")
	tmp := t.TempDir()
	t.Setenv("TMP", tmp) // la copia dell'eseguibile va qui e deve sparire a fine prova
	t.Setenv("TEMP", tmp)
	t.Setenv("TMPDIR", tmp)
	prima := albero(t, root)

	code, out, errOut := eseguiRegress(t, "base", root)
	if code != exitOK {
		t.Fatalf("uscita %d, attesa 0\n%s", code, errOut)
	}
	rows := righe(t, out)
	if len(rows) != 7 {
		t.Fatalf("righe %d, attese 7 (la pagina ritagliata e il .txt non sono nel corpus): %v", len(rows), rows)
	}
	if _, ok := rows["scans/ok1-pagina.png"]; ok {
		t.Error("la pagina ritagliata e' entrata nel corpus")
	}
	ok := rows["scans/ok1.png"]
	if ok.Err || !ok.MRZ || ok.Rotation != 180 || ok.checksOK() != 5 || ok.found() != 14 || ok.Warnings != 1 {
		t.Errorf("ok1: %+v", ok)
	}
	if m := rows["scans/mediocre1.png"]; !m.MRZ || m.checksOK() != 3 || m.found() != 9 {
		t.Errorf("mediocre1: %+v", m)
	}
	if n := rows["scans/nomrz1.png"]; n.Err || n.MRZ || n.checksOK() != 0 || n.found() != 2 || n.Rotation != 0 {
		t.Errorf("nomrz1: %+v", n)
	}
	for _, f := range []string{"scans/rotto1.png", "scans/garbage1.png"} {
		if !rows[f].Err {
			t.Errorf("%s doveva risultare in errore: %+v", f, rows[f])
		}
	}
	if !strings.Contains(errOut, "scans/rotto1.png: uscita: exit status 1; errore: guasto finto") ||
		!strings.Contains(errOut, "scans/garbage1.png: JSON non valido") {
		t.Errorf("avvisi sugli errori mancanti:\n%s", errOut)
	}
	if !strings.Contains(out, "# totali: MRZ 3/7; check 5/5 2/7; check 13/35; campi 41/98; errori 2\n") {
		t.Errorf("totali in intestazione:\n%s", out)
	}
	if strings.Contains(out+errOut, "VAL-") {
		t.Error("un valore letto dal documento e' finito nell'uscita")
	}
	if dopo := albero(t, root); !reflect.DeepEqual(prima, dopo) {
		t.Error("l'harness ha modificato o creato file nella radice")
	}
	if resti, _ := os.ReadDir(tmp); len(resti) != 0 {
		t.Errorf("la copia dell'eseguibile non e' stata eliminata: %v", resti)
	}
}

func TestRunConfrontoConBaseline(t *testing.T) {
	root := corpusFinto(t)
	baseline := filepath.Join(t.TempDir(), "baseline.tsv")

	if code, out, errOut := eseguiRegress(t, "base", root, "-o", baseline); code != exitOK || out != "" {
		t.Fatalf("baseline: uscita %d, stdout %q\n%s", code, out, errOut)
	}
	if _, err := os.Stat(baseline); err != nil {
		t.Fatal("baseline non scritta")
	}
	// -o non sovrascrive senza -force
	if code, _, errOut := eseguiRegress(t, "base", root, "-o", baseline); code != exitUsage || !strings.Contains(errOut, "esiste gia'") {
		t.Errorf("sovrascrittura: uscita %d\n%s", code, errOut)
	}
	if code, _, _ := eseguiRegress(t, "base", root, "-o", baseline, "-force"); code != exitOK {
		t.Errorf("con -force: uscita %d", code)
	}

	// stessa prova: nessuna differenza
	code, _, errOut := eseguiRegress(t, "base", root, "-baseline", baseline)
	if code != exitOK || !strings.Contains(errOut, "== esito: 0 con regressioni, 0 con miglioramenti, 6 invariate; nuove 0, assenti 0") {
		t.Errorf("stessa prova: uscita %d\n%s", code, errOut)
	}

	// peggio: ok1 e ok2 perdono check digit e campi
	code, _, errOut = eseguiRegress(t, "peggio", root, "-baseline", baseline)
	if code != exitRegression {
		t.Errorf("regressione: uscita %d, attesa 1", code)
	}
	for _, want := range []string{
		"REGRESSIONE   scans/ok1.png: check 5/5->3/5 (persi: optional,composite); campi 14/14->9/14 (persi: seamans_book,id_number,certificate,seafarers_id,regional)",
		"REGRESSIONE   facsimili/panama/ok2.jpg: ",
		"== esito: 2 con regressioni, 0 con miglioramenti, 4 invariate",
		"== REGRESSIONI TROVATE",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("manca %q in:\n%s", want, errOut)
		}
	}

	// meglio: mediocre1 diventa ok. Nessuna regressione, uscita 0
	code, _, errOut = eseguiRegress(t, "meglio", root, "-baseline", baseline)
	if code != exitOK || !strings.Contains(errOut, "MIGLIORAMENTO scans/mediocre1.png: check 3/5->5/5 (guadagnati: optional,composite); campi 9/14->14/14") ||
		strings.Contains(errOut, "REGRESSIONE") {
		t.Errorf("miglioramento: uscita %d\n%s", code, errOut)
	}
}

func TestRunElencoFiltriEFileIndicati(t *testing.T) {
	root := corpusFinto(t)
	// -list non esegue nulla: l'eseguibile puo' anche non esistere
	var o, e bytes.Buffer
	code := run(context.Background(), []string{"-exe", "non-esiste.exe", "-root", root, "-list", "-skip", "rotto,garbage"}, &o, &e)
	want := "facsimili/panama/ok2.jpg\nscans/mediocre1.png\nscans/nomrz1.png\nscans/ok1.png\n"
	if code != exitOK || o.String() != want {
		t.Errorf("-list: uscita %d\n%q\nattesa\n%q", code, o.String(), want)
	}
	// file indicati a mano (relativi e assoluti) al posto del corpus
	code, out, errOut := eseguiRegress(t, "base", root, "scans/ok1.png", filepath.Join(root, "scans", "nomrz1.png"))
	if rows := righe(t, out); code != exitOK || len(rows) != 2 || rows["scans/ok1.png"].File == "" || rows["scans/nomrz1.png"].File == "" {
		t.Errorf("file indicati: uscita %d, righe %v\n%s", code, rows, errOut)
	}
	// -only
	_, out, _ = eseguiRegress(t, "base", root, "-only", "ok")
	if rows := righe(t, out); len(rows) != 2 {
		t.Errorf("-only ok: %v", rows)
	}
}

func TestRunErroriDUso(t *testing.T) {
	root := corpusFinto(t)
	var e bytes.Buffer
	casi := map[string][]string{
		"jobs zero":              {"-exe", os.Args[0], "-root", root, "-j", "0"},
		"eseguibile assente":     {"-exe", filepath.Join(root, "non-esiste.exe"), "-root", root},
		"baseline assente":       {"-exe", os.Args[0], "-root", root, "-baseline", filepath.Join(root, "non-esiste.tsv")},
		"truth non cartella":     {"-exe", os.Args[0], "-root", root, "-truth", filepath.Join(root, "scans", "ok1.png")},
		"nessuna immagine":       {"-exe", os.Args[0], "-root", root, "-only", "zzz"},
		"flag sconosciuto":       {"-exe", os.Args[0], "-root", root, "-non-esiste"},
		"baseline non valida":    {"-exe", os.Args[0], "-root", root, "-baseline", filepath.Join(root, "scans", "ok1.png")},
		"radice senza corpus":    {"-exe", os.Args[0], "-root", t.TempDir()},
		"cartella di -o assente": {"-exe", os.Args[0], "-root", root, "-o", filepath.Join(root, "non", "c-e", "x.tsv")},
	}
	for nome, args := range casi {
		e.Reset()
		if code := run(context.Background(), args, &bytes.Buffer{}, &e); code != exitUsage {
			t.Errorf("%s: uscita %d, attesa 2\n%s", nome, code, e.String())
		}
	}
}

// -against confronta due tabelle gia' scritte, senza eseguire nulla (l'eseguibile non serve).
func TestRunConfrontoTraDueTabelle(t *testing.T) {
	dir := t.TempDir()
	scrivi := func(nome string, rows ...Row) string {
		p := filepath.Join(dir, nome)
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := writeTable(f, []string{"tabella di prova"}, rows); err != nil {
			t.Fatal(err)
		}
		return p
	}
	prima := scrivi("prima.tsv", buona("scans/a.png"), Row{File: "scans/b.png", CheckMap: "-----", FieldMap: "00000000000000"})
	dopo := scrivi("dopo.tsv", Row{File: "scans/a.png", MRZ: true, CheckMap: "11110", FieldMap: "11111111111111", Seconds: 10}, buona("scans/b.png"))
	var e bytes.Buffer
	code := run(context.Background(), []string{"-baseline", prima, "-against", dopo}, &bytes.Buffer{}, &e)
	if code != exitRegression {
		t.Errorf("uscita %d, attesa 1 (una regressione e un miglioramento)\n%s", code, e.String())
	}
	for _, want := range []string{
		"prima (", "dopo  (",
		"REGRESSIONE   scans/a.png: check 5/5->4/5 (persi: composite)",
		"MIGLIORAMENTO scans/b.png: MRZ no->si",
		"== esito: 1 con regressioni, 1 con miglioramenti, 0 invariate",
	} {
		if !strings.Contains(e.String(), want) {
			t.Errorf("manca %q in:\n%s", want, e.String())
		}
	}
	// al contrario non c'e' nessuna regressione per a.png ma b.png diventa peggiore
	e.Reset()
	if code := run(context.Background(), []string{"-baseline", dopo, "-against", prima}, &bytes.Buffer{}, &e); code != exitRegression {
		t.Errorf("inverso: uscita %d\n%s", code, e.String())
	}
	// stesso file con se' stesso: nessuna differenza
	e.Reset()
	if code := run(context.Background(), []string{"-baseline", prima, "-against", prima}, &bytes.Buffer{}, &e); code != exitOK {
		t.Errorf("stessa tabella: uscita %d\n%s", code, e.String())
	}
	// errori d'uso
	for nome, args := range map[string][]string{
		"senza -baseline":  {"-against", dopo},
		"tabella assente":  {"-baseline", prima, "-against", filepath.Join(dir, "non-esiste.tsv")},
		"baseline assente": {"-baseline", filepath.Join(dir, "non-esiste.tsv"), "-against", dopo},
	} {
		e.Reset()
		if code := run(context.Background(), args, &bytes.Buffer{}, &e); code != exitUsage {
			t.Errorf("%s: uscita %d, attesa 2\n%s", nome, code, e.String())
		}
	}
}

// L'impronta della verita' va nell'intestazione e il confronto avverte se due tabelle hanno verita' diverse.
func TestImprontaDellaVerita(t *testing.T) {
	dir := t.TempDir()
	scrivi := func(nome, testo string) {
		if err := os.WriteFile(filepath.Join(dir, nome), []byte(testo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	scrivi("a.json", `{"type":"SI"}`)
	scrivi("b.json", `{"type":"SI"}`)
	scrivi("nota.txt", "non conta")
	p1, n1, err := truthPrint(dir)
	if err != nil || n1 != 2 || len(p1) != 12 {
		t.Fatalf("impronta %q, %d file, %v", p1, n1, err)
	}
	if p2, _, _ := truthPrint(dir); p2 != p1 {
		t.Errorf("l'impronta non e' stabile: %q e %q", p1, p2)
	}
	scrivi("b.json", `{"type":"XX"}`)
	if p3, _, _ := truthPrint(dir); p3 == p1 {
		t.Error("l'impronta non cambia se cambia il contenuto")
	}
	if _, _, err := truthPrint(filepath.Join(dir, "non-esiste")); err == nil {
		t.Error("cartella assente senza errore")
	}

	scriviTabella := func(nome, impronta string) string {
		p := filepath.Join(dir, nome)
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		meta := []string{"tabella di prova"}
		if impronta != "" {
			meta = append(meta, "impronta-verita: "+impronta+" (2 file)")
		}
		if err := writeTable(f, meta, []Row{buona("scans/a.png")}); err != nil {
			t.Fatal(err)
		}
		return p
	}
	uno, due, senza := scriviTabella("uno.tsv", "aaaaaaaaaaaa"), scriviTabella("due.tsv", "bbbbbbbbbbbb"), scriviTabella("senza.tsv", "")
	if got := tablePrint(uno); got != "aaaaaaaaaaaa" {
		t.Errorf("tablePrint = %q", got)
	}
	if got := tablePrint(senza); got != "" {
		t.Errorf("tabella senza verita': %q", got)
	}
	for _, c := range []struct {
		prima, dopo string
		avviso      bool
	}{{uno, due, true}, {uno, uno, false}, {uno, senza, false}, {senza, due, false}} {
		var e bytes.Buffer
		if code := run(context.Background(), []string{"-baseline", c.prima, "-against", c.dopo}, &bytes.Buffer{}, &e); code != exitOK {
			t.Errorf("uscita %d\n%s", code, e.String())
		}
		if got := strings.Contains(e.String(), "ATTENZIONE: la verita' non e' la stessa"); got != c.avviso {
			t.Errorf("%s -> %s: avviso=%v, atteso %v\n%s", filepath.Base(c.prima), filepath.Base(c.dopo), got, c.avviso, e.String())
		}
	}
}

func TestRunAiuto(t *testing.T) {
	var o, e bytes.Buffer
	if code := run(context.Background(), []string{"-h"}, &o, &e); code != exitOK || !strings.Contains(e.String(), "-baseline") || !strings.Contains(e.String(), "-truth") {
		t.Errorf("-h: uscita %d\n%s", code, e.String())
	}
}

func TestRunTempoScaduto(t *testing.T) {
	root := t.TempDir()
	creaFile(t, root, "scans/lento1.png")
	code, out, errOut := eseguiRegress(t, "base", root, "-timeout", "700ms")
	if code != exitOK || !righe(t, out)["scans/lento1.png"].Err || !strings.Contains(errOut, "scans/lento1.png: tempo scaduto") {
		t.Errorf("tempo scaduto: uscita %d\n%s\n%s", code, out, errOut)
	}
}

func TestRunInterrotto(t *testing.T) {
	root := corpusFinto(t)
	t.Setenv(fakeEnv, "1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var o, e bytes.Buffer
	if code := run(ctx, []string{"-exe", os.Args[0], "-q", "-root", root}, &o, &e); code != exitInterrupt || o.Len() != 0 {
		t.Errorf("interrotto: uscita %d, stdout %q", code, o.String())
	}
}

func TestRunConVerita(t *testing.T) {
	root := corpusFinto(t)
	truth := t.TempDir()
	// verita' di ok1: tutti i valori uguali a quelli finti, tranne level (diverso) e certificate (vuoto)
	v := map[string]string{}
	for _, k := range outsum.FieldKeys {
		v[k] = fakeValue(k)
	}
	v["level"], v["certificate"] = "ALTRO", ""
	v["_riga_nome"] = "MARIO ROSSI" // la riga del nome, senza ordine: l'MRZ finto ha ROSSI e MARIO
	b, _ := json.Marshal(v)
	if err := os.WriteFile(filepath.Join(truth, "ok1.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	baseline := filepath.Join(t.TempDir(), "baseline.tsv")

	code, out, errOut := eseguiRegress(t, "base", root, "-truth", truth, "-o", baseline)
	if code != exitOK || out != "" {
		t.Fatalf("baseline con verita': uscita %d\n%s", code, errOut)
	}
	f, err := os.Open(baseline)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := readTable(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	var ok1, ok2 Row
	for _, r := range rows {
		switch r.File {
		case "scans/ok1.png":
			ok1 = r
		case "facsimili/panama/ok2.jpg":
			ok2 = r
		}
	}
	if !ok1.Truth || ok1.TruthMap != "GGGGGGGSGGGXGG" {
		t.Errorf("ok1 verita': %+v", ok1)
	}
	if n, m := ok1.right(); n != 12 || m != 13 {
		t.Errorf("giusti %d/%d, attesi 12/13", n, m)
	}
	if ok2.Truth {
		t.Errorf("ok2 non ha verita': %+v", ok2)
	}
	tsv, _ := os.ReadFile(baseline)
	if !strings.Contains(string(tsv), "\t12/13\tG\t11111\t11111111111111\tGGGGGGGSGGGXGG") ||
		!strings.Contains(string(tsv), "giusti 12/13 (su 1 immagini con verita')") ||
		!strings.Contains(string(tsv), "nomi MRZ giusti 1/1 (senza MRZ 0)") {
		t.Errorf("tabella con giusti e nomi:\n%s", tsv)
	}
	if ok1.Name != symRight {
		t.Errorf("nomi di ok1: %c, attesi G", ok1.nameSym())
	}

	// peggio: ok1 perde campi giusti (da giusti a mancanti); i nomi restano giusti
	code, _, errOut = eseguiRegress(t, "peggio", root, "-truth", truth, "-baseline", baseline)
	if code != exitRegression || !strings.Contains(errOut, "giusti 12/13->8/13 (da giusti a sbagliati o mancanti: seamans_book,id_number,seafarers_id,regional)") {
		t.Errorf("regressione sui giusti: uscita %d\n%s", code, errOut)
	}
	if strings.Contains(errOut, "nomi MRZ (riga 1)") {
		t.Errorf("i nomi non sono cambiati:\n%s", errOut)
	}
	// nomisbagliati: l'MRZ ha il cognome storpiato, la riga 1 non ha check digit ma la verita' se ne accorge
	code, out, errOut2 := eseguiRegress(t, "nomisbagliati", root, "-truth", truth, "-baseline", baseline)
	if code != exitRegression || !strings.Contains(errOut2, "REGRESSIONE   scans/ok1.png: nomi MRZ (riga 1) da giusti a sbagliati") {
		t.Errorf("regressione sui nomi: uscita %d\n%s", code, errOut2)
	}
	if strings.Contains(out+errOut+errOut2, "VAL-") || strings.Contains(out+errOut+errOut2, "ROSS") {
		t.Error("un valore letto dal documento o dalla verita' e' finito nell'uscita")
	}
}
