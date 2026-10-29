package verify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"scanlibretto/internal/outsum"
)

// Result e' l'esito della verifica di un JSON di libretto: il confronto con la verita' (se c'e') e le incoerenze
// trovate senza verita'. Non contiene valori letti ne' verita' (solo il riassunto Sum, che l'utente non deve stampare).
type Result struct {
	Name     string         // nome dell'immagine o del file, senza estensione
	Sum      outsum.Summary // l'uscita di libretto (contiene i valori letti: non stamparla)
	HasTruth bool
	Fields   []FieldResult // 14 esiti, vuoto senza verita'
	Findings []Finding
	Absent   []string   // chiavi mancanti dal file di verita' (probabili refusi)
	Names    NameStatus // cognome e nomi della riga 1 dell'MRZ contro la riga del nome stampata (NameNA senza `_riga_nome`)
}

// Verify confronta un'uscita di libretto con la verita' (nil se non c'e') e applica le regole di coerenza.
func Verify(name string, sum outsum.Summary, truth *Truth) Result {
	r := Result{Name: name, Sum: sum, Findings: Check(sum)}
	if truth != nil {
		r.HasTruth = true
		r.Fields = Compare(sum, *truth)
		r.Absent = truth.Absent
		r.Names = nameStatus(sum.Fields, *truth)
	}
	return r
}

// Counts conta gli esiti dei campi (tutti zero senza verita').
func (r Result) Counts() Counts { return Count(r.Fields) }

// Severities conta le incoerenze per gravita'.
func (r Result) Severities() (errs, warns, infos int) {
	for _, f := range r.Findings {
		switch f.Severity {
		case SevError:
			errs++
		case SevWarn:
			warns++
		default:
			infos++
		}
	}
	return
}

// Failed dice se la verifica e' da considerare fallita: un campo SBAGLIATO o MANCANTE, oppure un'incoerenza di
// gravita' ERRORE (anche AVVISO con strict). IN_PIU e INFO non fanno fallire.
func (r Result) Failed(strict bool) bool {
	c := r.Counts()
	if c.Wrong > 0 || c.Missing > 0 || r.Names == NameWrong {
		return true
	}
	errs, warns, _ := r.Severities()
	return errs > 0 || (strict && warns > 0)
}

// ParseOutput legge l'uscita JSON di `libretto extract`. Rifiuta i file che non sono uscite dello schema 1
// (per esempio i JSON salvati dalle prime versioni, con le chiavi in italiano).
func ParseOutput(b []byte) (outsum.Summary, error) {
	var head struct {
		SchemaVersion int               `json:"schema_version"`
		Fields        map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return outsum.Summary{}, fmt.Errorf("non e' un JSON: %w", err)
	}
	if head.SchemaVersion < 1 || head.Fields == nil {
		return outsum.Summary{}, errors.New("non e' un'uscita di libretto con schema_version 1 (manca schema_version o fields)")
	}
	sum, err := outsum.Parse(b)
	if err != nil {
		return outsum.Summary{}, err
	}
	normalizeKeys(sum.Fields) // un'uscita con il vecchio nome `endorsement` si legge come `certificate` (e viceversa)
	return sum, nil
}

// VerifyFile verifica un file JSON. truthPath vuoto = solo regole di coerenza.
func VerifyFile(jsonPath, truthPath string) (Result, error) {
	b, err := os.ReadFile(jsonPath)
	if err != nil {
		return Result{}, err
	}
	sum, err := ParseOutput(b)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w", filepath.Base(jsonPath), err)
	}
	var truth *Truth
	if truthPath != "" {
		t, err := LoadTruth(truthPath)
		if err != nil {
			return Result{}, err
		}
		truth = &t
	}
	return Verify(stem(jsonPath), sum, truth), nil
}

func stem(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// DirReport e' l'esito della verifica di tutti i JSON di una cartella.
type DirReport struct {
	Items   []Result // ordinati per nome
	Skipped []string // file .json che non sono uscite di libretto (nomi)
	NoTruth []string // JSON senza file di verita': verificati solo con le regole di coerenza
}

// VerifyDir verifica tutti i file *.json di dir (senza scendere nelle sottocartelle). Per ogni NOME.json cerca la
// verita' in truthDir/NOME.json; se truthDir e' vuoto non confronta nulla. I file che non sono uscite di libretto
// (verita' comprese, se le due cartelle coincidono) si saltano e si elencano in Skipped.
func VerifyDir(dir, truthDir string) (DirReport, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return DirReport{}, err
	}
	var rep DirReport
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		truthPath := ""
		if truthDir != "" {
			p := filepath.Join(truthDir, e.Name())
			if _, err := os.Stat(p); err == nil {
				truthPath = p
			}
		}
		res, err := VerifyFile(path, truthPath)
		if err != nil {
			if errors.Is(err, ErrBadTruth) {
				return DirReport{}, err // una verita' rotta e' un errore vero, non un file da saltare
			}
			rep.Skipped = append(rep.Skipped, e.Name())
			continue
		}
		if truthDir != "" && truthPath == "" {
			rep.NoTruth = append(rep.NoTruth, res.Name)
		}
		rep.Items = append(rep.Items, res)
	}
	sort.Slice(rep.Items, func(i, j int) bool { return rep.Items[i].Name < rep.Items[j].Name })
	return rep, nil
}

// Failed dice se almeno una immagine ha fallito.
func (d DirReport) Failed(strict bool) bool {
	for _, r := range d.Items {
		if r.Failed(strict) {
			return true
		}
	}
	return false
}

// Filter restituisce un report con le sole immagini per cui keep e' vero.
func (d DirReport) Filter(keep func(Result) bool) DirReport {
	out := DirReport{Skipped: d.Skipped, NoTruth: d.NoTruth}
	for _, r := range d.Items {
		if keep(r) {
			out.Items = append(out.Items, r)
		}
	}
	return out
}

// MRZValid dice se nell'uscita c'e' un MRZ con tutti e cinque i check digit validi.
func (r Result) MRZValid() bool { return r.Sum.HasMRZ && r.Sum.ChecksOK() == outsum.NChecks }

// Readable dice se la verita' ha almeno un campo stampato e leggibile (l'immagine mostra una pagina dati).
func (r Result) Readable() bool { return r.HasTruth && r.Counts().WithTruth() > 0 }

// FieldStats e' l'accuratezza di un campo su un insieme di immagini con verita'.
type FieldStats struct {
	Field string
	Counts
	Kinds    map[ErrKind]int // famiglie di errore dei campi sbagliati
	MRZFixes int             // sbagliati o mancanti che l'MRZ avrebbe corretto
	MRZNoFix int             // sbagliati o mancanti con MRZ che non li avrebbe corretti
	// MRZAbsent: sbagliati o mancanti su un campo che sta nell'MRZ, ma l'MRZ non c'era (o il suo check digit non tornava)
	MRZAbsent int
	Found     int // volte che il programma ha scritto un valore (la vecchia metrica "campi trovati")
}

// Stats e' la somma degli esiti di un insieme di immagini.
type Stats struct {
	Images int          // immagini con verita' leggibile (almeno un campo stampato)
	NoData int          // immagini con verita' vuota: la pagina dati non c'e' (copertine, pagine interne, strisce MRZ)
	Fields []FieldStats // 14, nell'ordine di outsum.FieldKeys
	Total  Counts
	// NamesOK, NamesWrong, NamesMissing contano le immagini con la riga del nome in verita': cognome e nomi dell'MRZ giusti,
	// sbagliati, o assenti perche' l'MRZ non c'e'.
	NamesOK, NamesWrong, NamesMissing int
	Findings                          map[string]int // per codice: in quante immagini compare
	Severity                          map[string]Severity
}

// Stats somma gli esiti di tutte le immagini del report.
func (d DirReport) Stats() Stats { return StatsOf(d.Items) }

// StatsOf somma gli esiti di un insieme di risultati: accuratezza per campo, famiglie di errore, correzioni
// possibili con l'MRZ e frequenza delle incoerenze. Le immagini senza verita' contano solo per le incoerenze.
func StatsOf(items []Result) Stats {
	a := Stats{Findings: map[string]int{}, Severity: map[string]Severity{}}
	for _, f := range outsum.FieldKeys {
		a.Fields = append(a.Fields, FieldStats{Field: f, Kinds: map[ErrKind]int{}})
	}
	for _, r := range items {
		seen := map[string]bool{}
		for _, f := range r.Findings {
			if !seen[f.Code] {
				seen[f.Code] = true
				a.Findings[f.Code]++
				a.Severity[f.Code] = f.Severity
			}
		}
		if !r.HasTruth {
			continue
		}
		switch r.Names {
		case NameOK:
			a.NamesOK++
		case NameWrong:
			a.NamesWrong++
		case NameMissing:
			a.NamesMissing++
		}
		if r.Readable() {
			a.Images++
		} else {
			a.NoData++
		}
		for i, fr := range r.Fields {
			s := &a.Fields[i]
			if lookup(r.Sum.Fields, fr.Field) != "" {
				s.Found++
			}
			switch fr.Status {
			case StatusOK:
				s.OK++
				a.Total.OK++
			case StatusWrong:
				s.Wrong++
				a.Total.Wrong++
				s.Kinds[fr.Class.Kind]++
			case StatusMissing:
				s.Missing++
				a.Total.Missing++
			case StatusNotPrinted:
				s.NotPrinted++
				a.Total.NotPrinted++
			case StatusExtra:
				s.Extra++
				a.Total.Extra++
			}
			if fr.Status == StatusWrong || fr.Status == StatusMissing {
				switch fr.MRZ {
				case MRZFixes:
					s.MRZFixes++
				case MRZNoFix:
					s.MRZNoFix++
				case MRZAbsent:
					s.MRZAbsent++
				}
			}
		}
	}
	return a
}
