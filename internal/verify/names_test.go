package verify

import "testing"

// Nomi INVENTATI.
func TestNameMatchesMRZ(t *testing.T) {
	const long = "VERYLONGSURNAMEHERE" // 19 lettere: con nomi lunghi l'MRZ (39 caratteri) tronca
	cases := []struct {
		name                string
		surname, given, row string
		want                bool
	}{
		{"uguali", "ROSSI", "MARIO LUIGI", "ROSSI MARIO LUIGI", true},
		{"senza ordine", "ROSSI", "MARIO LUIGI", "MARIO LUIGI ROSSI", true},
		{"nomi e cognome in ordine diverso", "ROSSI", "MARIO LUIGI", "LUIGI ROSSI MARIO", true},
		{"accenti e apostrofi", "D ANGELO", "JOSE", "JOSÉ D'ANGELO", true},
		{"trattino", "ROSSI VERDI", "ANNA", "ANNA ROSSI-VERDI", true},
		{"minuscole", "rossi", "mario", "ROSSI MARIO", true},
		{"solo cognome", "SMITH", "", "SMITH", true},
		{"una lettera sbagliata", "ROSSL", "MARIO", "ROSSI MARIO", false},
		{"manca una parola e l'MRZ era corto", "ROSSI", "MARIO", "MARIO LUIGI ROSSI", false},
		{"parola in piu' nell'MRZ", "ROSSI", "MARIO LUIGI", "MARIO ROSSI", false},
		{"due parole diverse", "ROSSL", "MARIA LUIGI", "ROSSI MARIO LUIGI", false},
		{"prefisso ma MRZ corto: lettere perse dall'OCR", "ROSSI", "MARI", "MARIO ROSSI", false},
		{"cognome e nomi scambiati tra loro", "MARIO", "ROSSI", "ROSSI MARIO", true},
		// troncatura a 39 caratteri: cognome (19) + `<<` + nomi
		{"troncata in mezzo a una parola", long, "ALPHA BRAVO CHARL", "ALPHA BRAVO CHARLIE DELTA " + long, true},
		{"troncata: la parola dopo non ci entrava", long, "ALPHA BRAVO", "ALPHA BRAVO CHARLIE " + long, true},
		{"parola persa che ci sarebbe entrata", long, "ALPHA BRAVO", "ALPHA BRAVO DELTA " + long, false},
		{"parola tagliata ma l'MRZ e' corto", "ROSSI", "ALPHA BRAV", "ALPHA BRAVO ROSSI", false},
		{"vuoti", "", "", "ROSSI", false},
		{"riga stampata vuota", "ROSSI", "MARIO", "", false},
	}
	for _, c := range cases {
		if got := NameMatchesMRZ(c.surname, c.given, c.row); got != c.want {
			t.Errorf("%s: NameMatchesMRZ(%q, %q, %q) = %v, atteso %v", c.name, c.surname, c.given, c.row, got, c.want)
		}
	}
}

func TestNameWords(t *testing.T) {
	got := nameWords("  D'Angelo-Verdi  JOSÉ<<MARÍA ")
	want := []string{"D", "ANGELO", "VERDI", "JOSE", "MARIA"}
	if len(got) != len(want) {
		t.Fatalf("parole: %v, attese %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parola %d: %q, attesa %q", i, got[i], want[i])
		}
	}
}

func TestTruthNameRowDaFile(t *testing.T) {
	tr, err := ParseTruth(truthJSON(t, goodFields(), map[string]any{"_riga_nome": "MARIO LUIGI ROSSI"}))
	if err != nil {
		t.Fatal(err)
	}
	if !NameMatchesMRZ("ROSSI", "MARIO LUIGI", tr.NameRow) {
		t.Errorf("la riga del nome letta dal file (%q) dovrebbe corrispondere", "_riga_nome")
	}
}
