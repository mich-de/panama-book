package outsum

import (
	"reflect"
	"testing"
)

// Uscita sintetica (nessun dato reale): MRZ con 4 check digit su 5, 12 campi stampati su 14, 2 avvisi.
const jsonSintetico = `{
  "schema_version": 1,
  "rotation_deg": 180,
  "mrz": {"checks": {"doc_number": true, "birth": true, "expiry": false, "optional": true, "composite": true}},
  "fields": {"type": "SI", "country_code": "PAN", "nationality": "X", "place_of_birth": "X", "date_of_issue": "X",
    "date_of_expiry": "X", "date_of_birth": "X", "level": "X", "capacity": "", "seamans_book": "X", "id_number": "X",
    "certificate": "", "seafarers_id": "X", "regional": "X", "surname": "X"},
  "warnings": [{"code": "MRZ_CHECK_DIGITS_INVALID"}, {"code": "PRINTED_FIELDS_NOT_READ", "fields": ["capacity"]}]
}`

func TestParseRiassume(t *testing.T) {
	s, err := Parse([]byte(jsonSintetico))
	if err != nil {
		t.Fatal(err)
	}
	if s.Rotation != 180 || !s.HasMRZ || s.Warnings != 2 {
		t.Errorf("rotazione/MRZ/avvisi: %+v", s)
	}
	if s.ChecksOK() != 4 || s.Checks != [NChecks]bool{true, true, false, true, true} {
		t.Errorf("check digit: %v (ok=%d)", s.Checks, s.ChecksOK())
	}
	if s.Found() != 12 {
		t.Errorf("campi trovati %d, attesi 12", s.Found())
	}
	if got, want := s.Missing(), []string{"capacity", "certificate"}; !reflect.DeepEqual(got, want) {
		t.Errorf("mancanti %v, attesi %v", got, want)
	}
}

// Le uscite prodotte prima del cambio di nome hanno ancora "endorsement": Parse le legge come "certificate", cosi' un
// JSON salvato (libretto summary) o un eseguibile vecchio non sembrano aver perso il campo.
func TestParseAccettaLaVecchiaChiaveEndorsement(t *testing.T) {
	vecchio := `{"schema_version":1,"rotation_deg":0,"fields":{"type":"SI","endorsement":"123456789"}}`
	s, err := Parse([]byte(vecchio))
	if err != nil {
		t.Fatal(err)
	}
	if s.Fields["certificate"] != "123456789" || !s.Present()[11] {
		t.Errorf("vecchia chiave non letta come certificate: %v", s.Fields)
	}
	if _, resta := s.Fields["endorsement"]; resta {
		t.Errorf("la vecchia chiave resta nei campi: %v", s.Fields)
	}
	// con entrambe le chiavi vale quella nuova; il nuovo nome senza il vecchio non cambia
	s, _ = Parse([]byte(`{"fields":{"certificate":"111111111","endorsement":"222222222"}}`))
	if s.Fields["certificate"] != "111111111" {
		t.Errorf("con le due chiavi deve valere certificate: %v", s.Fields)
	}
	s, _ = Parse([]byte(`{"fields":{"certificate":"333333333"}}`))
	if s.Fields["certificate"] != "333333333" || s.Found() != 1 {
		t.Errorf("solo certificate: %v", s.Fields)
	}
	// uscita senza "fields": niente da tradurre e niente panic
	if s, err := Parse([]byte(`{"schema_version":1}`)); err != nil || s.Found() != 0 {
		t.Errorf("senza fields: %+v %v", s, err)
	}
}

func TestParseSenzaMRZ(t *testing.T) {
	s, err := Parse([]byte(`{"schema_version":1,"rotation_deg":0,"fields":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.HasMRZ || s.ChecksOK() != 0 || s.Found() != 0 || len(s.Missing()) != NFields {
		t.Errorf("senza MRZ e senza campi: %+v", s)
	}
}

func TestParseJSONNonValido(t *testing.T) {
	for _, in := range []string{"", "non json", "{"} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("Parse(%q) senza errore", in)
		}
	}
}

// I nomi dei campi e dei check devono restare quelli dell'uscita JSON di libretto (cmd/libretto/output.go).
func TestNomiFissi(t *testing.T) {
	if len(FieldKeys) != 14 || len(CheckKeys) != 5 {
		t.Fatalf("attesi 14 campi e 5 check, trovati %d e %d", len(FieldKeys), len(CheckKeys))
	}
	if FieldKeys[0] != "type" || FieldKeys[11] != "certificate" || FieldKeys[13] != "regional" || CheckKeys[0] != "doc_number" || CheckKeys[4] != "composite" {
		t.Errorf("ordine cambiato: %v %v", FieldKeys, CheckKeys)
	}
}
