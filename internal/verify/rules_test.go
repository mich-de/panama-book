package verify

import (
	"strings"
	"testing"
)

func TestCheckUscitaCoerenteSenzaIncoerenze(t *testing.T) {
	fixedNow(t)
	if fs := Check(summary(goodFields())); len(fs) != 0 {
		t.Errorf("uscita coerente: attese 0 incoerenze, trovate %v", fs)
	}
}

func TestCheckFormati(t *testing.T) {
	fixedNow(t)
	cases := []struct {
		name string
		kv   []string
		code string
	}{
		{"tipo", []string{"type", "IS"}, "FORMAT_TYPE"},
		{"paese", []string{"country_code", "PA"}, "FORMAT_COUNTRY_CODE"},
		{"libretto corto", []string{"seamans_book", "PA12345"}, "FORMAT_SEAMANS_BOOK"},
		{"libretto con lettera", []string{"seamans_book", "PA12S4567"}, "FORMAT_SEAMANS_BOOK"},
		{"id marino 8 cifre", []string{"seafarers_id", "12345678"}, "FORMAT_SEAFARERS_ID"},
		{"id marino con lettera", []string{"seafarers_id", "98765432X"}, "FORMAT_SEAFARERS_ID"},
		{"certificato 8 cifre", []string{"certificate", "12345678"}, "FORMAT_CERTIFICATE"},
		{"certificato con il vecchio nome", []string{"certificate", "-", "endorsement", "12345678"}, "FORMAT_CERTIFICATE"},
		{"identificazione corta", []string{"id_number", "AB1"}, "FORMAT_ID_NUMBER"},
		{"identificazione con spazio", []string{"id_number", "AB 123456"}, "FORMAT_ID_NUMBER"},
		{"identificazione lunga", []string{"id_number", "ABCDEFGHIJKLM"}, "FORMAT_ID_NUMBER"},
		{"livello", []string{"level", "SUPPORTX"}, "FORMAT_LEVEL"},
	}
	for _, c := range cases {
		fs := Check(summary(with(goodFields(), c.kv...)))
		if !hasCode(fs, c.code) {
			t.Errorf("%s: manca %s in %v", c.name, c.code, fs)
		}
	}
	// formati ammessi
	for _, kv := range [][]string{
		{"seamans_book", "P1234567", "seamans_book_mrz", "P1234567"}, // modelli 2017 e 2020
		{"id_number", "1-234-5678"},                                  // con trattini
		{"id_number", "K123456"},                                     // lettera e cifre
		{"id_number", "Q7XLB4TKW"},                                   // alfanumerico di 9
		{"level", "OPERATIONAL"}, {"level", "MANAGEMENT"},
		{"certificate", ""}, // assente: il campo c'e' solo su alcuni titolari, non e' un errore
	} {
		fields := with(goodFields(), kv...)
		if fs := Check(summary(fields)); len(fs) != 0 {
			t.Errorf("%v: attese 0 incoerenze, trovate %v", kv, fs)
		}
	}
}

func TestCheckDate(t *testing.T) {
	fixedNow(t)
	cases := []struct {
		name string
		kv   []string
		code string
	}{
		{"data non valida", []string{"date_of_issue", "31-02-2026"}, "FORMAT_DATE"},
		{"formato senza zeri", []string{"date_of_issue", "5-3-2026"}, "FORMAT_DATE"},
		{"formato ISO al posto di quello stampato", []string{"date_of_expiry", "2031-03-04"}, "FORMAT_DATE"},
		{"nascita dopo emissione", []string{"date_of_birth", "06-03-2026", "date_of_birth_mrz", "2026-03-06"}, "DATE_BIRTH_NOT_BEFORE_ISSUE"},
		{"scadenza prima di emissione", []string{"date_of_expiry", "04-03-2025", "date_of_expiry_mrz", "2025-03-04"}, "DATE_EXPIRY_NOT_AFTER_ISSUE"},
		{"scadenza uguale a emissione", []string{"date_of_expiry", "05-03-2026", "date_of_expiry_mrz", "2026-03-05"}, "DATE_EXPIRY_NOT_AFTER_ISSUE"},
		{"validita' oltre 5 anni", []string{"date_of_issue", "05-03-2023", "date_of_issue_iso", "2023-03-05"}, "DATE_VALIDITY_IMPLAUSIBLE"},
		{"validita' di pochi giorni", []string{"date_of_expiry", "20-03-2026", "date_of_expiry_mrz", "2026-03-20"}, "DATE_VALIDITY_IMPLAUSIBLE"},
		{"titolare minorenne", []string{"date_of_birth", "01-01-2015", "date_of_birth_mrz", "2015-01-01"}, "DATE_YEAR_RANGE"},
		{"nascita troppo vecchia", []string{"date_of_birth", "01-01-1900", "date_of_birth_mrz", "1900-01-01"}, "DATE_YEAR_RANGE"},
		{"scadenza troppo lontana", []string{"date_of_expiry", "04-03-2060", "date_of_expiry_mrz", "2060-03-04"}, "DATE_YEAR_RANGE"},
		{"emissione nel futuro lontano", []string{"date_of_issue", "05-03-2040"}, "DATE_YEAR_RANGE"},
		{"forma ISO diversa", []string{"date_of_issue_iso", "2026-03-06"}, "DATE_ISO_MISMATCH"},
	}
	for _, c := range cases {
		fs := Check(summary(with(goodFields(), c.kv...)))
		if !hasCode(fs, c.code) {
			t.Errorf("%s: manca %s in %v", c.name, c.code, fs)
		}
	}
	// 5 mesi di validita' (visti sui libretti 2026) e 5 anni esatti sono plausibili
	for _, kv := range [][]string{
		{"date_of_expiry", "17-02-2027", "date_of_expiry_mrz", "2027-02-17", "date_of_expiry_iso", "2027-02-17", "date_of_issue", "16-09-2026", "date_of_issue_iso", "2026-09-16"},
		{"date_of_expiry", "05-03-2031", "date_of_expiry_mrz", "2031-03-05", "date_of_expiry_iso", "2031-03-05"},
	} {
		if fs := Check(summary(with(goodFields(), kv...))); len(fs) != 0 {
			t.Errorf("%v: attese 0 incoerenze, trovate %v", kv, fs)
		}
	}
}

func TestCheckGravitaDelleDate(t *testing.T) {
	fixedNow(t)
	fs := Check(summary(with(goodFields(), "date_of_expiry", "04-03-2025", "date_of_expiry_mrz", "2025-03-04")))
	f, ok := findCode(fs, "DATE_EXPIRY_NOT_AFTER_ISSUE")
	if !ok || f.Severity != SevError {
		t.Errorf("scadenza prima dell'emissione: %+v", f)
	}
	fs = Check(summary(with(goodFields(), "date_of_issue", "05-03-2023", "date_of_issue_iso", "2023-03-05")))
	if f, ok := findCode(fs, "DATE_VALIDITY_IMPLAUSIBLE"); !ok || f.Severity != SevWarn {
		t.Errorf("validita' implausibile e' un avviso, non un errore: %+v", f)
	}
}

func TestCheckCaratteriAnomali(t *testing.T) {
	fixedNow(t)
	cases := []struct {
		name, field, value, code string
	}{
		{"lettera accentata", "place_of_birth", "PANAMÁ ITA", "CHAR_ACCENT"},
		{"simbolo", "place_of_birth", "ROMA|ITA", "CHAR_SYMBOLS"},
		{"minuscole", "capacity", "Ordinary Seaman", "CHAR_LOWERCASE"},
		{"carattere di controllo", "regional", "PANAMA\x01DR", "CHAR_NON_PRINTABLE"},
		{"simbolo non ASCII", "regional", "PANAMA • DR", "CHAR_NON_PRINTABLE"},
		{"spazi doppi", "place_of_birth", "ROMA  ITA", "FIELD_SPACES"},
		{"spazio finale", "regional", "PANAMA DR ", "FIELD_SPACES"},
		{"cargo troppo corto", "capacity", "MAS", "FIELD_TOO_SHORT"},
		{"luogo troppo corto", "place_of_birth", "RO", "FIELD_TOO_SHORT"},
		{"nazionalita' troppo corta", "nationality", "ITA", "FIELD_TOO_SHORT"},
		{"cifre nel luogo", "place_of_birth", "ROMA 12", "FIELD_HAS_DIGITS"},
		{"cifra dentro una parola del cargo", "capacity", "CY9D HOTEL", "CAPACITY_SUSPICIOUS_TOKEN"},
		{"parola di una lettera nel cargo", "capacity", "ORDINARY SEAMAN L", "FIELD_NOISE_TOKEN"},
		{"parola di una lettera nel luogo", "place_of_birth", "ROMA Q", "FIELD_NOISE_TOKEN"},
	}
	for _, c := range cases {
		fs := Check(summary(with(goodFields(), c.field, c.value)))
		f, ok := findCode(fs, c.code)
		if !ok {
			t.Errorf("%s: manca %s in %v", c.name, c.code, fs)
			continue
		}
		if len(f.Fields) == 0 || f.Fields[0] != c.field {
			t.Errorf("%s: il campo coinvolto dovrebbe essere %s, e' %v", c.name, c.field, f.Fields)
		}
	}
	// il simbolo non ASCII e' un errore, l'accento e' un avviso
	fs := Check(summary(with(goodFields(), "regional", "PANAMA\x01DR", "place_of_birth", "PANAMÁ")))
	if f, _ := findCode(fs, "CHAR_NON_PRINTABLE"); f.Severity != SevError {
		t.Errorf("CHAR_NON_PRINTABLE dovrebbe essere un errore: %+v", f)
	}
	if f, _ := findCode(fs, "CHAR_ACCENT"); f.Severity != SevWarn {
		t.Errorf("CHAR_ACCENT dovrebbe essere un avviso: %+v", f)
	}
}

func TestCheckGradiDelCargoAmmessi(t *testing.T) {
	fixedNow(t)
	for _, v := range []string{"SAMPLE OFFICER II/2", "SAMPLE OPERATOR IV/2", "SAMPLE ENGINEER A-VI/2", "SAMPLE DECK III", "SAMPLE CADET 1/2"} {
		if fs := Check(summary(with(goodFields(), "capacity", v))); len(fs) != 0 {
			t.Errorf("%q: attese 0 incoerenze, trovate %v", v, fs)
		}
	}
}

func TestCheckNazionalita(t *testing.T) {
	fixedNow(t)
	cases := []struct {
		name string
		kv   []string
		code string
		sev  Severity
	}{
		{"aggettivo di un altro paese", []string{"nationality", "GERMAN"}, "NATIONALITY_ISO_MISMATCH", SevError},
		{"testo della riga del nome", []string{"nationality", "ROSSI MARIO"}, "NATIONALITY_LOOKS_LIKE_NAME", SevError},
		{"nome storpiato", []string{"nationality", "ROSSL MARIQ"}, "NATIONALITY_LOOKS_LIKE_NAME", SevError},
		{"non e' un aggettivo", []string{"nationality", "ZZZZIAN"}, "NATIONALITY_NOT_IN_TABLE", SevError},
		{"codice ISO fuori tabella", []string{"nationality_iso", "ZZZ"}, "NATIONALITY_ISO_UNKNOWN", SevInfo},
	}
	for _, c := range cases {
		fs := Check(summary(with(goodFields(), c.kv...)))
		f, ok := findCode(fs, c.code)
		if !ok || f.Severity != c.sev {
			t.Errorf("%s: %s (gravita' %v) non trovato in %v", c.name, c.code, c.sev, fs)
		}
	}
	// senza codice ISO (MRZ non trovato): solo un avviso, e solo se non e' un aggettivo noto
	g := with(goodFields(), "nationality_iso", "-", "surname", "-", "given_names", "-")
	if fs := Check(noMRZ(g)); hasCode(fs, "NATIONALITY_NOT_IN_TABLE") || hasCode(fs, "NATIONALITY_ISO_MISMATCH") {
		t.Errorf("aggettivo noto senza codice: %v", fs)
	}
	g = with(g, "nationality", "ZZZZIAN")
	if f, ok := findCode(Check(noMRZ(g)), "NATIONALITY_NOT_IN_TABLE"); !ok || f.Severity != SevWarn {
		t.Errorf("aggettivo ignoto senza codice: %+v", f)
	}
	// un aggettivo doppio della tabella e' accettato (KOR: KOREAN e SOUTH KOREAN)
	for _, adj := range []string{"KOREAN", "SOUTH KOREAN"} {
		if fs := Check(summary(with(goodFields(), "nationality", adj, "nationality_iso", "KOR"))); len(fs) != 0 {
			t.Errorf("KOR %q: %v", adj, fs)
		}
	}
}

func TestCheckConfrontoConMRZ(t *testing.T) {
	fixedNow(t)
	cases := []struct {
		name string
		kv   []string
		code string
		sev  Severity
	}{
		{"nascita", []string{"date_of_birth", "02-01-1990"}, "DATE_OF_BIRTH_DIFFERS_FROM_MRZ", SevError},
		{"scadenza", []string{"date_of_expiry", "05-03-2031"}, "DATE_OF_EXPIRY_DIFFERS_FROM_MRZ", SevError},
		{"numero libretto", []string{"seamans_book", "PA1234568"}, "SEAMANS_BOOK_DIFFERS_FROM_MRZ", SevError},
		{"regionale diverso", []string{"regional", "NAPOLI"}, "REGIONAL_DIFFERS_FROM_MRZ", SevWarn},
		{"sesso", []string{"sex", "Q"}, "SEX_UNKNOWN_VALUE", SevWarn},
	}
	for _, c := range cases {
		fs := Check(summary(with(goodFields(), c.kv...)))
		f, ok := findCode(fs, c.code)
		if !ok || f.Severity != c.sev {
			t.Errorf("%s: %s non trovato (o gravita' sbagliata) in %v", c.name, c.code, fs)
		}
	}
	// il regionale stampato puo' essere piu' lungo dell'opzionale dell'MRZ ("PANAMA DR" per "PANAMA") o uguale ("NAPOLI")
	for _, kv := range [][]string{{}, {"regional", "NAPOLI", "regional_mrz", "NAPOLI"}} {
		if fs := Check(summary(with(goodFields(), kv...))); hasCode(fs, "REGIONAL_DIFFERS_FROM_MRZ") {
			t.Errorf("%v: regionale coerente segnalato: %v", kv, fs)
		}
	}
	// check digit non validi: avviso, ma i confronti si fanno lo stesso
	s := summary(with(goodFields(), "seamans_book", "PA1234568"))
	s.Checks[0] = false
	fs := Check(s)
	if f, ok := findCode(fs, "MRZ_CHECK_DIGITS_INVALID"); !ok || f.Severity != SevWarn {
		t.Errorf("check digit non validi: %v", fs)
	}
	if !hasCode(fs, "SEAMANS_BOOK_DIFFERS_FROM_MRZ") {
		t.Errorf("il confronto con l'MRZ va fatto anche con i check digit non validi: %v", fs)
	}
	// senza MRZ: una sola nota
	fs = Check(noMRZ(with(goodFields(), "date_of_birth_mrz", "-", "date_of_expiry_mrz", "-", "seamans_book_mrz", "-", "regional_mrz", "-")))
	if f, ok := findCode(fs, "MRZ_NOT_FOUND"); !ok || f.Severity != SevInfo {
		t.Errorf("MRZ assente: %v", fs)
	}
	if hasCode(fs, "DATE_OF_BIRTH_DIFFERS_FROM_MRZ") {
		t.Errorf("senza MRZ non si confronta nulla: %v", fs)
	}
}

func TestCheckNonStampaValori(t *testing.T) {
	fixedNow(t)
	bad := with(goodFields(), "nationality", "ROSSI MARIO", "place_of_birth", "ZQXJ|ZQXJ", "capacity", "CY9D", "id_number", "AB")
	for _, f := range Check(summary(bad)) {
		for _, secret := range []string{"ROSSI", "MARIO", "ZQXJ", "CY9D", "1234567", "ITALIAN"} {
			if strings.Contains(f.Text, secret) || strings.Contains(f.Code, secret) {
				t.Errorf("%s contiene un valore (%q): %s", f.Code, secret, f.Text)
			}
		}
	}
}

func TestSeverityString(t *testing.T) {
	if SevInfo.String() != "INFO" || SevWarn.String() != "AVVISO" || SevError.String() != "ERRORE" {
		t.Errorf("etichette: %s %s %s", SevInfo, SevWarn, SevError)
	}
}

func TestTabellaNazionalita(t *testing.T) {
	if n := len(nationalityAdjectives); n != 199 {
		t.Errorf("la copia della tabella ha %d codici, attesi 199 (come internal/libreta/viz_nationality.go)", n)
	}
	want := map[string]string{"PAN": "PANAMANIAN", "ITA": "ITALIAN", "NIC": "NICARAGUAN", "DEU": "GERMAN", "TUN": "TUNISIAN", "MUS": "MAURITIAN", "PHL": "FILIPINO", "GBR": "BRITISH"}
	for code, adj := range want {
		got := nationalityAdjectives[code]
		if len(got) == 0 || got[0] != adj {
			t.Errorf("%s: %v, atteso %s per primo", code, got, adj)
		}
	}
	if len(nationalityAdjectives["KOR"]) != 2 {
		t.Errorf("KOR dovrebbe avere due aggettivi: %v", nationalityAdjectives["KOR"])
	}
	if !isNationalityAdjective("SOUTHKOREAN") || isNationalityAdjective("ZZZZIAN") {
		t.Error("isNationalityAdjective usa la forma normalizzata")
	}
}
