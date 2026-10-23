package libreta

// Prove della seconda lettura (mrzmerge.go): OCR contro lettore a template sulle parti dell'MRZ senza check digit.
// Dati inventati (ROSSI/MARIO), nessuna immagine.

import (
	"context"
	"image"
	"reflect"
	"strings"
	"testing"

	"scanlibretto/internal/mrz"
)

func TestRiconciliaLettureUguali(t *testing.T) {
	o, c := mrzCon(t, "ROSSI", "MARIO", "ITA", "M"), mrzCon(t, "ROSSI", "MARIO", "ITA", "M")
	d, notes, warns := reconcileMRZ(o, []string{"ocr"}, c, []string{"riga1 pos 3: lettura incerta (S)"})
	if d != o || !reflect.DeepEqual(notes, []string{"ocr"}) || len(warns) != 0 {
		t.Errorf("letture uguali: %v %v %v", d == o, notes, warns)
	}
}

// OCR e lettore a template differiscono in una lettera dei nomi e il lettore non segnala quella cella: si prende la sua
// riga 1. I check digit restano tutti validi e la correzione e' scritta nelle note.
func TestRiconciliaNomiDiversiCellaSicura(t *testing.T) {
	o, c := mrzCon(t, "ROSSX", "MARIO", "ITA", "M"), mrzCon(t, "ROSSI", "MARIO", "ITA", "M")
	for nome, cnotes := range map[string][]string{
		"nessuna nota":                     nil,
		"un'altra cella incerta":           {"riga1 pos 30: lettura incerta (<)", "riga2 pos 7: O→0 (vincoli MRZ del lettore a template)"},
		"nota della riga 2 sulla stessa x": {"riga2 pos 10: lettura incerta (5)"}, // numero della cella 10, ma della riga 2
	} {
		d, notes, warns := reconcileMRZ(o, []string{"ocr"}, c, cnotes)
		if d == o || d.Surname != "ROSSI" || d.GivenNames != "MARIO" || !d.Checks.AllOK() || d.Line2 != o.Line2 {
			t.Errorf("%s: dati %+v", nome, d)
			continue
		}
		if len(notes) != 2 || notes[0] != "ocr" || !strings.HasPrefix(notes[1], "riga1: cognome e nomi presi dal lettore a template") || len(warns) != 0 {
			t.Errorf("%s: note %q avvisi %q", nome, notes, warns)
		}
	}
	if o.Surname != "ROSSX" {
		t.Error("la lettura OCR e' stata modificata")
	}
}

// Se la cella che differisce e' segnalata incerta dal lettore a template, resta l'OCR e c'e' un avviso.
func TestRiconciliaNomiDiversiCellaIncerta(t *testing.T) {
	o, c := mrzCon(t, "ROSSX", "MARIO", "ITA", "M"), mrzCon(t, "ROSSI", "MARIO", "ITA", "M")
	for nome, cnotes := range map[string][]string{
		"cella incerta":      {"riga1 pos 10: lettura incerta (I)"}, // la decima cella e' la x/I
		"cella da vincoli":   {"riga1 pos 10: X→I (vincoli MRZ del lettore a template)"},
		"glifo sintetico":    {"riga1 pos 10: I riconosciuta solo da un glifo sintetico (mai vista su una scansione): da verificare"},
		"incerta e un'altra": {"riga1 pos 2: lettura incerta (S)", "riga1 pos 10: lettura incerta (I)"},
	} {
		d, notes, warns := reconcileMRZ(o, []string{"ocr"}, c, cnotes)
		if d != o || !reflect.DeepEqual(notes, []string{"ocr"}) || !reflect.DeepEqual(warns, []string{warnMRZLine1}) {
			t.Errorf("%s: %v %q %q", nome, d == o, notes, warns)
		}
	}
}

// Nazionalita' e sesso della riga 2 non hanno check digit: stessa regola dei nomi.
func TestRiconciliaNazionalitaESesso(t *testing.T) {
	o, c := mrzCon(t, "ROSSI", "MARIO", "ITA", "M"), mrzCon(t, "ROSSI", "MARIO", "ITB", "F")
	d, notes, warns := reconcileMRZ(o, nil, c, nil)
	if d == o || d.Nationality != "ITB" || d.Sex != "F" || !d.Checks.AllOK() || len(warns) != 0 {
		t.Fatalf("senza note: %+v %q", d, warns)
	}
	if len(notes) != 2 || !strings.HasPrefix(notes[0], "riga2: nazionalita' presa") || !strings.HasPrefix(notes[1], "riga2: sesso preso") {
		t.Errorf("note: %q", notes)
	}
	// il numero libretto, le date e il campo opzionale restano quelli dell'OCR
	if d.DocNumber != o.DocNumber || d.BirthDate != o.BirthDate || d.ExpiryDate != o.ExpiryDate || d.Optional != o.Optional {
		t.Errorf("parti coperte dai check cambiate: %+v", d)
	}
	// la cella diversa della nazionalita' (la 13) e il sesso (la 21) sono incerti: niente sostituzione, avvisi con il
	// nome del campo
	d, notes, warns = reconcileMRZ(o, nil, c, []string{"riga2 pos 13: lettura incerta (B)", "riga2 pos 21: lettura incerta (F)"})
	if d != o || len(notes) != 0 || !reflect.DeepEqual(warns, []string{"campo MRZ incerto: nazionalita_iso", "campo MRZ incerto: sesso"}) {
		t.Errorf("incerte: %v %q %q", d == o, notes, warns)
	}
	// una cella incerta che non e' quella che differisce (la 12, uguale nelle due letture) non impedisce la sostituzione
	d, _, warns = reconcileMRZ(o, nil, c, []string{"riga2 pos 12: lettura incerta (T)"})
	if d == o || d.Nationality != "ITB" || len(warns) != 0 {
		t.Errorf("cella incerta ma uguale: %+v %q", d, warns)
	}
	// solo la nazionalita' incerta: il sesso si prende
	d, _, warns = reconcileMRZ(o, nil, c, []string{"riga2 pos 13: lettura incerta (B)"})
	if d == o || d.Sex != "F" || d.Nationality != "ITA" || !reflect.DeepEqual(warns, []string{"campo MRZ incerto: nazionalita_iso"}) {
		t.Errorf("nazionalita' incerta e sesso no: %+v %q", d, warns)
	}
}

// La stessa riga 1 si puo' leggere con piu' o meno riempimenti '<': contano le lettere, non lo spazio tra le parole.
func TestRiconciliaSolaPosizioneDeiRiempimenti(t *testing.T) {
	o := mrzCon(t, "ROSSI", "MARIO", "ITA", "M")
	c := *o
	c.Line1 = "ISPAN" + "ROSSI<MARIO" + strings.Repeat("<", 44-16)
	d, notes, warns := reconcileMRZ(o, nil, &c, []string{"riga1 pos 11: lettura incerta (<)"})
	if d != o || len(notes) != 0 || len(warns) != 0 {
		t.Errorf("solo riempimenti: %v %q %q", d == o, notes, warns)
	}
}

// Due letture con tutti i check digit validi ma parti coperte diverse non sono confrontabili: non si decide niente.
func TestRiconciliaPartiControllateDiverse(t *testing.T) {
	o := mrzCon(t, "ROSSX", "MARIO", "ITA", "M")
	c := *mrzCon(t, "ROSSI", "MARIO", "ITA", "M")
	b := []byte(c.Line2)
	b[14] = '9' // un'altra data di nascita
	c.Line2 = string(b)
	d, notes, warns := reconcileMRZ(o, []string{"ocr"}, &c, nil)
	if d != o || !reflect.DeepEqual(notes, []string{"ocr"}) || len(warns) != 0 {
		t.Errorf("parti controllate diverse: %v %q %q", d == o, notes, warns)
	}
	// righe di lunghezza sbagliata o letture mancanti: nessun effetto
	for _, c := range []*mrz.Data{nil, {Line1: "ISPAN", Line2: "x"}} {
		if d, _, warns := reconcileMRZ(o, nil, c, nil); d != o || len(warns) != 0 {
			t.Errorf("lettura a template non valida: %v %q", d == o, warns)
		}
	}
}

func TestCelleSegnalate(t *testing.T) {
	notes := []string{
		"riga1 pos 22: lettura incerta (O)",
		"riga2 pos 7: O→0 (vincoli MRZ del lettore a template)",
		"riga1 pos 3: W riconosciuta solo da un glifo sintetico (mai vista su una scansione): da verificare",
		"riga2 pos 21: lettura incerta (M)",
		"testo che non e' una nota di cella",
		"riga3 pos 1: non esiste",
		"riga1 pos 0: posizione non valida",
	}
	if got := flaggedCells(notes, 1); !reflect.DeepEqual(got, map[int]bool{21: true, 2: true}) {
		t.Errorf("riga 1: %v", got)
	}
	if got := flaggedCells(notes, 2); !reflect.DeepEqual(got, map[int]bool{6: true, 20: true}) {
		t.Errorf("riga 2: %v", got)
	}
	if !anyFlagged(map[int]bool{5: true}, []int{1, 5}) || anyFlagged(map[int]bool{5: true}, []int{1, 2}) || anyFlagged(nil, []int{0}) {
		t.Error("anyFlagged")
	}
}

// Con readMRZOriented la sostituzione arriva fino alla lettura restituita: dati, note e punteggio della banda.
func TestRiconciliaDentroReadMRZOriented(t *testing.T) {
	ocr, celle := mrzCon(t, "ROSSX", "MARIO", "ITA", "M"), mrzCon(t, "ROSSI", "MARIO", "ITA", "M")
	l := installa(t, func(image.Image, string) (*mrz.Data, []string, int) { return celle, nil, 13 })
	r, err := readMRZOriented(context.Background(), immagineProva, "", bandaFissa(ocr, []string{"ocr"}, 14, nil))
	if err != nil || r.Data == ocr || r.Data.Surname != "ROSSI" || r.Score != 14 || len(r.Notes) != 2 || len(r.Warnings) != 0 {
		t.Errorf("lettura riconciliata: %+v %v", r, err)
	}
	if l.calls != 1 {
		t.Errorf("chiamate al lettore: %d", l.calls)
	}
	// un lettore a template con lettura parziale (check digit non tutti validi) non si usa per la seconda lettura
	parziale := mrzInventato(t, true)
	installa(t, func(image.Image, string) (*mrz.Data, []string, int) { return parziale, nil, scoreMRZ(*parziale) })
	if r, _ := readMRZOriented(context.Background(), immagineProva, "", bandaFissa(ocr, nil, 14, nil)); r.Data != ocr {
		t.Errorf("lettura parziale del lettore usata: %+v", r)
	}
}
