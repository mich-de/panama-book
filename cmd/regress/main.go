// Comando regress: harness di regressione di libretto. Esegue `libretto extract -i FILE` su un corpus di immagini
// (solo lettura: non modifica le immagini, non avvia lo scanner), riassume ogni esito in una riga di tabella TSV senza
// dati personali e, con -baseline, lo confronta con una prova precedente segnalando REGRESSIONI e MIGLIORAMENTI.
//
//	regress -exe libretto.exe -o docs\baseline-regressione.tsv                 crea la baseline
//	regress -exe _out\candidato.exe -baseline docs\baseline-regressione.tsv    confronta un candidato
//	regress -exe _out\candidato.exe scans\scana2.png scans\provamrz.png        solo alcune immagini
//
// Corpus predefinito (relativo a -root): scans\*.png, scans\vecchi test WIA\*.png (senza le pagine ritagliate
// *-pagina*.png) e facsimili\panama\*.jpg e *.png. La tabella va su stdout (o in -o); avanzamento e confronto su stderr.
// Uscita: 0 nessuna regressione, 1 regressioni, 2 errore d'uso, 130 interrotto.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"scanlibretto/internal/outsum"
)

const (
	exitOK         = 0
	exitRegression = 1
	exitUsage      = 2
	exitInterrupt  = 130
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run e' il corpo del comando, separato da main per poterlo provare nei test.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("regress", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "uso: regress [-exe libretto.exe] [-baseline FILE.tsv] [-o FILE.tsv [-force]] [-truth CARTELLA] [-j 3]")
		fmt.Fprintln(stderr, "             [-root .] [-only parole] [-skip parole] [-timeout 4m] [-list] [-q] [immagini...]")
		fmt.Fprintln(stderr, "     regress -baseline PRIMA.tsv -against DOPO.tsv      confronta due tabelle gia' prodotte, senza eseguire nulla")
		fs.PrintDefaults()
	}
	exe := fs.String("exe", "libretto.exe", "eseguibile di libretto da provare (se ne prova una copia)")
	baseline := fs.String("baseline", "", "tabella TSV di una prova precedente con cui confrontare")
	against := fs.String("against", "", "con -baseline: confronta le due tabelle gia' prodotte (prima = -baseline, dopo = questa), senza eseguire nulla")
	outPath := fs.String("o", "", "scrive la tabella in questo file invece che su stdout")
	force := fs.Bool("force", false, "consente a -o di sostituire un file esistente")
	truth := fs.String("truth", "", "cartella delle verita' (NOME.json per ogni NOME.png): conta anche i campi giusti")
	root := fs.String("root", ".", "radice del progetto: base dei percorsi relativi e cartella di lavoro dei processi")
	jobs := fs.Int("j", 3, "processi in parallelo")
	timeout := fs.Duration("timeout", 4*time.Minute, "tempo massimo per immagine")
	only := fs.String("only", "", "solo i file il cui percorso contiene una di queste parole (separate da virgola)")
	skip := fs.String("skip", "", "salta i file il cui percorso contiene una di queste parole (separate da virgola)")
	list := fs.Bool("list", false, "elenca il corpus ed esce, senza eseguire nulla")
	quiet := fs.Bool("q", false, "niente righe di avanzamento su stderr")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK // -h: l'uso e' gia' stato stampato
		}
		return exitUsage
	}
	fail := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "errore: "+format+"\n", a...)
		return exitUsage
	}
	if *jobs < 1 {
		return fail("-j deve essere almeno 1")
	}
	if *against != "" { // confronto tra due tabelle gia' scritte: niente corpus, niente esecuzioni
		if *baseline == "" {
			return fail("-against richiede -baseline")
		}
		before, err := loadTable(*baseline)
		if err != nil {
			return fail("baseline %q: %v", *baseline, err)
		}
		after, err := loadTable(*against)
		if err != nil {
			return fail("%q: %v", *against, err)
		}
		fmt.Fprintf(stderr, "prima (%s): %s\n", *baseline, totals(before))
		fmt.Fprintf(stderr, "dopo  (%s): %s\n", *against, totals(after))
		warnTruthChanged(stderr, tablePrint(*baseline), tablePrint(*against))
		if printComparison(stderr, fmt.Sprintf("confronto di %s con %s", *against, *baseline), compare(before, after)) {
			return exitRegression
		}
		return exitOK
	}

	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		return fail("radice: %v", err)
	}
	files := fs.Args()
	for i := range files {
		files[i] = relToRoot(rootAbs, files[i])
	}
	if len(files) == 0 {
		if files, err = discover(rootAbs); err != nil {
			return fail("corpus: %v", err)
		}
	}
	files = filterFiles(files, *only, *skip)
	if len(files) == 0 {
		return fail("nessuna immagine da provare")
	}
	if *list {
		for _, f := range files {
			fmt.Fprintln(stdout, f)
		}
		return exitOK
	}

	// controlli prima di spendere minuti di OCR
	exeAbs, err := filepath.Abs(*exe)
	if err != nil {
		return fail("eseguibile: %v", err)
	}
	if st, err := os.Stat(exeAbs); err != nil || st.IsDir() {
		return fail("eseguibile %q non trovato", *exe)
	}
	var base []Row
	if *baseline != "" {
		if base, err = loadTable(*baseline); err != nil {
			return fail("baseline %q: %v", *baseline, err)
		}
	}
	if *outPath != "" {
		if st, err := os.Stat(filepath.Dir(*outPath)); err != nil || !st.IsDir() {
			return fail("la cartella di %q non esiste", *outPath)
		}
		if _, err := os.Stat(*outPath); err == nil && !*force {
			return fail("%q esiste gia' (usa -force per sostituirlo)", *outPath)
		}
	}
	if *truth != "" {
		if st, err := os.Stat(*truth); err != nil || !st.IsDir() {
			return fail("-truth %q non e' una cartella", *truth)
		}
	}

	// si prova una copia dell'eseguibile (vedi copyExe)
	exeCopy, cleanup, err := copyExe(exeAbs)
	if err != nil {
		return fail("copia dell'eseguibile: %v", err)
	}
	defer cleanup()
	exeInfo, err := describeExe(exeCopy) // la copia mantiene nome e data di modifica dell'originale
	if err != nil {
		return fail("eseguibile: %v", err)
	}

	before := snapshot(rootAbs, files)
	var progress io.Writer = stderr
	if *quiet {
		progress = nil
	}
	fmt.Fprintf(stderr, "corpus: %d immagini; processi: %d; eseguibile: %s\n", len(files), *jobs, exeInfo)
	start := time.Now()
	truthDir := *truth
	truthFP, truthN := "", 0
	if truthDir != "" {
		if truthDir, err = filepath.Abs(truthDir); err != nil {
			return fail("-truth: %v", err)
		}
		if fp, n, err := truthPrint(truthDir); err == nil {
			truthFP, truthN = fp, n
		}
	}
	rows := runAll(ctx, config{exe: exeCopy, root: rootAbs, jobs: *jobs, timeout: *timeout, truth: truthDir}, files, progress)
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "interrotto: tabella non scritta")
		return exitInterrupt
	}
	wall := time.Since(start)

	code := exitOK
	if changed := changedSince(rootAbs, before); len(changed) > 0 {
		fmt.Fprintf(stderr, "ERRORE GRAVE: %d immagini cambiate durante la prova (l'harness non le tocca: l'ha fatto qualcun altro): %s\n",
			len(changed), strings.Join(changed, ", "))
		code = exitUsage
	}

	meta := tableMeta(exeInfo, *truth, truthFP, truthN, files, rows, wall, *jobs)
	if err := emitTable(*outPath, stdout, meta, rows); err != nil {
		return fail("tabella: %v", err)
	}
	for _, r := range rows {
		if r.Err || r.Note != "" {
			fmt.Fprintf(stderr, "avviso: %s: %s\n", r.File, r.Note)
		}
	}
	fmt.Fprintln(stderr, summaryLine(rows, wall))

	if *baseline != "" {
		warnTruthChanged(stderr, tablePrint(*baseline), truthFP)
		if printComparison(stderr, "confronto con "+*baseline, compare(base, rows)) && code == exitOK {
			code = exitRegression
		}
	}
	return code
}

// emitTable scrive la tabella in path (se non vuoto) o su stdout.
func emitTable(path string, stdout io.Writer, meta []string, rows []Row) error {
	if path == "" {
		return writeTable(stdout, meta, rows)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := writeTable(f, meta, rows); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// tableMeta sono le righe di commento in testa alla tabella: chi ha prodotto i numeri e come leggere le mappe.
// truthFP e truthN (impronta e numero dei file della verita') sono vuoti se non si e' usato -truth.
func tableMeta(exeInfo, truthDir, truthFP string, truthN int, files []string, rows []Row, wall time.Duration, jobs int) []string {
	truth := "-"
	if truthDir != "" {
		truth = filepath.ToSlash(truthDir)
	}
	meta := []string{
		"regress: esiti di libretto extract, senza dati personali",
		"eseguibile: " + exeInfo,
		fmt.Sprintf("data: %s; immagini: %d; processi: %d; durata: %.0f s; verita: %s",
			time.Now().Format("2006-01-02T15:04:05"), len(files), jobs, wall.Seconds(), truth),
	}
	if truthFP != "" {
		meta = append(meta, fmt.Sprintf("%s%s (%d file)", strings.TrimPrefix(truthPrintPrefix, "# "), truthFP, truthN))
	}
	return append(meta,
		"mappa_check (ordine): "+strings.Join(outsum.CheckKeys[:], " ")+"; 1 valido, 0 non valido, - non disponibile",
		"mappa_campi (ordine): "+strings.Join(outsum.FieldKeys[:], " ")+"; 1 letto, 0 non letto, - non disponibile",
		"mappa_giusti (stesso ordine): G giusto, S sbagliato, M mancante, X letto ma vuoto in verita', - niente da confrontare",
		"nomi: cognome e nomi della riga 1 dell'MRZ contro la riga del nome in verita'; G uguali, S diversi, M MRZ non trovato, - niente da confrontare",
		"totali: "+totals(rows),
	)
}

// warnTruthChanged avverte se due tabelle sono state prodotte con verita' diverse: i campi giusti non sono confrontabili.
func warnTruthChanged(w io.Writer, basePrint, curPrint string) {
	if basePrint != "" && curPrint != "" && basePrint != curPrint {
		fmt.Fprintf(w, "ATTENZIONE: la verita' non e' la stessa (impronta %s contro %s): i campi giusti non sono confrontabili; rigenera la baseline con -truth\n",
			basePrint, curPrint)
	}
}

// totals e' il riassunto numerico dell'intera prova.
func totals(rows []Row) string {
	var mrz, all5, errs, checks, fields, rightN, rightM, withTruth, nameG, nameS, nameM int
	for _, r := range rows {
		if r.Err {
			errs++
			continue
		}
		switch r.nameSym() {
		case symRight:
			nameG++
		case symWrong:
			nameS++
		case symMissing:
			nameM++
		}
		if r.MRZ {
			mrz++
		}
		if r.checksOK() == outsum.NChecks {
			all5++
		}
		checks += r.checksOK()
		fields += r.found()
		if r.Truth {
			withTruth++
			n, m := r.right()
			rightN, rightM = rightN+n, rightM+m
		}
	}
	s := fmt.Sprintf("MRZ %d/%d; check 5/5 %d/%d; check %d/%d; campi %d/%d; errori %d",
		mrz, len(rows), all5, len(rows), checks, len(rows)*outsum.NChecks, fields, len(rows)*outsum.NFields, errs)
	if withTruth > 0 {
		s += fmt.Sprintf("; giusti %d/%d (su %d immagini con verita')", rightN, rightM, withTruth)
	}
	if nameG+nameS+nameM > 0 {
		s += fmt.Sprintf("; nomi MRZ giusti %d/%d (senza MRZ %d)", nameG, nameG+nameS, nameM)
	}
	return s
}

func summaryLine(rows []Row, wall time.Duration) string {
	return fmt.Sprintf("totali: %s; durata %.0f s", totals(rows), wall.Seconds())
}

// loadTable legge una tabella TSV prodotta da regress (baseline o prova salvata con -o).
func loadTable(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readTable(f)
}

// printComparison stampa il confronto (titolo in header) e dice se c'e' almeno una regressione.
func printComparison(w io.Writer, header string, o Outcome) (regressed bool) {
	fmt.Fprintf(w, "== %s\n", header)
	for _, f := range o.Findings {
		fmt.Fprintf(w, "%-13s %s: %s\n", f.Kind, f.File, f.Text)
	}
	fmt.Fprintf(w, "== esito: %d con regressioni, %d con miglioramenti, %d invariate; nuove %d, assenti %d\n",
		o.Regressed, o.Improved, o.Same, o.New, o.Absent)
	if o.Regressed > 0 {
		fmt.Fprintln(w, "== REGRESSIONI TROVATE")
	}
	return o.Regressed > 0
}
