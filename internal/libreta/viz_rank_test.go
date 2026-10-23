package libreta

import "testing"

func TestVIZDecodeRankCode(t *testing.T) {
	// letture reali del motore OCR (ricavate dal laboratorio) per due sigle, e il loro valore atteso
	for in, want := range map[string]string{
		"11/2": "II/2", "1112": "II/2", "II/2": "II/2", "11.12": "II/2", "11,'2": "II/2", "lI/2": "II/2", "Il/2": "II/2",
		"IV/2": "IV/2", "N/2": "IV/2", "N12": "IV/2", "NT2": "IV/2", "IV12": "IV/2", "TVT2": "IV/2", "tvt2": "IV/2",
		"III/1": "III/1", "111/1": "III/1", "VI/3": "VI/3", "V/2": "V/2", "Vl/2": "VI/2",
	} {
		got, _, ok := vizDecodeRankCode(in)
		if !ok || got != want {
			t.Errorf("vizDecodeRankCode(%q) = %q (ok=%v), atteso %q", in, got, ok, want)
		}
	}
	// parole ordinarie e rumore: non sono sigle
	for _, in := range []string{"STAFF", "SEAMAN", "OPERATOR", "ORDINARY", "HOTEL", "MASTER", "CADET", "", "I", "WARD", "GENERAL", "STEWARD", "Itn", "UAZTER"} {
		if got, cost, ok := vizDecodeRankCode(in); ok {
			t.Errorf("vizDecodeRankCode(%q) = %q (costo %.2f): non e' una sigla di grado", in, got, cost)
		}
	}
}

func TestVIZDecodeRankTail(t *testing.T) {
	for in, want := range map[string]string{
		"MASTER 11/2":                  "MASTER II/2",
		"MASTER 1112":                  "MASTER II/2",
		"GMDSS GENERAL OPERATOR N12":   "GMDSS GENERAL OPERATOR IV/2",
		"GMDSS GENERAL OPERATOR tvt2":  "GMDSS GENERAL OPERATOR IV/2",
		"HOTEL STAFF":                  "HOTEL STAFF",
		"ORDINARY SEAMAN":              "ORDINARY SEAMAN",
		"11/2":                         "II/2",
		"CHIEF ENGINEER III/2":         "CHIEF ENGINEER III/2",
		"STEWARD":                      "STEWARD",
		"MASTER 11 /2":                 "MASTER II/2",
		"MASTER Itn":                   "MASTER Itn",
		"":                             "",
		"ENGINE CADET":                 "ENGINE CADET",
		"OFFICER IN CHARGE OF A WATCH": "OFFICER IN CHARGE OF A WATCH",
	} {
		got, _ := vizDecodeRankTail(in)
		if got != want {
			t.Errorf("vizDecodeRankTail(%q) = %q, atteso %q", in, got, want)
		}
	}
}

func TestVIZRankCodesGrammar(t *testing.T) {
	codes := vizAllRankCodes()
	if len(codes) != len(vizRomans)*9 {
		t.Fatalf("sigle %d, attese %d", len(codes), len(vizRomans)*9)
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if seen[c] {
			t.Errorf("sigla doppia %s", c)
		}
		seen[c] = true
	}
	for _, c := range []string{"II/2", "IV/2", "III/1", "VII/1", "X/9"} {
		if !seen[c] {
			t.Errorf("manca la sigla %s", c)
		}
	}
}
