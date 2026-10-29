// Package outsum riassume in numeri il JSON prodotto da `libretto extract` e `libretto read`, senza stampare dati
// personali. Lo usano il comando `libretto summary` e l'harness di regressione `cmd/regress`: l'elenco dei 14 campi
// stampati attesi e dei 5 check digit sta cosi' in un posto solo.
package outsum

import "encoding/json"

// Numero di campi stampati attesi e di check digit dell'MRZ.
const (
	NFields = 14
	NChecks = 5
)

// FieldKeys sono i 14 campi stampati attesi, con i nomi del JSON in uscita (stessi di cmd/libretto/output.go): gli stessi
// delle etichette stampate sulla pagina dati. "certificate" e' il "Nº Título / Certificate" (9 cifre, solo modello 2026).
var FieldKeys = [NFields]string{"type", "country_code", "nationality", "place_of_birth", "date_of_issue", "date_of_expiry",
	"date_of_birth", "level", "capacity", "seamans_book", "id_number", "certificate", "seafarers_id", "regional"}

// legacyFieldKeys sono i vecchi nomi di campo che Parse accetta ancora in lettura, per le uscite JSON prodotte prima del
// cambio di nome (vecchio -> nuovo): "endorsement" e' diventato "certificate". [TEMPORANEO] da togliere quando non
// restano uscite con il nome vecchio (per esempio quelle salvate in scans\).
var legacyFieldKeys = map[string]string{"endorsement": "certificate"}

// CheckKeys sono i 5 check digit dell'MRZ, nell'ordine ICAO (chiavi di `mrz.checks`).
var CheckKeys = [NChecks]string{"doc_number", "birth", "expiry", "optional", "composite"}

// Summary e' quello che serve sapere di un'uscita JSON. Fields contiene i valori letti, cioe' dati personali:
// va usato per i confronti in memoria, mai stampato.
type Summary struct {
	Rotation int
	HasMRZ   bool
	Checks   [NChecks]bool // validita' dei check digit, nell'ordine di CheckKeys
	Fields   map[string]string
	Warnings int // numero di avvisi
}

// Parse legge il JSON di extract o read.
func Parse(b []byte) (Summary, error) {
	var r struct {
		RotationDeg int `json:"rotation_deg"`
		MRZ         *struct {
			Checks map[string]bool `json:"checks"`
		} `json:"mrz"`
		Fields   map[string]string `json:"fields"`
		Warnings []json.RawMessage `json:"warnings"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return Summary{}, err
	}
	for old, cur := range legacyFieldKeys {
		if v, ok := r.Fields[old]; ok {
			if r.Fields[cur] == "" {
				r.Fields[cur] = v
			}
			delete(r.Fields, old)
		}
	}
	s := Summary{Rotation: r.RotationDeg, Fields: r.Fields, Warnings: len(r.Warnings)}
	if r.MRZ != nil {
		s.HasMRZ = true
		for i, k := range CheckKeys {
			s.Checks[i] = r.MRZ.Checks[k]
		}
	}
	return s, nil
}

// ChecksOK e' il numero di check digit validi (0-5).
func (s Summary) ChecksOK() int {
	n := 0
	for _, ok := range s.Checks {
		if ok {
			n++
		}
	}
	return n
}

// Present dice, per ciascuno dei 14 campi di FieldKeys, se e' stato letto (valore non vuoto).
func (s Summary) Present() [NFields]bool {
	var p [NFields]bool
	for i, k := range FieldKeys {
		p[i] = s.Fields[k] != ""
	}
	return p
}

// Found e' il numero di campi stampati letti (0-14).
func (s Summary) Found() int {
	n := 0
	for _, ok := range s.Present() {
		if ok {
			n++
		}
	}
	return n
}

// Missing elenca i nomi dei campi stampati non letti, nell'ordine di FieldKeys.
func (s Summary) Missing() []string {
	var out []string
	for i, ok := range s.Present() {
		if !ok {
			out = append(out, FieldKeys[i])
		}
	}
	return out
}
