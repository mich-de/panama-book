package main

// Riga della tabella di regressione e sua lettura/scrittura in TSV. Nella tabella ci sono solo numeri e simboli:
// mai i valori letti dal documento.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"scanlibretto/internal/outsum"
)

// columns sono le colonne del TSV, nell'ordine di scrittura. Chi legge una baseline cerca le colonne per nome.
var columns = []string{"file", "rot", "mrz", "check", "campi", "avvisi", "sec", "giusti", "nomi", "mappa_check", "mappa_campi", "mappa_giusti"}

// Simboli delle mappe. mappa_check e mappa_campi: '1' valido/letto, '0' no, '-' non disponibile.
// mappa_giusti (un simbolo per campo, stesso ordine di outsum.FieldKeys): vedi truth.go.
const (
	symOK       = '1'
	symKO       = '0'
	symNA       = '-'
	symRight    = 'G' // letto e uguale alla verita'
	symWrong    = 'S' // letto ma diverso dalla verita'
	symMissing  = 'M' // non letto, ma la verita' ha un valore
	symSpurious = 'X' // letto, ma la verita' e' vuota
)

// Row e' l'esito di una immagine del corpus.
type Row struct {
	File     string // percorso relativo alla radice, con '/'
	Err      bool   // nessun JSON valido: errore, timeout o uscita diversa da zero
	Note     string // motivo dell'errore (solo per lo stderr, non va nel TSV)
	Rotation int
	MRZ      bool
	CheckMap string // outsum.NChecks simboli, ordine di outsum.CheckKeys
	FieldMap string // outsum.NFields simboli, ordine di outsum.FieldKeys
	Warnings int
	Seconds  float64
	Truth    bool   // esiste la verita' per questa immagine
	TruthMap string // outsum.NFields simboli (G S M X -), solo se Truth
	Name     byte   // riga 1 dell'MRZ contro la riga del nome in verita': G giusta, S sbagliata, M senza MRZ, 0 o '-' niente da confrontare
}

func dashes(n int) string { return strings.Repeat(string(rune(symNA)), n) }

// nameSym restituisce il simbolo dei nomi di una riga ('-' se non c'e' niente da confrontare).
func (r Row) nameSym() byte {
	if r.Name == 0 {
		return symNA
	}
	return r.Name
}

// rowFromSummary costruisce la riga di una prova riuscita. truthMap e' vuota se non c'e' verita'; name e' il simbolo
// dei nomi (0 se la verita' non ha la riga del nome).
func rowFromSummary(file string, s outsum.Summary, seconds float64, truthMap string, name byte) Row {
	r := Row{File: file, Rotation: s.Rotation, MRZ: s.HasMRZ, Warnings: s.Warnings, Seconds: seconds,
		CheckMap: dashes(outsum.NChecks), Truth: truthMap != "", TruthMap: truthMap, Name: name}
	if s.HasMRZ {
		r.CheckMap = boolMap(s.Checks[:])
	}
	present := s.Present()
	r.FieldMap = boolMap(present[:])
	return r
}

func boolMap(v []bool) string {
	b := make([]byte, len(v))
	for i, ok := range v {
		b[i] = symKO
		if ok {
			b[i] = symOK
		}
	}
	return string(b)
}

func (r Row) checksOK() int { return strings.Count(r.CheckMap, string(rune(symOK))) }
func (r Row) found() int    { return strings.Count(r.FieldMap, string(rune(symOK))) }

// right restituisce quanti campi sono giusti (n) su quanti hanno un valore in verita' (m).
func (r Row) right() (n, m int) {
	n = strings.Count(r.TruthMap, string(rune(symRight)))
	m = n + strings.Count(r.TruthMap, string(rune(symWrong))) + strings.Count(r.TruthMap, string(rune(symMissing)))
	return n, m
}

// cells sono i valori delle colonne, nell'ordine di columns.
func (r Row) cells() []string {
	sec := fmt.Sprintf("%.1f", r.Seconds)
	if r.Err {
		return []string{r.File, "-", "err", "-", "-", "-", sec, "-", "-", dashes(outsum.NChecks), dashes(outsum.NFields), dashes(outsum.NFields)}
	}
	mrz := "no"
	if r.MRZ {
		mrz = "si"
	}
	giusti, gmap := "-", dashes(outsum.NFields)
	if r.Truth {
		n, m := r.right()
		giusti, gmap = fmt.Sprintf("%d/%d", n, m), r.TruthMap
	}
	return []string{r.File, strconv.Itoa(r.Rotation), mrz,
		fmt.Sprintf("%d/%d", r.checksOK(), outsum.NChecks), fmt.Sprintf("%d/%d", r.found(), outsum.NFields),
		strconv.Itoa(r.Warnings), sec, giusti, string(rune(r.nameSym())), r.CheckMap, r.FieldMap, gmap}
}

// writeTable scrive le righe di commento (meta), l'intestazione e una riga per immagine.
func writeTable(w io.Writer, meta []string, rows []Row) error {
	bw := bufio.NewWriter(w)
	for _, m := range meta {
		fmt.Fprintf(bw, "# %s\n", m)
	}
	fmt.Fprintln(bw, strings.Join(columns, "\t"))
	for _, r := range rows {
		fmt.Fprintln(bw, strings.Join(r.cells(), "\t"))
	}
	return bw.Flush()
}

// truthPrintPrefix apre la riga di intestazione con l'impronta della verita' usata per i "giusti" della tabella.
const truthPrintPrefix = "# impronta-verita: "

// tablePrint restituisce l'impronta della verita' scritta nell'intestazione della tabella in path ("" se la tabella
// non e' stata prodotta con -truth o non si legge).
func tablePrint(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "#") {
			break // finite le righe di commento
		}
		if rest, ok := strings.CutPrefix(line, truthPrintPrefix); ok {
			p, _, _ := strings.Cut(rest, " ")
			return strings.TrimSpace(p)
		}
	}
	return ""
}

// readTable legge una tabella scritta da writeTable. Salta righe vuote e commenti ('#'); le colonne si cercano
// per nome, cosi' una baseline con colonne in piu' resta leggibile.
func readTable(r io.Reader) ([]Row, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var idx map[string]int
	var rows []Row
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cells := strings.Split(line, "\t")
		if idx == nil {
			idx = make(map[string]int, len(cells))
			for i, c := range cells {
				idx[c] = i
			}
			for _, need := range []string{"file", "mrz", "mappa_check", "mappa_campi"} {
				if _, ok := idx[need]; !ok {
					return nil, fmt.Errorf("riga %d: manca la colonna %q (tabella non prodotta da regress?)", n, need)
				}
			}
			continue
		}
		get := func(name string) string {
			if i, ok := idx[name]; ok && i < len(cells) {
				return cells[i]
			}
			return ""
		}
		row := Row{File: get("file"), Err: get("mrz") == "err", MRZ: get("mrz") == "si",
			CheckMap: get("mappa_check"), FieldMap: get("mappa_campi")}
		if n := get("nomi"); n != "" && n != "-" { // colonna assente nelle tabelle vecchie: niente da confrontare
			row.Name = n[0]
		}
		row.Rotation, _ = strconv.Atoi(get("rot"))
		row.Warnings, _ = strconv.Atoi(get("avvisi"))
		row.Seconds, _ = strconv.ParseFloat(get("sec"), 64)
		if len(row.CheckMap) != outsum.NChecks || len(row.FieldMap) != outsum.NFields {
			return nil, fmt.Errorf("riga %d: mappe di lunghezza sbagliata (attese %d e %d)", n, outsum.NChecks, outsum.NFields)
		}
		if g := get("giusti"); g != "" && g != "-" {
			row.Truth, row.TruthMap = true, get("mappa_giusti")
			if len(row.TruthMap) != outsum.NFields {
				return nil, fmt.Errorf("riga %d: mappa_giusti di lunghezza sbagliata (attesa %d)", n, outsum.NFields)
			}
		}
		rows = append(rows, row)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if idx == nil {
		return nil, fmt.Errorf("tabella vuota: manca l'intestazione")
	}
	return rows, nil
}
