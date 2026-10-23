package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"scanlibretto/internal/outsum"
)

func TestTabellaAndataERitorno(t *testing.T) {
	rows := []Row{
		{File: "scans/a.png", Rotation: 180, MRZ: true, CheckMap: "11111", FieldMap: "11111111111110", Warnings: 2, Seconds: 12.3},
		{File: "scans/vecchi test WIA/b.png", CheckMap: "-----", FieldMap: "00000000000000", Seconds: 4},
		{File: "scans/c.png", Err: true, Seconds: 1.2},
		{File: "scans/d.png", MRZ: true, CheckMap: "11011", FieldMap: "11111111111111", Truth: true, TruthMap: "GGGGGGGGSMX---", Name: 'S', Seconds: 9},
	}
	var buf bytes.Buffer
	if err := writeTable(&buf, []string{"commento di prova"}, rows); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.HasPrefix(text, "# commento di prova\nfile\trot\tmrz\tcheck\tcampi\tavvisi\tsec\tgiusti\tnomi\tmappa_check\tmappa_campi\tmappa_giusti\n") {
		t.Errorf("intestazione: %q", text)
	}
	for _, want := range []string{
		"scans/a.png\t180\tsi\t5/5\t13/14\t2\t12.3\t-\t-\t",
		"scans/vecchi test WIA/b.png\t0\tno\t0/5\t0/14\t0\t4.0\t-\t-\t",
		"scans/c.png\t-\terr\t-\t-\t-\t1.2\t-\t-\t",
		"scans/d.png\t0\tsi\t4/5\t14/14\t0\t9.0\t8/10\tS\t11011\t11111111111111\tGGGGGGGGSMX---",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("manca la riga %q in:\n%s", want, text)
		}
	}

	got, err := readTable(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	want := append([]Row(nil), rows...)
	want[2].CheckMap, want[2].FieldMap = dashes(outsum.NChecks), dashes(outsum.NFields) // la riga d'errore ha mappe vuote
	if !reflect.DeepEqual(got, want) {
		t.Errorf("riletta:\n got  %+v\n want %+v", got, want)
	}
}

func TestLetturaPerNomeDiColonna(t *testing.T) {
	// colonne in ordine diverso, una colonna sconosciuta, niente colonne della verita': deve leggere lo stesso
	in := "# commento\n\nextra\tmappa_campi\tfile\tmrz\tmappa_check\n" +
		"zzz\t11111111111111\tscans/x.png\tsi\t10101\r\n"
	rows, err := readTable(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].File != "scans/x.png" || !rows[0].MRZ || rows[0].checksOK() != 3 || rows[0].found() != 14 || rows[0].Truth {
		t.Errorf("riga letta: %+v", rows)
	}
}

func TestLetturaTabellaNonValida(t *testing.T) {
	casi := map[string]string{
		"vuota":                "",
		"solo commenti":        "# niente\n",
		"senza mappe":          "file\trot\tmrz\ncheck\n",
		"mappa corta":          "file\tmrz\tmappa_check\tmappa_campi\nx\tsi\t111\t11111111111111\n",
		"mappa giusti corta":   "file\tmrz\tmappa_check\tmappa_campi\tgiusti\tmappa_giusti\nx\tsi\t11111\t11111111111111\t1/2\tGG\n",
		"intestazione diversa": "nome\tesito\nx\ty\n",
	}
	for nome, in := range casi {
		if _, err := readTable(strings.NewReader(in)); err == nil {
			t.Errorf("%s: nessun errore", nome)
		}
	}
}

func TestRowFromSummary(t *testing.T) {
	s := outsum.Summary{Rotation: 90, HasMRZ: true, Checks: [outsum.NChecks]bool{true, false, true, true, false}, Warnings: 3,
		Fields: map[string]string{"type": "SI", "regional": "X", "surname": "non conta"}}
	r := rowFromSummary("scans/a.png", s, 7.25, "", 0)
	if r.Rotation != 90 || !r.MRZ || r.CheckMap != "10110" || r.FieldMap != "10000000000001" || r.Warnings != 3 || r.Truth {
		t.Errorf("riga: %+v", r)
	}
	if r.checksOK() != 3 || r.found() != 2 {
		t.Errorf("conteggi: %d, %d", r.checksOK(), r.found())
	}
	// senza MRZ i check digit non sono disponibili
	if r := rowFromSummary("x", outsum.Summary{}, 1, "", 0); r.CheckMap != "-----" || r.MRZ || r.nameSym() != '-' {
		t.Errorf("senza MRZ: %+v", r)
	}
	// con la verita'
	r = rowFromSummary("x", s, 1, "GGSM-X--------", 'G')
	if n, m := r.right(); !r.Truth || n != 2 || m != 4 || r.nameSym() != 'G' {
		t.Errorf("giusti %d/%d truth=%v nomi=%c", n, m, r.Truth, r.nameSym())
	}
}
