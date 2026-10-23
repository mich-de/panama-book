package libreta

import (
	"strings"
	"testing"

	"scanlibretto/internal/mrz"
)

// Riga 2 sintetica (persona e numeri inventati) con tutti i check digit calcolati.
func tplSyntheticLine2(doc, nat, birth, sex, exp, opt string) string {
	l := doc + string(mrz.CheckDigit(doc)) + nat + birth + string(mrz.CheckDigit(birth)) + sex + exp + string(mrz.CheckDigit(exp)) +
		opt + string(mrz.CheckDigit(opt))
	comp := l[0:10] + l[13:20] + l[21:43]
	return l + string(mrz.CheckDigit(comp))
}

const tplSyntheticLine1 = "ISPANROSSI<<MARIO<LUIGI<<<<<<<<<<<<<<<<<<<<<"

// tplSyntheticLP costruisce punteggi: il simbolo vero vale -0,2, gli altri -12; per ogni cella di wrong il classificatore
// preferisce il simbolo indicato (-0,1) e il vero scende a -1,2.
func tplSyntheticLP(truth string, wrong map[int]byte) *tplLogp {
	var lp tplLogp
	for k := range lp {
		for s := range lp[k] {
			lp[k][s] = -12
		}
		lp[k][tplSym(truth[k])] = -0.2
		if w, ok := wrong[k]; ok {
			lp[k][tplSym(truth[k])] = -1.2
			lp[k][tplSym(w)] = -0.1
		}
	}
	return &lp
}

func TestTplDecodeLine1Grammatica(t *testing.T) {
	if len(tplSyntheticLine1) != tplCells {
		t.Fatalf("riga 1 sintetica lunga %d", len(tplSyntheticLine1))
	}
	// una cifra letta al posto di una lettera (S/5) e un '<' letto come lettera nel riempimento (K): la grammatica
	// riporta le lettere e il riempimento
	lp := tplSyntheticLP(tplSyntheticLine1, map[int]byte{9: '5', 40: 'K'})
	d := tplDecodeLine1(lp)
	if !d.ok || d.text != tplSyntheticLine1 {
		t.Errorf("riga 1: %q, attesa %q", d.text, tplSyntheticLine1)
	}
	if d.post < 0.5 {
		t.Errorf("probabilita' a posteriori bassa: %.3f", d.post)
	}
	// il raw argmax invece sbaglia
	if raw := tplArgmax(lp); raw == tplSyntheticLine1 {
		t.Error("l'argmax non dovrebbe coincidere: il test non prova nulla")
	}
}

func TestTplDecodeLine1CognomeComposto(t *testing.T) {
	l1 := "ISPANDE<LA<CRUZ<<ANA<MARIA" + strings.Repeat("<", 44-26)
	d := tplDecodeLine1(tplSyntheticLP(l1, nil))
	if !d.ok || d.text != l1 {
		t.Errorf("riga 1 con cognome composto: %q", d.text)
	}
}

func TestTplDecodeLine2CheckDigit(t *testing.T) {
	l2 := tplSyntheticLine2("PA1234567", "NIC", "850412", "M", "310315", "PANAMA<<<<<<<<")
	if len(l2) != tplCells {
		t.Fatalf("riga 2 sintetica lunga %d", len(l2))
	}
	if dd, _ := mrz.Parse(tplSyntheticLine1, l2); !dd.Checks.AllOK() {
		t.Fatalf("la riga 2 sintetica non ha i check validi: %+v", dd.Checks)
	}
	// nessun errore
	if d := tplDecodeLine2(tplSyntheticLP(l2, nil)); !d.ok || d.text != l2 || d.post < 0.9 {
		t.Errorf("senza errori: %q post %.3f", d.text, d.post)
	}
	// 8 letto al posto di 3 nel numero (3 cifre simili: lo risolve solo il check), 6 al posto di 5 nella nascita,
	// 0 al posto di O nel campo opzionale, 7 al posto di 1 nella scadenza: un errore per campo
	for name, wrong := range map[string]map[int]byte{
		"numero":    {4: '8'},
		"nascita":   {15: '5'},
		"scadenza":  {22: '7'},
		"opzionale": {29: '0'},
	} {
		got := tplDecodeLine2(tplSyntheticLP(l2, wrong))
		if !got.ok || got.text != l2 {
			t.Errorf("%s: %q, atteso %q", name, got.text, l2)
		}
	}
	// piu' errori insieme in campi diversi
	got := tplDecodeLine2(tplSyntheticLP(l2, map[int]byte{4: '8', 15: '5', 22: '7', 29: '0'}))
	if got.text != l2 {
		t.Errorf("errori multipli: %q", got.text)
	}
}

func TestTplDecodeLine2Tipi(t *testing.T) {
	l2 := tplSyntheticLine2("PA7654321", "PHL", "900229", "F", "290930", "NAPOLES<<<<<<<")
	// lettera al posto di cifra (O/0, B/8) e M letta come H: il tipo della posizione le corregge anche senza check
	d := tplDecodeLine2(tplSyntheticLP(l2, map[int]byte{3: 'O', 6: 'B', 20: 'H', 11: '1'}))
	if d.text != l2 {
		t.Errorf("tipi: %q, atteso %q", d.text, l2)
	}
	// nazionalita' fuori lista: il prior sui codici ICAO sceglie quella valida piu' vicina
	l2n := tplSyntheticLine2("PA1111111", "UKR", "850412", "M", "310315", "PANAMA<<<<<<<<")
	d = tplDecodeLine2(tplSyntheticLP(l2n, map[int]byte{12: 'N'})) // UKN non esiste
	if d.text != l2n {
		t.Errorf("nazionalita': %q, atteso %q", d.text, l2n)
	}
}

func TestTplDecodeLine2OpzionaleVuoto(t *testing.T) {
	// campo opzionale tutto '<': il check puo' essere '<' (e vale 0)
	opt := strings.Repeat("<", 14)
	l := tplSyntheticLine2("PA1234567", "NIC", "850412", "M", "310315", opt)
	d := tplDecodeLine2(tplSyntheticLP(l, nil))
	if d.text != l {
		t.Errorf("opzionale vuoto: %q, atteso %q", d.text, l)
	}
	// stessa riga con check '<' al posto di '0'
	b := []byte(l)
	b[42] = '<'
	b[43] = mrz.CheckDigit(string(b[0:10]) + string(b[13:20]) + string(b[21:43]))
	l = string(b)
	d = tplDecodeLine2(tplSyntheticLP(l, nil))
	if d.text != l {
		t.Errorf("opzionale vuoto con check '<': %q, atteso %q", d.text, l)
	}
	if dd, _ := mrz.Parse(tplSyntheticLine1, d.text); !dd.Checks.AllOK() {
		t.Errorf("check non validi dopo la decodifica: %+v", dd.Checks)
	}
}

// Quando l'evidenza e' ambigua la probabilita' a posteriori deve scendere (due cifre indistinguibili nello stesso campo).
func TestTplDecodeLine2AmbiguaAbbassaLaProbabilita(t *testing.T) {
	l2 := tplSyntheticLine2("PA1234567", "NIC", "850412", "M", "310315", "PANAMA<<<<<<<<")
	lp := tplSyntheticLP(l2, nil)
	// celle 5 e 6 (cifre del numero): il classificatore e' indifferente tra la cifra vera e un'altra
	for _, k := range []int{5, 6} {
		for s := range lp[k] {
			if tplIsDigit(s) {
				lp[k][s] = -2.3
			}
		}
	}
	d := tplDecodeLine2(lp)
	if d.post > 0.9 {
		t.Errorf("evidenza ambigua ma probabilita' %.3f", d.post)
	}
}
