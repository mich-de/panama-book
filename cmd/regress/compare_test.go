package main

import (
	"strings"
	"testing"
)

// buona e' una riga con MRZ 5/5 e tutti i 14 campi letti.
func buona(file string) Row {
	return Row{File: file, MRZ: true, CheckMap: "11111", FieldMap: "11111111111111", Seconds: 10}
}

func trova(fs []Finding, kind string) *Finding {
	for i := range fs {
		if fs[i].Kind == kind {
			return &fs[i]
		}
	}
	return nil
}

func TestCompareUguali(t *testing.T) {
	b := buona("a.png")
	if fs := compareRow(b, b); len(fs) != 0 {
		t.Errorf("segnalazioni su righe uguali: %v", fs)
	}
	o := compare([]Row{b}, []Row{b})
	if o.Same != 1 || o.Regressed != 0 || o.Improved != 0 {
		t.Errorf("esito: %+v", o)
	}
}

func TestCompareMRZPersa(t *testing.T) {
	b := buona("a.png")
	c := Row{File: "a.png", CheckMap: "-----", FieldMap: "11111111110000", Seconds: 10}
	fs := compareRow(b, c)
	f := trova(fs, kindRegression)
	if f == nil {
		t.Fatalf("manca la regressione: %v", fs)
	}
	for _, want := range []string{"MRZ si->no", "check 5/5->0/5 (persi: doc_number,birth,expiry,optional,composite)", "campi 14/14->10/14 (persi: id_number,certificate,seafarers_id,regional)"} {
		if !strings.Contains(f.Text, want) {
			t.Errorf("manca %q in %q", want, f.Text)
		}
	}
	if trova(fs, kindImprovement) != nil {
		t.Errorf("miglioramento inatteso: %v", fs)
	}
}

func TestCompareMiglioramenti(t *testing.T) {
	b := Row{File: "a.png", CheckMap: "-----", FieldMap: "11110000000000", Seconds: 10}
	c := buona("a.png")
	fs := compareRow(b, c)
	f := trova(fs, kindImprovement)
	if f == nil || trova(fs, kindRegression) != nil {
		t.Fatalf("attesi solo miglioramenti: %v", fs)
	}
	for _, want := range []string{"MRZ no->si", "check 0/5->5/5 (guadagnati: doc_number", "campi 4/14->14/14 (guadagnati: date_of_issue,date_of_expiry"} {
		if !strings.Contains(f.Text, want) {
			t.Errorf("manca %q in %q", want, f.Text)
		}
	}
}

// Un campo perso e' una regressione anche se un altro, nello stesso momento, viene letto: le due cose si vedono.
func TestCompareScambioDiCampi(t *testing.T) {
	b := buona("a.png")
	b.FieldMap = "11111111111110" // manca regional
	c := buona("a.png")
	c.FieldMap = "01111111111111" // manca type
	o := compare([]Row{b}, []Row{c})
	if o.Regressed != 1 || o.Improved != 1 || o.Same != 0 {
		t.Errorf("esito: %+v", o)
	}
	if f := trova(o.Findings, kindRegression); f == nil || !strings.Contains(f.Text, "persi: type") {
		t.Errorf("regressione: %+v", f)
	}
	if f := trova(o.Findings, kindImprovement); f == nil || !strings.Contains(f.Text, "guadagnati: regional") {
		t.Errorf("miglioramento: %+v", f)
	}
}

func TestCompareErrori(t *testing.T) {
	ok, ko := buona("a.png"), Row{File: "a.png", Err: true}
	if f := trova(compareRow(ok, ko), kindRegression); f == nil {
		t.Error("da riuscita a fallita deve essere regressione")
	}
	if f := trova(compareRow(ko, ok), kindImprovement); f == nil {
		t.Error("da fallita a riuscita deve essere miglioramento")
	}
	if fs := compareRow(ko, ko); len(fs) != 0 {
		t.Errorf("due fallimenti: %v", fs)
	}
}

func TestCompareVerita(t *testing.T) {
	b := buona("a.png")
	b.Truth, b.TruthMap = true, "GGGGGGGGGGGGG-"
	c := buona("a.png")
	c.Truth, c.TruthMap = true, "GGGGGGGSGGGGGX"
	fs := compareRow(b, c)
	f := trova(fs, kindRegression)
	if f == nil || !strings.Contains(f.Text, "giusti 13/13->12/13 (da giusti a sbagliati o mancanti: level)") {
		t.Fatalf("regressione sui giusti: %v", fs)
	}
	if n := trova(fs, kindNote); n == nil || !strings.Contains(n.Text, "campi letti ma vuoti in verita': regional") {
		t.Errorf("nota sul campo spurio: %v", fs)
	}
	// il campo giusto che diventa mancante e' una regressione anche se il numero di campi letti e' uguale
	c2 := buona("a.png")
	c2.Truth, c2.TruthMap = true, "GGGGGGGGGGGGM-"
	if f := trova(compareRow(b, c2), kindRegression); f == nil || !strings.Contains(f.Text, "mancanti: seafarers_id") {
		t.Errorf("giusto -> mancante: %v", f)
	}
	// verita' solo da una parte: nessun confronto sui giusti
	c3 := buona("a.png")
	if fs := compareRow(b, c3); len(fs) != 0 {
		t.Errorf("verita' presente solo nella baseline: %v", fs)
	}
	// da sbagliato a giusto: miglioramento
	if f := trova(compareRow(c, b), kindImprovement); f == nil || !strings.Contains(f.Text, "ora giusti: level") {
		t.Errorf("sbagliato -> giusto: %v", f)
	}
}

// Dove la verita' dice che un campo e' vuoto (copertina, striscia MRZ) un valore letto e' inventato: perderlo non e' una
// regressione e guadagnarne un altro non e' un miglioramento. Senza verita' resta uno scambio (regressione e miglioramento).
func TestCompareCampiInventatiNonContano(t *testing.T) {
	b, c := buona("a.png"), buona("a.png")
	b.FieldMap, c.FieldMap = "00000000000100", "00000010000000" // letti: prima certificate, ora date_of_birth
	b.Truth, b.TruthMap = true, "-----------X--"
	c.Truth, c.TruthMap = true, "------X-------"
	fs := compareRow(b, c)
	if trova(fs, kindRegression) != nil || trova(fs, kindImprovement) != nil {
		t.Errorf("campi inventati contati: %v", fs)
	}
	if n := trova(fs, kindNote); n == nil || !strings.Contains(n.Text, "campi letti ma vuoti in verita': date_of_birth") {
		t.Errorf("la nota sul campo inventato manca: %v", fs)
	}
	// senza verita' lo stesso scambio e' una regressione e un miglioramento
	b.Truth, c.Truth = false, false
	if fs := compareRow(b, c); trova(fs, kindRegression) == nil || trova(fs, kindImprovement) == nil {
		t.Errorf("senza verita': %v", fs)
	}
	// un campo vero perso (giusto prima, mancante ora) resta una regressione anche con la verita'
	b.Truth, b.TruthMap, b.FieldMap = true, "-----------G--", "00000000000100"
	c.Truth, c.TruthMap, c.FieldMap = true, "-----------M--", "00000000000000"
	if f := trova(compareRow(b, c), kindRegression); f == nil || !strings.Contains(f.Text, "campi 1/14->0/14 (persi: certificate)") {
		t.Errorf("campo vero perso: %v", f)
	}
}

// La riga 1 dell'MRZ non ha check digit: i nomi che passano da giusti a sbagliati sono una regressione.
func TestCompareNomiMRZ(t *testing.T) {
	b, c := buona("a.png"), buona("a.png")
	b.Name, c.Name = symRight, symWrong
	if f := trova(compareRow(b, c), kindRegression); f == nil || !strings.Contains(f.Text, "nomi MRZ (riga 1) da giusti a sbagliati") {
		t.Errorf("giusti -> sbagliati: %v", f)
	}
	if f := trova(compareRow(c, b), kindImprovement); f == nil || !strings.Contains(f.Text, "da sbagliati a giusti") {
		t.Errorf("sbagliati -> giusti: %v", f)
	}
	// MRZ trovato per la prima volta ma con i nomi sbagliati: solo una nota
	b.Name = symMissing
	if fs := compareRow(b, c); len(fs) != 1 || fs[0].Kind != kindNote || !strings.Contains(fs[0].Text, "MRZ trovato con i nomi sbagliati") {
		t.Errorf("mancante -> sbagliati: %v", fs)
	}
	// tabella vecchia, senza la colonna dei nomi: niente da confrontare
	b.Name = 0
	if fs := compareRow(b, c); len(fs) != 0 {
		t.Errorf("baseline senza nomi: %v", fs)
	}
}

// Un campo che prima mancava e ora e' sbagliato non e' una regressione ma si nota: il vuoto si vede, l'errore no.
func TestCompareDaMancanteASbagliato(t *testing.T) {
	b := buona("a.png")
	b.Truth, b.TruthMap = true, "GGMMGGGGGGGGGG"
	c := buona("a.png")
	c.Truth, c.TruthMap = true, "GGSMGGGGGGGGGG"
	fs := compareRow(b, c)
	if len(fs) != 1 || fs[0].Kind != kindNote || !strings.Contains(fs[0].Text, "campi ora sbagliati (prima mancanti o non confrontabili): nationality") {
		t.Errorf("segnalazioni: %v", fs)
	}
	// da giusto a sbagliato e' una regressione e non si ripete tra le note
	b.TruthMap = "GGGMGGGGGGGGGG"
	fs = compareRow(b, c)
	if trova(fs, kindRegression) == nil || trova(fs, kindNote) != nil {
		t.Errorf("giusto -> sbagliato: %v", fs)
	}
}

func TestCompareSoloNote(t *testing.T) {
	b := buona("a.png")
	c := buona("a.png")
	c.Rotation, c.Warnings, c.Seconds = 180, 3, 25
	fs := compareRow(b, c)
	if len(fs) != 1 || fs[0].Kind != kindNote {
		t.Fatalf("attesa una sola nota: %v", fs)
	}
	for _, want := range []string{"rotazione 0->180", "avvisi 0->3", "tempo 10.0->25.0 s"} {
		if !strings.Contains(fs[0].Text, want) {
			t.Errorf("manca %q in %q", want, fs[0].Text)
		}
	}
	// le note non cambiano l'esito
	if o := compare([]Row{b}, []Row{c}); o.Same != 1 || o.Regressed != 0 || o.Improved != 0 {
		t.Errorf("esito: %+v", o)
	}
	// un aumento di tempo piccolo non e' una nota
	c.Rotation, c.Warnings, c.Seconds = 0, 0, 12
	if fs := compareRow(b, c); len(fs) != 0 {
		t.Errorf("tempo: %v", fs)
	}
}

func TestCompareNuoveEAssenti(t *testing.T) {
	base := []Row{buona("a.png"), buona("solo-base.png")}
	cur := []Row{buona("a.png"), buona("nuova.png")}
	o := compare(base, cur)
	if o.New != 1 || o.Absent != 1 || o.Same != 1 {
		t.Errorf("esito: %+v", o)
	}
}
