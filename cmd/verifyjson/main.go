// Comando verifyjson: verifica i JSON prodotti da `libretto extract` contro la verita' letta a occhio e con regole di
// coerenza, senza mai stampare un valore letto o una verita' (solo nomi di campo, esiti e conteggi).
//
//	verifyjson -json FILE.json [-truth scans\verita\NOME.json]
//	verifyjson -dir CARTELLA_JSON [-truth scans\verita] [-v]
//
// Uscita: 0 tutto in ordine, 1 almeno un campo SBAGLIATO o MANCANTE o un'incoerenza ERRORE, 2 uso errato o file illeggibile.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"scanlibretto/internal/verify"
)

// Codici di uscita.
const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

// defaultTruthDir e' la cartella della verita' per convenzione del progetto (ignorata da git: dati personali).
var defaultTruthDir = filepath.Join("scans", "verita")

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

const usageText = `uso:
  verifyjson -json FILE.json [-truth VERITA.json | -truth CARTELLA_VERITA] [-strict]
  verifyjson -dir CARTELLA_JSON [-truth CARTELLA_VERITA] [-only REGEX] [-exclude REGEX] [-strict] [-v]

  -json    JSON prodotto da "libretto extract" ("-" legge dallo standard input)
  -truth   per -json: il file di verita' (o la cartella, in cui cerca NOME.json); per -dir: la cartella della
           verita' (predefinita: scans\verita se esiste). La verita' ha i 14 nomi dei campi in inglese
           (type, country_code, nationality, place_of_birth, date_of_issue, date_of_expiry, date_of_birth, level,
           capacity, seamans_book, id_number, certificate, seafarers_id, regional); stringa vuota = non stampato
           o illeggibile (il certificato c'e' solo su alcuni titolari: assente = NON_STAMPATO, non e' un errore).
           Il vecchio nome "endorsement" si legge ancora come alias di "certificate", nella verita' e nell'uscita.
           Senza verita' si applicano solo le regole di coerenza.
  -dir     verifica tutti i *.json della cartella, ognuno con la verita' dallo stesso nome
  -only    con -dir: solo le immagini il cui nome (senza estensione) corrisponde all'espressione regolare
  -exclude con -dir: salta le immagini il cui nome corrisponde (es. "(-grezza|-pagina[0-9]*)$" per tenere le scansioni distinte)
  -strict  gli avvisi (AVVISO) fanno fallire come gli errori
  -v       con -dir: dettaglio dei campi per ogni immagine

Per campo stampa OK, SBAGLIATO, MANCANTE, NON_STAMPATO (la verita' e' vuota) o IN_PIU (letto dove la verita' e'
vuota), senza mai mostrare i valori. Uscita: 0 in ordine; 1 almeno un SBAGLIATO, un MANCANTE o un'incoerenza
ERRORE; 2 uso errato o file illeggibile.
`

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verifyjson", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usageText) }
	jsonPath := fs.String("json", "", "JSON di libretto extract")
	truth := fs.String("truth", "", "verita' (file o cartella)")
	dir := fs.String("dir", "", "cartella di JSON")
	strict := fs.Bool("strict", false, "gli avvisi fanno fallire")
	verbose := fs.Bool("v", false, "dettaglio per immagine")
	only := fs.String("only", "", "solo i nomi che corrispondono (con -dir)")
	exclude := fs.String("exclude", "", "salta i nomi che corrispondono (con -dir)")
	// i flag possono stare anche dopo il nome del file (`verifyjson x.json -truth t.json`): il pacchetto flag si
	// ferma al primo argomento senza trattino, quindi si riprende dopo ognuno
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) { // -h: l'aiuto e' stato stampato, non e' un errore
				return exitOK
			}
			return exitUsage
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if *jsonPath == "" && len(positional) == 1 {
		*jsonPath = positional[0]
	} else if len(positional) > 0 {
		fmt.Fprintln(stderr, "verifyjson: argomenti in piu'")
		fs.Usage()
		return exitUsage
	}
	switch {
	case *jsonPath != "" && *dir != "":
		fmt.Fprintln(stderr, "verifyjson: -json e -dir si escludono")
		return exitUsage
	case *jsonPath != "":
		return runFile(*jsonPath, *truth, *strict, stdin, stdout, stderr)
	case *dir != "":
		return runDir(*dir, *truth, *only, *exclude, *strict, *verbose, stdout, stderr)
	}
	fs.Usage()
	return exitUsage
}

// runFile verifica un solo JSON.
func runFile(jsonPath, truth string, strict bool, stdin io.Reader, stdout, stderr io.Writer) int {
	var res verify.Result
	if jsonPath == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintln(stderr, "verifyjson:", err)
			return exitUsage
		}
		sum, err := verify.ParseOutput(b)
		if err != nil {
			fmt.Fprintln(stderr, "verifyjson: standard input:", err)
			return exitUsage
		}
		var t *verify.Truth
		if truth != "" {
			tt, err := verify.LoadTruth(truth)
			if err != nil {
				fmt.Fprintln(stderr, "verifyjson:", err)
				return exitUsage
			}
			t = &tt
		}
		res = verify.Verify("standard input", sum, t)
	} else {
		truthPath := truth
		if truth != "" {
			if st, err := os.Stat(truth); err == nil && st.IsDir() { // cartella: cerca NOME.json
				truthPath = filepath.Join(truth, filepath.Base(jsonPath))
			}
		}
		var err error
		res, err = verify.VerifyFile(jsonPath, truthPath)
		if err != nil {
			fmt.Fprintln(stderr, "verifyjson:", err)
			return exitUsage
		}
	}
	verify.WriteResult(stdout, res)
	if res.Failed(strict) {
		return exitFail
	}
	return exitOK
}

// runDir verifica tutti i JSON di una cartella.
func runDir(dir, truth, only, exclude string, strict, verbose bool, stdout, stderr io.Writer) int {
	keep, err := nameFilter(only, exclude)
	if err != nil {
		fmt.Fprintln(stderr, "verifyjson:", err)
		return exitUsage
	}
	if truth == "" {
		if st, err := os.Stat(defaultTruthDir); err == nil && st.IsDir() {
			truth = defaultTruthDir
		}
	}
	if truth != "" {
		if st, err := os.Stat(truth); err != nil || !st.IsDir() {
			fmt.Fprintln(stderr, "verifyjson: con -dir la verita' (-truth) dev'essere una cartella")
			return exitUsage
		}
	}
	rep, err := verify.VerifyDir(dir, truth)
	if err != nil {
		fmt.Fprintln(stderr, "verifyjson:", err)
		return exitUsage
	}
	rep = rep.Filter(func(r verify.Result) bool { return keep(r.Name) })
	if len(rep.Items) == 0 {
		fmt.Fprintln(stderr, "verifyjson: nessuna uscita di libretto nella cartella (o tutte escluse da -only/-exclude)")
		return exitUsage
	}
	verify.WriteDir(stdout, rep, verbose)
	if rep.Failed(strict) {
		fmt.Fprintln(stdout, "\nESITO: FALLITO")
		return exitFail
	}
	fmt.Fprintln(stdout, "\nESITO: OK")
	return exitOK
}

// nameFilter costruisce il filtro sui nomi delle immagini da -only e -exclude (espressioni regolari).
func nameFilter(only, exclude string) (func(string) bool, error) {
	var reOnly, reExclude *regexp.Regexp
	var err error
	if only != "" {
		if reOnly, err = regexp.Compile(only); err != nil {
			return nil, fmt.Errorf("-only non e' un'espressione regolare valida: %w", err)
		}
	}
	if exclude != "" {
		if reExclude, err = regexp.Compile(exclude); err != nil {
			return nil, fmt.Errorf("-exclude non e' un'espressione regolare valida: %w", err)
		}
	}
	return func(name string) bool {
		return (reOnly == nil || reOnly.MatchString(name)) && (reExclude == nil || !reExclude.MatchString(name))
	}, nil
}
