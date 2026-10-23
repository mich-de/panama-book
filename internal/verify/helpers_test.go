package verify

import (
	"encoding/json"
	"testing"
	"time"

	"scanlibretto/internal/outsum"
)

// Tutti i dati dei test sono INVENTATI (nomi, numeri e date): nessun dato di persone reali.

// goodFields e' un'uscita di libretto coerente e senza difetti, con MRZ.
func goodFields() map[string]string {
	return map[string]string{
		"type": "SI", "country_code": "PAN", "nationality": "ITALIAN", "place_of_birth": "ROMA ITA",
		"date_of_issue": "05-03-2026", "date_of_expiry": "04-03-2031", "date_of_birth": "01-01-1990",
		"level": "SUPPORT", "capacity": "ORDINARY SEAMAN", "seamans_book": "PA1234567", "id_number": "AB1234567",
		"certificate": "123456789", "seafarers_id": "987654321", "regional": "PANAMA DR",
		"date_of_issue_iso": "2026-03-05", "date_of_expiry_iso": "2031-03-04", "date_of_birth_iso": "1990-01-01",
		"surname": "ROSSI", "given_names": "MARIO LUIGI", "nationality_iso": "ITA", "sex": "M",
		"date_of_birth_mrz": "1990-01-01", "date_of_expiry_mrz": "2031-03-04", "seamans_book_mrz": "PA1234567",
		"regional_mrz": "PANAMA",
	}
}

// with restituisce una copia di m con le modifiche di kv (chiave, valore, chiave, valore, ...); un valore "-" toglie la chiave.
func with(m map[string]string, kv ...string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] == "-" {
			delete(out, kv[i])
		} else {
			out[kv[i]] = kv[i+1]
		}
	}
	return out
}

// summary costruisce il riassunto di un'uscita con MRZ valido (5 check digit su 5).
func summary(fields map[string]string) outsum.Summary {
	return outsum.Summary{HasMRZ: true, Checks: [outsum.NChecks]bool{true, true, true, true, true}, Fields: fields}
}

// noMRZ costruisce il riassunto di un'uscita senza MRZ.
func noMRZ(fields map[string]string) outsum.Summary { return outsum.Summary{Fields: fields} }

// truthOf costruisce una verita' con i 14 campi di m (gli altri valori di m si ignorano). Il campo `certificate` si trova
// anche con il vecchio nome, quale che sia il nome in uso in outsum.FieldKeys.
func truthOf(m map[string]string) Truth {
	t := Truth{Fields: map[string]string{}, Notes: map[string]string{}}
	for _, k := range outsum.FieldKeys {
		t.Fields[k] = lookup(m, k)
	}
	return t
}

// certKey e' il nome del campo certificato in outsum.FieldKeys (`certificate`, oppure `endorsement` finche'
// l'integratore non rinomina l'elenco); altCertKey e' l'altro nome. I test non dipendono da quale dei due e' in uso.
func certKey() string {
	for _, k := range outsum.FieldKeys {
		if Canonical(k) == FieldCertificate {
			return k
		}
	}
	return ""
}

func altCertKey() string {
	if certKey() == FieldCertificate {
		return fieldEndorsement
	}
	return FieldCertificate
}

// outputJSON serializza un'uscita sintetica di libretto (schema 1) con MRZ valido.
func outputJSON(t *testing.T, fields map[string]string, mrz bool) []byte {
	t.Helper()
	out := map[string]any{"schema_version": 1, "rotation_deg": 0, "fields": fields}
	if mrz {
		out["mrz"] = map[string]any{"checks": map[string]bool{"doc_number": true, "birth": true, "expiry": true, "optional": true, "composite": true}}
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// truthJSON serializza una verita' sintetica (14 campi al livello superiore).
func truthJSON(t *testing.T, m map[string]string, extra map[string]any) []byte {
	t.Helper()
	out := map[string]any{}
	for _, k := range outsum.FieldKeys {
		out[k] = lookup(m, k)
	}
	for k, v := range extra {
		out[k] = v
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fixedNow ferma l'orologio dei controlli sugli anni (26 ottobre 2026) per la durata del test.
func fixedNow(t *testing.T) {
	t.Helper()
	old := now
	now = func() time.Time { return time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = old })
}

// hasCode dice se tra le incoerenze c'e' il codice.
func hasCode(fs []Finding, code string) bool {
	for _, f := range fs {
		if f.Code == code {
			return true
		}
	}
	return false
}

func findCode(fs []Finding, code string) (Finding, bool) {
	for _, f := range fs {
		if f.Code == code {
			return f, true
		}
	}
	return Finding{}, false
}

// statusOf restituisce lo stato di un campo in un confronto.
func statusOf(rs []FieldResult, field string) FieldResult {
	for _, r := range rs {
		if Canonical(r.Field) == Canonical(field) {
			return r
		}
	}
	return FieldResult{Field: field, Status: -1}
}
