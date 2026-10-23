package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"scanlibretto/internal/memoria"
	"scanlibretto/internal/mrz"
)

// fieldDef descrive un campo mostrato: chiave JSON di "libretto extract" ed etichetta stampata bilingue.
type fieldDef struct {
	Key     string
	ES, EN  string
	Kind    string // "testo", "data"
	FromMRZ bool   // il valore viene dall'MRZ (riga 1 o 2), non dalla zona stampata
}

// fieldDefs è l'ordine di visualizzazione. Le etichette sono quelle stampate sul libretto.
var fieldDefs = []fieldDef{
	{Key: "type", ES: "Tipo", EN: "Type"},
	{Key: "country_code", ES: "Código del país", EN: "Country code"},
	{Key: "given_names", ES: "Nombres", EN: "Given Names", FromMRZ: true},
	{Key: "surname", ES: "Apellidos", EN: "Surname", FromMRZ: true},
	{Key: "nationality", ES: "Nacionalidad", EN: "Nationality"},
	{Key: "sex", ES: "Sexo", EN: "Sex", FromMRZ: true},
	{Key: "place_of_birth", ES: "Lugar de nacimiento", EN: "Place of birth"},
	{Key: "date_of_issue", ES: "Fecha de emisión", EN: "Date of issue", Kind: "data"},
	{Key: "date_of_expiry", ES: "Fecha de expiración", EN: "Date of expiry", Kind: "data"},
	{Key: "date_of_birth", ES: "Fecha de nacimiento", EN: "Date of birth", Kind: "data"},
	{Key: "level", ES: "Nivel", EN: "Level"},
	{Key: "capacity", ES: "Cargo", EN: "Capacity"},
	{Key: "seamans_book", ES: "Libreta de embarque", EN: "Seaman's Book"},
	{Key: "id_number", ES: "Identificación", EN: "ID number"},
	{Key: "certificate", ES: "Nº Título", EN: "Certificate"},
	{Key: "seafarers_id", ES: "Id Marino", EN: "Seafarers Id"},
	{Key: "regional", ES: "Regional", EN: "Regional"},
}

func findDef(key string) (fieldDef, bool) {
	for _, d := range fieldDefs {
		if d.Key == key {
			return d, true
		}
	}
	return fieldDef{}, false
}

// warning è un avviso con codice stabile (docs/campi.md).
type warning struct {
	Code   string   `json:"code"`
	Field  string   `json:"field,omitempty"`
	Fields []string `json:"fields,omitempty"`
}

// extractOut è il JSON stampato da "libretto extract" (schema_version 1).
type extractOut struct {
	SchemaVersion  int               `json:"schema_version"`
	RotationDeg    int               `json:"rotation_deg"`
	MRZ            *mrz.Data         `json:"mrz"`
	MRZCorrections []string          `json:"mrz_corrections"`
	Fields         map[string]string `json:"fields"`
	Warnings       []warning         `json:"warnings"`
}

// parseExtract legge lo stdout di "libretto extract". Accetta testo prima del JSON (si parte dalla prima graffa).
func parseExtract(b []byte) (*extractOut, error) {
	if i := bytes.IndexByte(b, '{'); i > 0 {
		b = b[i:]
	}
	var o extractOut
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, fmt.Errorf("l'estrazione non ha prodotto un JSON valido: %w", err)
	}
	if o.Fields == nil {
		return nil, errors.New("l'estrazione non ha prodotto la sezione dei campi")
	}
	return &o, nil
}

// readValues ricava il valore letto per ogni campo mostrato. Il campo `certificate` si legge anche con il vecchio
// nome `endorsement`; cognome, nomi e sesso, se mancano dai campi, si prendono dall'MRZ.
func readValues(o *extractOut) map[string]string {
	v := make(map[string]string, len(fieldDefs))
	for _, d := range fieldDefs {
		v[d.Key] = strings.TrimSpace(o.Fields[d.Key])
	}
	if v["certificate"] == "" {
		v["certificate"] = strings.TrimSpace(o.Fields["endorsement"])
	}
	if o.MRZ != nil {
		if v["surname"] == "" {
			v["surname"] = strings.TrimSpace(o.MRZ.Surname)
		}
		if v["given_names"] == "" {
			v["given_names"] = strings.TrimSpace(o.MRZ.GivenNames)
		}
		if v["sex"] == "" {
			v["sex"] = strings.TrimSpace(o.MRZ.Sex)
		}
	}
	return v
}

// mrzValid dice se l'MRZ c'è e tutti e cinque i check digit tornano.
func mrzValid(m *mrz.Data) bool {
	if m == nil {
		return false
	}
	c := m.Checks
	return c.DocNumber && c.Birth && c.Expiry && c.Optional && c.Composite
}

// Doc è un'immagine letta, con lo stato che serve a salvare le correzioni. Tenuto in memoria dal server.
type Doc struct {
	mu       sync.Mutex
	ID       string
	File     string // nome del file (es. "NOME.png") in scans\ oppure, se Dir e' "preview", in dati\anteprima\
	PageFile string // anteprima letta: nome della sola pagina dati ritagliata in dati\\anteprima ("" se non trovata)
	Dir      string // "scans" (salvato) | "preview" (anteprima temporanea non salvata)
	Name     string // NOME senza estensione
	Raw      []byte // stdout di extract (per scans\NOME.json)
	Out      *extractOut
	Read     map[string]string // valori letti dal programma
	Initial  map[string]string // valori mostrati all'inizio (dopo la memoria) o all'ultimo salvataggio
	Applied  map[string]memoria.Correzione
	Key      string // chiave d'identita' del libretto ("" se l'MRZ non è valida)
	When     time.Time

	Issues    map[string][]string         // motivi per cui un campo è da controllare, ricavati dagli avvisi del lettore
	NotRead   map[string]bool             // campi che il lettore dichiara non letti
	Confirmed map[string]memoria.Conferma // valori già controllati in passato per questo libretto (memoria)
	Reviewed  map[string]string           // campi "da controllare" segnati controllati: "user" o "memory"
}

// reasons elenca i motivi per cui il campo con valore v è da controllare: avvisi del lettore, formato non plausibile,
// valore diverso da quello confermato l'ultima volta. Vuoto = niente da controllare.
func (d *Doc) reasons(def fieldDef, v string) []string {
	r := append([]string(nil), d.Issues[def.Key]...)
	if why := formatIssue(def, v); why != "" {
		r = append(r, why)
	}
	if c, ok := d.Confirmed[def.Key]; ok && c.Valore != v {
		prev := c.Valore
		if prev == "" {
			prev = "(vuoto)"
		}
		r = append(r, "l'ultima volta era stato confermato «"+prev+"»")
	}
	return r
}

// newDoc costruisce il documento e applica la memoria (solo con MRZ valida).
func newDoc(id, file string, raw []byte, out *extractOut, mem *memoria.Archivio) *Doc {
	d := &Doc{
		ID: id, File: file, Name: strings.TrimSuffix(file, fileExt(file)), Raw: raw, Out: out,
		Read: readValues(out), Applied: map[string]memoria.Correzione{}, When: time.Now(),
		Confirmed: map[string]memoria.Conferma{}, Reviewed: map[string]string{},
	}
	d.Issues, d.NotRead = issues(out)
	d.Initial = make(map[string]string, len(d.Read))
	for k, v := range d.Read {
		d.Initial[k] = v
	}
	if key, err := memoria.Chiave(out.MRZ); err == nil {
		d.Key = key
		for field, c := range mem.Campi(key) {
			if _, ok := findDef(field); !ok {
				continue
			}
			if d.Read[field] != c.Corretto {
				d.Applied[field] = c
				d.Initial[field] = c.Corretto
			}
		}
		for field, c := range mem.Confermati(key) {
			if _, ok := findDef(field); ok {
				d.Confirmed[field] = c
			}
		}
	}
	// un campo già controllato in passato con lo stesso valore è subito "Controllato (da memoria)"
	for _, f := range fieldDefs {
		c, ok := d.Confirmed[f.Key]
		if !ok || c.Valore != d.Read[f.Key] {
			continue
		}
		if _, applied := d.Applied[f.Key]; applied {
			continue
		}
		if len(d.reasons(f, d.Read[f.Key])) > 0 {
			d.Reviewed[f.Key] = "memory"
		}
	}
	return d
}

func fileExt(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i:]
	}
	return ""
}

// ---- vista per l'interfaccia ------------------------------------------------------------------------------------

// FieldView è una riga della tabella dei campi.
type FieldView struct {
	Key        string `json:"key"`
	ES         string `json:"es"`
	EN         string `json:"en"`
	Kind       string `json:"kind"`
	Value      string `json:"value"`
	Read       string `json:"read"`
	Status     string `json:"status"`             // ok | check | reviewed | empty | memory | edited
	Flag       bool   `json:"flag"`               // c'è un motivo per controllarlo (check, o reviewed se già controllato)
	Reviewed   string `json:"reviewed,omitempty"` // con status reviewed: "user" o "memory"
	StatusText string `json:"status_text"`
	Hint       string `json:"hint,omitempty"`   // perché è da controllare, o perché è vuoto
	Source     string `json:"source,omitempty"` // "letto dall'MRZ"
	Extra      string `json:"extra,omitempty"`
	MemRead    string `json:"mem_read,omitempty"` // con status memory: il valore che il programma aveva letto
	MemDate    string `json:"mem_date,omitempty"`
}

// CheckView è un check digit con il suo nome in italiano.
type CheckView struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	OK    bool   `json:"ok"`
}

// MRZView è la sezione MRZ.
type MRZView struct {
	Found       bool        `json:"found"`
	Valid       bool        `json:"valid"`
	Line1       string      `json:"line1"`
	Line2       string      `json:"line2"`
	Checks      []CheckView `json:"checks"`
	Corrections []string    `json:"corrections"`
}

// WarnView è un avviso tradotto in italiano semplice.
type WarnView struct {
	Code     string `json:"code"`
	Text     string `json:"text"`
	Severity string `json:"severity"` // info | warn | error
}

// MemView dice che cosa si può fare con la memoria per questo libretto.
type MemView struct {
	Usable    bool   `json:"usable"`    // MRZ valida: la memoria si applica e si salva
	Reason    string `json:"reason"`    // perché non è usabile
	Applied   int    `json:"applied"`   // campi corretti dalla memoria
	Known     int    `json:"known"`     // correzioni memorizzate per questo libretto
	Confirmed int    `json:"confirmed"` // campi controllati in passato
}

// DocView è tutto cio' che l'interfaccia mostra per un'immagine letta.
type DocView struct {
	ID        string      `json:"doc_id"`
	File      string      `json:"file"`
	Name      string      `json:"name"`
	Dir       string      `json:"dir"`
	PageURL   string      `json:"page_url,omitempty"` // anteprima letta: la pagina dati ritagliata che verra' salvata
	Unsaved   bool        `json:"unsaved"`            // anteprima temporanea: non c'e' ancora nulla in scans\
	ImageURL  string      `json:"image_url"`
	Rotation  int         `json:"rotation"`
	Fields    []FieldView `json:"fields"`
	MRZ       MRZView     `json:"mrz"`
	Warnings  []WarnView  `json:"warnings"`
	Memory    MemView     `json:"memory"`
	ReadAt    string      `json:"read_at"`
	SavedJSON bool        `json:"saved_json"`
	ToCheck   int         `json:"to_check"` // campi ancora "Da controllare"
}

var checkLabels = []struct{ key, label string }{
	{"doc_number", "Numero libretto"},
	{"birth", "Data di nascita"},
	{"expiry", "Data di scadenza"},
	{"optional", "Dati opzionali"},
	{"composite", "Controllo finale"},
}

var (
	reData = regexp.MustCompile(`^\d{2}-\d{2}-\d{4}$`)
	reBook = regexp.MustCompile(`^PA?\d{7}$`)
)

// formatIssue segnala un valore non plausibile per il formato del campo ("" se va bene o se è vuoto).
func formatIssue(def fieldDef, v string) string {
	if v == "" {
		return ""
	}
	switch {
	case def.Kind == "data":
		if !reData.MatchString(v) {
			return "formato data non valido (GG-MM-AAAA)"
		}
		if _, err := time.Parse("02-01-2006", v); err != nil {
			return "data inesistente"
		}
	case def.Key == "seamans_book":
		if !reBook.MatchString(v) {
			return "formato insolito (P o PA e 7 cifre)"
		}
	case def.Key == "sex":
		if v != "M" && v != "F" && v != "X" {
			return "valore insolito (M o F)"
		}
	case def.Key == "type":
		if v != "SI" {
			return "sul libretto è stampato SI"
		}
	}
	return ""
}

// issues raccoglie, per campo, i motivi per cui un valore è da controllare, ricavati dagli avvisi del lettore.
func issues(o *extractOut) (check map[string][]string, notRead map[string]bool) {
	check = map[string][]string{}
	notRead = map[string]bool{}
	add := func(k, why string) {
		for _, w := range check[k] {
			if w == why {
				return
			}
		}
		check[k] = append(check[k], why)
	}
	for _, w := range o.Warnings {
		switch w.Code {
		case "UNCERTAIN_FIELD":
			add(w.Field, "lettura incerta")
		case "PRINTED_FIELD_DIFFERS_FROM_MRZ":
			add(w.Field, "diverso da quello dell'MRZ")
		case "MRZ_FIELD_UNCERTAIN":
			f := w.Field
			if f == "nationality_iso" {
				f = "nationality"
			}
			add(f, "lettura MRZ incerta")
		case "MRZ_LINE1_UNCERTAIN":
			add("surname", "lettura MRZ incerta")
			add("given_names", "lettura MRZ incerta")
		case "DATE_OF_EXPIRY_NOT_AFTER_DATE_OF_ISSUE":
			add("date_of_expiry", "non è dopo l'emissione")
			add("date_of_issue", "emissione non prima della scadenza")
		case "DATE_OF_BIRTH_NOT_BEFORE_DATE_OF_ISSUE":
			add("date_of_birth", "non è prima dell'emissione")
			add("date_of_issue", "emissione non dopo la nascita")
		case "PRINTED_FIELDS_NOT_READ":
			for _, f := range w.Fields {
				notRead[f] = true
			}
		}
	}
	return
}

// fieldLabelIT restituisce l'etichetta stampata di un campo, per i messaggi.
func fieldLabelIT(key string) string {
	if d, ok := findDef(key); ok {
		return d.ES + " / " + d.EN
	}
	if key == "endorsement" {
		return "Nº Título / Certificate"
	}
	return key
}

func warnText(w warning) WarnView {
	v := WarnView{Code: w.Code, Severity: "warn"}
	label := func() string {
		f := w.Field
		if f == "nationality_iso" {
			f = "nationality"
		}
		return fieldLabelIT(f)
	}
	switch w.Code {
	case "MRZ_NOT_FOUND":
		v.Severity = "error"
		v.Text = "Le due righe MRZ (quelle con i simboli <<<) non sono state trovate. I dati stampati non si possono verificare e la memoria non può essere usata. Controlla come è inserito il libretto (bordo MRZ per primo, centrato, dritto, spinto fino in fondo) e rifai la scansione."
	case "MRZ_CHECK_DIGITS_INVALID":
		v.Severity = "error"
		v.Text = "Le righe MRZ sono state lette, ma uno o più codici di controllo non tornano: qualche carattere è probabilmente sbagliato. Controlla bene tutti i dati. La memoria non viene usata."
	case "PRINTED_FIELDS_NOT_READ":
		v.Severity = "info"
		labels := make([]string, 0, len(w.Fields))
		for _, f := range w.Fields {
			labels = append(labels, fieldLabelIT(f))
		}
		sort.Strings(labels)
		v.Text = "Campi stampati non letti: " + strings.Join(labels, ", ") + ". Se sul libretto ci sono, scrivili a mano; se non ci sono (normale per alcuni modelli) va bene così."
	case "MRZ_LINE1_UNCERTAIN":
		v.Text = "Cognome e nomi (riga 1 dell'MRZ) sono incerti: due letture diverse. Confrontali con il libretto."
	case "MRZ_FIELD_UNCERTAIN":
		v.Text = "Il campo " + label() + " dell'MRZ è incerto: due letture diverse. Confrontalo con il libretto."
	case "UNCERTAIN_FIELD":
		v.Text = "Lettura incerta per il campo " + label() + ": controllalo con l'immagine."
	case "PRINTED_FIELD_DIFFERS_FROM_MRZ":
		v.Text = "Il campo " + label() + " stampato è diverso da quello dell'MRZ: uno dei due è letto male. L'MRZ di solito è più affidabile."
	case "DATE_OF_EXPIRY_NOT_AFTER_DATE_OF_ISSUE":
		v.Text = "La data di scadenza non è dopo la data di emissione: una delle due è letta male."
	case "DATE_OF_BIRTH_NOT_BEFORE_DATE_OF_ISSUE":
		v.Text = "La data di nascita non è prima della data di emissione: una delle due è letta male."
	case "UNKNOWN_WARNING":
		v.Text = "Il lettore ha segnalato un'anomalia che il programma non sa descrivere: controlla i dati a mano."
	default:
		v.Text = "Avviso del lettore non riconosciuto (" + w.Code + "): controlla i dati a mano."
	}
	return v
}

// view costruisce la vista per l'interfaccia. Va chiamata con d.mu preso.
func (d *Doc) view(mem *memoria.Archivio, savedJSON bool) DocView {
	o := d.Out
	v := DocView{ID: d.ID, File: d.File, Name: d.Name, Dir: d.Dir, Unsaved: d.Dir == dirPreview, Rotation: o.RotationDeg, ReadAt: d.When.Format(time.RFC3339), SavedJSON: savedJSON}
	for _, f := range fieldDefs {
		fv := FieldView{Key: f.Key, ES: f.ES, EN: f.EN, Kind: f.Kind, Value: d.Initial[f.Key], Read: d.Read[f.Key]}
		if f.FromMRZ {
			fv.Source = "letto dall'MRZ"
		}
		if f.Key == "nationality" && o.Fields["nationality_iso"] != "" {
			fv.Extra = "ISO " + o.Fields["nationality_iso"]
		}
		applied, isApplied := d.Applied[f.Key]
		switch {
		case isApplied:
			fv.Status, fv.StatusText = "memory", "Corretto da memoria"
			fv.MemRead = d.Read[f.Key] // quello che il programma ha letto questa volta
			fv.MemDate = applied.Data.Format("2006-01-02")
		case fv.Value != d.Read[f.Key]:
			fv.Status, fv.StatusText = "edited", "Modificato da te"
		case len(d.reasons(f, fv.Value)) > 0:
			fv.Flag = true
			fv.Hint = strings.Join(d.reasons(f, fv.Value), "; ")
			if src := d.Reviewed[f.Key]; src != "" {
				fv.Status, fv.Reviewed = "reviewed", src
				fv.StatusText = "Controllato"
				if src == "memory" {
					fv.StatusText = "Controllato (da memoria)"
				}
			} else {
				fv.Status, fv.StatusText = "check", "Da controllare"
			}
		case fv.Value == "":
			fv.Status, fv.StatusText = "empty", "Non letto"
			if d.NotRead[f.Key] {
				fv.Hint = "non letto: se sul libretto c'è scrivilo, se non c'è va bene così"
			} else {
				fv.Hint = "campo assente o vuoto"
			}
		default:
			fv.Status, fv.StatusText = "ok", "OK"
		}
		v.Fields = append(v.Fields, fv)
		if fv.Status == "check" {
			v.ToCheck++
		}
	}
	// MRZ
	v.MRZ = MRZView{Found: o.MRZ != nil, Valid: mrzValid(o.MRZ), Corrections: append([]string{}, o.MRZCorrections...)}
	if o.MRZ != nil {
		v.MRZ.Line1, v.MRZ.Line2 = o.MRZ.Line1, o.MRZ.Line2
		ok := map[string]bool{"doc_number": o.MRZ.Checks.DocNumber, "birth": o.MRZ.Checks.Birth, "expiry": o.MRZ.Checks.Expiry,
			"optional": o.MRZ.Checks.Optional, "composite": o.MRZ.Checks.Composite}
		for _, c := range checkLabels {
			v.MRZ.Checks = append(v.MRZ.Checks, CheckView{Key: c.key, Label: c.label, OK: ok[c.key]})
		}
	}
	for _, w := range o.Warnings {
		v.Warnings = append(v.Warnings, warnText(w))
	}
	// memoria
	v.Memory.Applied = len(d.Applied)
	if d.Key != "" {
		v.Memory.Usable = true
		v.Memory.Known = len(mem.Campi(d.Key))
		v.Memory.Confirmed = len(d.Confirmed)
	} else if o.MRZ == nil {
		v.Memory.Reason = "MRZ non trovata: senza MRZ valida non si può riconoscere il libretto, quindi la memoria non viene né applicata né salvata."
	} else {
		v.Memory.Reason = "MRZ con codici di controllo non validi: il libretto non si può riconoscere con sicurezza, quindi la memoria non viene né applicata né salvata."
	}
	return v
}
