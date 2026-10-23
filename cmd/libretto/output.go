package main

import (
	"regexp"
	"sort"
	"strings"

	"scanlibretto/internal/libreta"
	"scanlibretto/internal/mrz"
)

// schemaVersion e' la versione dello schema JSON in uscita.
const schemaVersion = 1

// output e' il JSON stampato da extract e read. I nomi dei campi sono quelli stampati sul documento
// (etichette inglesi in snake_case), cosi' chi legge il JSON ritrova le stesse parole della pagina dati.
type output struct {
	SchemaVersion  int               `json:"schema_version"`
	RotationDeg    int               `json:"rotation_deg"`
	MRZ            *mrz.Data         `json:"mrz,omitempty"`
	MRZCorrections []string          `json:"mrz_corrections,omitempty"`
	Fields         map[string]string `json:"fields"`
	Warnings       []warning         `json:"warnings,omitempty"`
}

// warning e' un avviso con codice stabile, senza testo libero e senza valori personali.
type warning struct {
	Code   string   `json:"code"`
	Field  string   `json:"field,omitempty"`
	Fields []string `json:"fields,omitempty"`
}

// fieldNames traduce le chiavi interne nelle etichette del documento ("Nationality" -> nationality, ecc.).
var fieldNames = map[string]string{
	// zona visiva (etichette stampate)
	"tipo":            "type",
	"paese":           "country_code",
	"nacionalidad":    "nationality",
	"luogo_nascita":   "place_of_birth",
	"data_emissione":  "date_of_issue",
	"data_scadenza":   "date_of_expiry",
	"data_nascita":    "date_of_birth",
	"livello":         "level",
	"cargo":           "capacity",
	"numero_libretto": "seamans_book",
	"identificazione": "id_number",
	"refrendo":        "certificate", // etichetta stampata "Nº Título / Certificate" (9 cifre, solo modello 2026)
	"id_marino":       "seafarers_id",
	"regionale":       "regional",
	// derivati dall'MRZ
	"cognome":             "surname",
	"nomi":                "given_names",
	"nazionalita_iso":     "nationality_iso",
	"sesso":               "sex",
	"data_nascita_mrz":    "date_of_birth_mrz",
	"data_scadenza_mrz":   "date_of_expiry_mrz",
	"numero_libretto_mrz": "seamans_book_mrz",
	"regionale_mrz":       "regional_mrz",
}

// fieldName restituisce il nome di campo in inglese; mantiene il suffisso _iso delle date normalizzate.
func fieldName(internal string) string {
	if n, ok := fieldNames[internal]; ok {
		return n
	}
	if base, ok := strings.CutSuffix(internal, "_iso"); ok {
		if n, ok := fieldNames[base]; ok {
			return n + "_iso"
		}
	}
	return internal
}

var (
	reNotRead      = regexp.MustCompile(`^campi stampati non letti: (.+)$`)
	reUncertain    = regexp.MustCompile(`^campo incerto: ([a-z_]+)`)
	reMRZUncertain = regexp.MustCompile(`^campo MRZ incerto: ([a-z_]+)$`)
	reDiffers      = regexp.MustCompile(`^(.+) stampat[oa] divers[ao] dall'MRZ$`)
)

// differsField abbina il testo "<campo> stampato diverso dall'MRZ" al nome del campo.
var differsField = map[string]string{
	"nazionalita'":     "nationality",
	"numero libretto":  "seamans_book",
	"regionale":        "regional",
	"sesso":            "sex",
	"data di nascita":  "date_of_birth",
	"data di scadenza": "date_of_expiry",
}

// toWarning converte un avviso interno (testo) in un avviso con codice.
func toWarning(text string) warning {
	switch {
	case strings.HasPrefix(text, "MRZ non trovato"):
		return warning{Code: "MRZ_NOT_FOUND"}
	case strings.HasPrefix(text, "check digit MRZ non tutti validi"):
		return warning{Code: "MRZ_CHECK_DIGITS_INVALID"}
	case strings.HasPrefix(text, "data di nascita stampata non precedente"):
		return warning{Code: "DATE_OF_BIRTH_NOT_BEFORE_DATE_OF_ISSUE"}
	case strings.HasPrefix(text, "data di scadenza stampata non successiva"):
		return warning{Code: "DATE_OF_EXPIRY_NOT_AFTER_DATE_OF_ISSUE"}
	case strings.HasPrefix(text, "riga 1 MRZ incerta"):
		return warning{Code: "MRZ_LINE1_UNCERTAIN"}
	case strings.HasPrefix(text, "nome stampato diverso dall'MRZ"):
		return warning{Code: "MRZ_NAME_DIFFERS_FROM_PRINTED"}
	}
	if m := reMRZUncertain.FindStringSubmatch(text); m != nil {
		return warning{Code: "MRZ_FIELD_UNCERTAIN", Field: fieldName(m[1])}
	}
	if m := reNotRead.FindStringSubmatch(text); m != nil {
		var names []string
		for _, n := range strings.Split(m[1], ",") {
			names = append(names, fieldName(strings.TrimSpace(n)))
		}
		sort.Strings(names)
		return warning{Code: "PRINTED_FIELDS_NOT_READ", Fields: names}
	}
	if m := reUncertain.FindStringSubmatch(text); m != nil {
		return warning{Code: "UNCERTAIN_FIELD", Field: fieldName(m[1])}
	}
	if m := reDiffers.FindStringSubmatch(text); m != nil {
		if f, ok := differsField[m[1]]; ok {
			return warning{Code: "PRINTED_FIELD_DIFFERS_FROM_MRZ", Field: f}
		}
	}
	return warning{Code: "UNKNOWN_WARNING"}
}

// correctionEN traduce le correzioni MRZ: quelle dell'OCR ("riga2 pos 14: S→5") e le frasi del lettore a template e
// della seconda lettura (extract.go, mrzmerge.go). Le frasi lunghe vengono prima di "riga": a parita' di posizione
// vince la prima voce che coincide.
var correctionEN = strings.NewReplacer(
	"cognome e nomi presi dal lettore a template", "surname and given names taken from the template reader",
	"nazionalita' presa dal lettore a template", "nationality taken from the template reader",
	"sesso preso dal lettore a template", "sex taken from the template reader",
	"OCR e lettore differivano, nessuna cella diversa era incerta", "OCR and reader differed, none of the differing cells was uncertain",
	"vincoli MRZ del lettore a template", "MRZ constraints of the template reader",
	"riconosciuta solo da un glifo sintetico (mai vista su una scansione): da verificare", "recognised only from a synthetic glyph (never seen on a scan): to be checked",
	"lettura incerta", "uncertain reading",
	"riga", "line",
	"→", "->",
)

// correction rende in inglese le correzioni MRZ ("riga2 pos 14: S→5" -> "line2 pos 14: S->5").
func correction(s string) string { return correctionEN.Replace(s) }

// toOutput costruisce il JSON in uscita da un risultato di estrazione.
func toOutput(res *libreta.Result) output {
	out := output{
		SchemaVersion: schemaVersion,
		RotationDeg:   res.Rotation,
		MRZ:           res.MRZ,
		Fields:        make(map[string]string, len(res.Fields)),
	}
	for k, v := range res.Fields {
		out.Fields[fieldName(k)] = v
	}
	if panamaNameOrder(res.MRZ) {
		// il parser MRZ segue lo standard ICAO (primo gruppo = cognome): per i libretti di Panama va invertito
		m := *res.MRZ
		m.Surname, m.GivenNames = res.MRZ.GivenNames, res.MRZ.Surname
		out.MRZ = &m
		out.Fields["surname"], out.Fields["given_names"] = out.Fields["given_names"], out.Fields["surname"]
	}
	for _, c := range res.MRZCorrections {
		out.MRZCorrections = append(out.MRZCorrections, correction(c))
	}
	for _, w := range res.Warnings {
		out.Warnings = append(out.Warnings, toWarning(w))
	}
	return out
}

// panamaNameOrder dice se i nomi dell'MRZ vanno letti all'incontrario rispetto allo standard ICAO. Nella riga 1 dei
// libretti di Panama (codice documento IS, stato PAN; modelli 2020 e 2026 verificati) il primo gruppo, prima di `<<`,
// contiene i NOMI e il secondo i COGNOMI: "ISPAN" + NOMI<<COGNOMI. E' lo stesso ordine della riga stampata
// "Nombres/Given Names Apellidos/Surname". Se c'e' un solo gruppo non si inverte (resta il cognome).
func panamaNameOrder(m *mrz.Data) bool {
	return m != nil && m.DocCode == "IS" && m.Issuer == "PAN" && strings.TrimSpace(m.GivenNames) != "" && strings.TrimSpace(m.Surname) != ""
}
