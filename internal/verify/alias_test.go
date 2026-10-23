package verify

import (
	"encoding/json"
	"strings"
	"testing"

	"scanlibretto/internal/outsum"
)

// Il campo certificato (`Nº Título / Certificate`) si chiama `certificate`; `endorsement` e' il vecchio nome, ancora
// accettato in lettura. I test non dipendono da quale dei due nomi e' in uso in outsum.FieldKeys (l'integratore lo
// rinomina per conto suo): usano certKey() e altCertKey(). Dati inventati.

func TestCanonicalEAlias(t *testing.T) {
	if Canonical("endorsement") != "certificate" || Canonical("certificate") != "certificate" || Canonical("capacity") != "capacity" {
		t.Error("Canonical")
	}
	if certKey() == "" {
		t.Fatal("outsum.FieldKeys non ha il campo certificato")
	}
	if got := lookup(map[string]string{"endorsement": "111"}, "certificate"); got != "111" {
		t.Errorf("lookup del nome nuovo con la chiave vecchia: %q", got)
	}
	if got := lookup(map[string]string{"certificate": "222"}, "endorsement"); got != "222" {
		t.Errorf("lookup del nome vecchio con la chiave nuova: %q", got)
	}
	if got := lookup(map[string]string{"certificate": "222", "endorsement": "111"}, "certificate"); got != "222" {
		t.Errorf("se ci sono entrambe vale il nome chiesto: %q", got)
	}
	if got := lookup(map[string]string{"certificate": ""}, "endorsement"); got != "" {
		t.Errorf("vuoto: %q", got)
	}
	if got := lookup(map[string]string{"capacity": "X"}, "level"); got != "" {
		t.Errorf("campo assente: %q", got)
	}
}

func TestVeritaConIlVecchioNome(t *testing.T) {
	// la verita' scritta con `endorsement` (e la nota con la chiave vecchia) si legge come `certificate`
	b := []byte(`{"type": "SI", "endorsement": "123456789", "_note": {"endorsement": "nota"}}`)
	tr, err := ParseTruth(b)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Field("certificate") != "123456789" || tr.Field("endorsement") != "123456789" || tr.Fields[certKey()] != "123456789" {
		t.Errorf("campo letto con il vecchio nome: %v", tr.Fields)
	}
	if tr.Notes["certificate"] != "nota" {
		t.Errorf("la nota con la chiave vecchia va sotto il nome nuovo: %v", tr.Notes)
	}
	for _, k := range tr.Absent {
		if Canonical(k) == FieldCertificate {
			t.Errorf("il certificato e' presente nel file ma risulta assente")
		}
	}
	// dentro "fields" e con il nome nuovo
	tr, err = ParseTruth([]byte(`{"fields": {"certificate": "987654321"}}`))
	if err != nil || tr.Field("endorsement") != "987654321" {
		t.Errorf("nome nuovo dentro fields: %v %v", tr.Fields, err)
	}
	// se ci sono entrambi vale quello in uso in outsum.FieldKeys
	both := map[string]string{certKey(): "111111111", altCertKey(): "222222222"}
	raw, _ := json.Marshal(both)
	tr, err = ParseTruth(raw)
	if err != nil || tr.Fields[certKey()] != "111111111" {
		t.Errorf("entrambi i nomi: %v %v", tr.Fields, err)
	}
	// un valore che non e' una stringa e' un errore che cita il nome del campo e non il valore
	if _, err := ParseTruth([]byte(`{"endorsement": 5}`)); err == nil || !strings.Contains(err.Error(), certKey()) {
		t.Errorf("errore sul certificato: %v", err)
	}
}

func TestUscitaConIlVecchioNome(t *testing.T) {
	for _, name := range []string{"endorsement", "certificate"} {
		fields := with(goodFields(), "certificate", "-")
		fields[name] = "123456789"
		sum, err := ParseOutput(outputJSON(t, fields, true))
		if err != nil {
			t.Fatal(err)
		}
		if sum.Fields[certKey()] != "123456789" {
			t.Errorf("%s: il valore non e' sotto %s: %v", name, certKey(), sum.Fields)
		}
		if sum.Found() != outsum.NFields {
			t.Errorf("%s: campi letti %d, attesi %d (i metodi di outsum devono vedere il certificato)", name, sum.Found(), outsum.NFields)
		}
	}
}

func TestConfrontoConUnNomeDiversoPerParte(t *testing.T) {
	// uscita con un nome e verita' con l'altro, in entrambi i sensi: il campo e' giusto, e Compare non richiede
	// che i nomi coincidano (cmd/regress passa un Summary di outsum.Parse senza normalizzare)
	for _, c := range []struct{ got, want string }{{certKey(), altCertKey()}, {altCertKey(), certKey()}} {
		got := with(goodFields(), "certificate", "-")
		got[c.got] = "123456789"
		truth := Truth{Fields: map[string]string{}}
		for _, k := range outsum.FieldKeys {
			truth.Fields[k] = lookup(goodFields(), k)
		}
		delete(truth.Fields, certKey())
		truth.Fields[c.want] = "123456789"
		rs := Compare(summary(got), truth)
		if r := statusOf(rs, "certificate"); r.Status != StatusOK {
			t.Errorf("uscita con %q e verita' con %q: %s", c.got, c.want, r.Status)
		}
		// e uno sbagliato resta sbagliato
		got[c.got] = "123456780"
		if r := statusOf(Compare(summary(got), truth), "certificate"); r.Status != StatusWrong {
			t.Errorf("certificato diverso: %s", r.Status)
		}
	}
}

func TestCertificatoAssenteNonEUnErrore(t *testing.T) {
	fixedNow(t)
	// il campo c'e' solo su alcuni titolari: verita' vuota e niente letto = NON_STAMPATO, nessuna incoerenza
	got := with(goodFields(), "certificate", "-")
	tr := truthOf(with(goodFields(), "certificate", ""))
	res := Verify("senza certificato", summary(got), &tr)
	if st := statusOf(res.Fields, "certificate").Status; st != StatusNotPrinted {
		t.Errorf("certificato assente: %s, atteso NON_STAMPATO", st)
	}
	if res.Failed(true) || len(res.Findings) != 0 {
		t.Errorf("un certificato assente non e' un errore: %+v", res.Findings)
	}
	// e se la verita' lo ha e il programma non lo legge e' MANCANTE (un difetto vero)
	tr2 := truthOf(goodFields())
	if st := statusOf(Compare(summary(got), tr2), "certificate").Status; st != StatusMissing {
		t.Errorf("certificato presente ma non letto: %s, atteso MANCANTE", st)
	}
}

func TestIReportStampanoIlNomeAttuale(t *testing.T) {
	fixedNow(t)
	// qualunque sia il nome in uso in outsum, i report dicono `certificate` e mai `endorsement`
	got := with(goodFields(), "certificate", "12345678") // 8 cifre: anche un'incoerenza
	tr := truthOf(goodFields())
	res := Verify("immagine-sintetica", summary(got), &tr)
	var b strings.Builder
	WriteResult(&b, res)
	WriteDir(&b, DirReport{Items: []Result{res}}, false)
	WriteDir(&b, DirReport{Items: []Result{res}}, true)
	out := b.String()
	if !strings.Contains(out, "certificate") {
		t.Errorf("il report non nomina il certificato:\n%s", out)
	}
	if strings.Contains(out, "endorsement") || strings.Contains(out, "ENDORSEMENT") {
		t.Errorf("il report usa il vecchio nome:\n%s", out)
	}
	if !strings.Contains(out, "FORMAT_CERTIFICATE") {
		t.Errorf("manca FORMAT_CERTIFICATE:\n%s", out)
	}
}
