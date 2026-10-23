// Comando libretto: scansione di una sola facciata con lo scanner fi-800R ed estrazione dei dati
// dalla pagina dati di una Libreta de embarque (Panama).
//
//	libretto info                       diagnostica dello scanner (non scansiona)
//	libretto scan    -o scan.png        scansiona una facciata (alimentatore, niente duplex)
//	libretto extract -i scan.png        estrae i dati da un'immagine gia' acquisita
//	libretto read    -o scan.png        scansiona e poi estrae
//
// Prima di avviare lo scanner (scan e read) il programma chiede conferma: il documento deve essere gia' inserito.
// Con -yes la conferma e' saltata (per gli script). Un file di uscita esistente non viene sovrascritto senza -force.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/libreta"
	"scanlibretto/internal/wia"
)

// Codici di uscita: 1 errore generico, 2 uso errato, 3 carta assente, 4 inceppamento, 130 interrotto da Ctrl+C.
const (
	exitError   = 1
	exitUsage   = 2
	exitNoPaper = 3
	exitJam     = 4
	exitSigint  = 130
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "info":
		err = cmdInfo(os.Args[2:])
	case "area-probe": // diagnostica nascosta: solo proprieta' dell'item, nessuna acquisizione
		err = cmdAreaProbe(os.Args[2:])
	case "scan":
		err = cmdScan(os.Args[2:])
	case "extract":
		err = cmdExtract(os.Args[2:])
	case "pdf":
		err = cmdPDF(os.Args[2:])
	case "rename":
		err = cmdRename(os.Args[2:])
	case "summary":
		err = cmdSummary(os.Args[2:])
	case "read":
		err = cmdRead(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		os.Exit(exitCode(err))
	}
}

// withInterrupt restituisce un contesto annullato da Ctrl+C. Si usa solo per la lettura (OCR), mai durante la
// conferma o la scansione: li' Ctrl+C deve terminare il processo come di consueto, perche' un trasferimento
// WIA in corso non si puo' annullare in modo pulito.
func withInterrupt() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func exitCode(err error) int {
	switch {
	case errors.Is(err, context.Canceled):
		return exitSigint
	case wia.IsPaperEmpty(err), errors.Is(err, errTwainNoPaper):
		return exitNoPaper
	case wia.IsPaperJam(err), errors.Is(err, errTwainJam):
		return exitJam
	}
	return exitError
}

func usage() {
	fmt.Fprint(os.Stderr, `uso:
  libretto info
  libretto scan    [-o file.png] [-dpi 300] [-gray] [-yes] [-force]
  libretto pdf     -i scan.png [-o scan.pdf] [-max 1000000]   PDF di una pagina entro 1 MB (JPEG a qualita' massima che sta nel limite)
  libretto rename  -json scans/NOME.json [-n]   rinomina immagine, JSON e PDF in "NOME - COGNOME - NUMERO LIBRETTO" (solo con MRZ valida)
  libretto summary risultato.json     riassume in una riga un JSON gia' prodotto
  libretto extract -i scan.png [-page-out pagina.png] [-force] [-debug cartella]
  libretto read    [-o file.png] [-dpi 300] [-gray] [-yes] [-force] [-page-out pagina.png] [-debug cartella]

  -o      file di uscita (predefinito: scans\scan-AAAAMMGG-hhmmss.png)
  -rotate ruota l'immagine salvata di 90, 180 o 270 gradi (180 per la striscia MRZ letta con TWAIN)
  -driver wia (predefinito) o twain: con twain l'inizio dell'area (-from) e' rispettato (serve twainscan32.exe)
  -yes    non chiedere conferma prima di avviare lo scanner
  -force  consente di sostituire un file di uscita esistente
  -from   inizia a leggere dopo N mm dal bordo d'ingresso (es. 90 se la pagina dati e' la seconda del foglio)
  -len    lunghezza letta in mm a partire da -from (es. 100 per la sola pagina dati)
  -width  legge solo N mm di larghezza, centrati sull'apertura
  -raw    salva anche la scansione grezza, prima del ritaglio (misura dove cade il documento)
  -page-only  il file di uscita (-o) contiene solo la pagina dati ritagliata (125x88 mm): CONTIENE DATI PERSONALI
  -top-margin  mm lasciati sopra il titolo REPUBLICA DE PANAMA nella pagina dati salvata (predefinito 2, -1 = pagina intera)
  -page-out  salva la sola pagina dati ritagliata (125x88 mm, trovata dalla posizione dell'MRZ): CONTIENE DATI PERSONALI
  -debug  salva immagini e testo OCR intermedi: CONTENGONO DATI PERSONALI, da eliminare a fine lavoro
`)
	os.Exit(exitUsage)
}

func cmdInfo(args []string) error {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	dev := fs.String("device", "fi-800R", "parte del nome dello scanner")
	_ = fs.Parse(args)
	i, err := wia.Describe(*dev)
	if err != nil {
		return err
	}
	fmt.Printf("%s (%s) driver %s porta %s\n", i.Name, i.Manufacturer, i.DriverVersion, i.Port)
	fmt.Printf("risoluzione WIA: %d-%d dpi\n", i.DPIMin, i.DPIMax)
	fmt.Printf("Document Handling: capacita'=%d (1 alimentatore, 4 duplex) selezione=%d stato=%d (bit 1 = alimentatore pronto; non indica la presenza di carta)\n",
		i.HandlingCapabilities, i.HandlingSelect, i.HandlingStatus)
	return nil
}

// cmdAreaProbe (nascosto) scrive/rilegge start ed extent dell'item per capire i limiti del driver. Non scansiona.
func cmdAreaProbe(args []string) error {
	fs := flag.NewFlagSet("area-probe", flag.ExitOnError)
	dpi := fs.Int("dpi", 300, "risoluzione")
	start := fs.Int("start", 1063, "Vertical Start Position in pixel")
	ext := fs.Int("extent", 1181, "Vertical Extent in pixel")
	_ = fs.Parse(args)
	lines, err := wia.ProbeArea(*dpi, *start, *ext)
	for _, l := range lines {
		fmt.Println(l)
	}
	return err
}

// scanConfig raccoglie le opzioni dei comandi che avviano lo scanner.
type scanConfig struct {
	opts     wia.Options
	rotate   int    // gradi (0, 90, 180, 270) con cui si salva l'immagine, es. 180 per la striscia MRZ letta con TWAIN
	driver   string // "wia" (predefinito) o "twain" (PaperStream IP, via twainscan32.exe)
	debug    string
	pageOut  string
	pageOnly bool
	raw      string
	yes      bool
}

func scanFlags(name string, args []string) scanConfig {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	var c scanConfig
	fs.StringVar(&c.opts.DeviceMatch, "device", "fi-800R", "parte del nome dello scanner")
	fs.IntVar(&c.opts.DPI, "dpi", 300, "risoluzione 75-600")
	fs.IntVar(&c.rotate, "rotate", 0, "ruota l'immagine salvata di 0, 90, 180 o 270 gradi (180: la striscia letta con TWAIN esce capovolta)")
	fs.StringVar(&c.driver, "driver", defaultDriver, "driver dello scanner: wia o twain (PaperStream IP, rispetta l'inizio dell'area)")
	fs.BoolVar(&c.opts.Gray, "gray", false, "scala di grigi")
	fs.StringVar(&c.opts.OutPath, "o", defaultOutPath(time.Now()), "file PNG di uscita")
	fs.Float64Var(&c.opts.StartMM, "from", 0, "inizia a leggere dopo N mm dal bordo d'ingresso (0 = dall'inizio)")
	fs.Float64Var(&c.opts.HeightMM, "len", 0, "lunghezza letta in mm dal bordo d'ingresso (0 = tutto il campo, 356 mm)")
	fs.Float64Var(&c.opts.WidthMM, "width", 0, "larghezza letta in mm, centrata (0 = tutto il campo, 216 mm)")
	fs.BoolVar(&c.opts.Force, "force", false, "sostituisci il file di uscita se esiste")
	fs.BoolVar(&c.yes, "yes", false, "non chiedere conferma prima di scansionare")
	fs.StringVar(&c.debug, "debug", "", "cartella per immagini e testo OCR intermedi (contengono dati personali)")
	fs.StringVar(&c.pageOut, "page-out", "", "salva qui la sola pagina dati ritagliata (PNG, contiene dati personali)")
	fs.BoolVar(&c.pageOnly, "page-only", false, "il file di uscita (-o) contiene solo la pagina dati ritagliata, non la scansione intera")
	fs.Float64Var(&topMarginMM, "top-margin", topMarginMM, "mm lasciati sopra il titolo REPUBLICA DE PANAMA nella pagina dati salvata (-1 = pagina intera)")
	fs.StringVar(&c.raw, "raw", "", "salva qui anche la scansione grezza, prima del ritaglio (serve a misurare dove cade il documento)")
	_ = fs.Parse(args)
	return c
}

// defaultOutPath da' a ogni scansione un nome diverso, cosi' la successiva non sovrascrive la precedente.
func defaultOutPath(t time.Time) string {
	return filepath.Join("scans", "scan-"+t.Format("20060102-150405")+".png")
}

// warnDebug avvisa che la cartella di debug conterra' dati personali.
func warnDebug(dir string) {
	if dir != "" {
		fmt.Fprintf(os.Stderr, "attenzione: -debug salva immagini e testo OCR con dati personali in %s; eliminala a fine lavoro\n", dir)
	}
}

// confirmScan chiede all'operatore di confermare che il documento e' inserito. Senza terminale interattivo
// rifiuta di procedere: negli script serve -yes esplicito.
func confirmScan(yes bool) error {
	if yes {
		return nil
	}
	st, err := os.Stdin.Stat()
	if err != nil || st.Mode()&os.ModeCharDevice == 0 {
		return errors.New("input non interattivo: per avviare lo scanner senza conferma usa -yes")
	}
	fmt.Fprint(os.Stderr, "Documento inserito? Premi Invio per scansionare, Ctrl+C per annullare: ")
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		return fmt.Errorf("conferma non letta: %w", err)
	}
	return nil
}

// scanAndPost chiede conferma, scansiona una facciata, scarta le scansioni vuote e ritaglia l'immagine al solo documento.
func scanAndPost(c scanConfig) error {
	o := c.opts
	if c.rotate%90 != 0 || c.rotate < 0 || c.rotate > 270 {
		return fmt.Errorf("-rotate %d non valido: usa 0, 90, 180 o 270", c.rotate)
	}
	if err := os.MkdirAll(filepath.Dir(o.OutPath), 0o700); err != nil {
		return fmt.Errorf("cartella di uscita: %w", err)
	}
	if err := confirmScan(c.yes); err != nil {
		return err
	}
	if o.StartMM > 0 || o.HeightMM > 0 || o.WidthMM > 0 {
		fmt.Fprintf(os.Stderr, "area richiesta: da %.0f mm, lunghezza %.0f mm, larghezza %.0f mm (0 = tutto il campo)\n", o.StartMM, o.HeightMM, o.WidthMM)
	}
	if err := acquire(c); err != nil {
		if errors.Is(err, errTwainNoPaper) || wia.IsPaperEmpty(err) {
			return fmt.Errorf("%w (suggerimento: inserisci il documento dal fronte per libretti e carte, o nel caricatore per i fogli, e riprova)", err)
		}
		return err
	}
	if c.raw != "" {
		if err := saveRaw(o.OutPath, c.raw, o.Force); err != nil {
			fmt.Fprintln(os.Stderr, "avviso:", err) // accessorio: la scansione resta valida
		}
	}
	img, err := imgproc.Load(o.OutPath)
	if err != nil {
		return fmt.Errorf("lettura della scansione: %w", err)
	}
	if imgproc.IsBlank(img) {
		return errors.New("scansione vuota: nessun documento rilevato (un foglio bianco non e' distinguibile dallo sfondo)")
	}
	out, _ := imgproc.AutoCropInfo(img)
	out = imgproc.TrimWhite(out)
	if c.rotate != 0 {
		out = imgproc.Rotate(out, c.rotate)
	}
	return imgproc.SavePNG(out, o.OutPath)
}

// saveRaw copia la scansione cosi' come esce dallo scanner, prima di ritaglio e controllo del vuoto.
func saveRaw(src, dst string, force bool) error {
	if _, err := os.Stat(dst); err == nil && !force {
		return fmt.Errorf("scansione grezza non salvata: %q esiste gia' (usa -force)", dst)
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "attenzione:", dst, "contiene dati personali")
	return os.WriteFile(dst, b, 0o600)
}

func cmdScan(args []string) error {
	c := scanFlags("scan", args)
	if err := scanAndPost(c); err != nil {
		return err
	}
	fmt.Println("salvato", c.opts.OutPath)
	return nil
}

// checkPageOut rifiuta subito un file di -page-out gia' esistente: per read prima di scansionare, non dopo.
func checkPageOut(pageOut string, force bool) error {
	if pageOut == "" || force {
		return nil
	}
	if _, err := os.Stat(pageOut); err == nil {
		return fmt.Errorf("pagina dati: %q esiste gia' (usa -force)", pageOut)
	}
	return nil
}

func cmdExtract(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	in := fs.String("i", "", "immagine da leggere")
	debug := fs.String("debug", "", "cartella per immagini e testo OCR intermedi (contengono dati personali)")
	pageOut := fs.String("page-out", "", "salva qui la sola pagina dati ritagliata (PNG, contiene dati personali)")
	force := fs.Bool("force", false, "sostituisci il file di -page-out se esiste")
	fs.Float64Var(&topMarginMM, "top-margin", topMarginMM, "mm lasciati sopra il titolo REPUBLICA DE PANAMA nella pagina dati salvata (-1 = pagina intera)")
	_ = fs.Parse(args)
	if *in == "" {
		usage()
	}
	if err := checkPageOut(*pageOut, *force); err != nil {
		return err
	}
	warnDebug(*debug)
	return extract(*in, *debug, *pageOut, *force)
}

func cmdRead(args []string) error {
	c := scanFlags("read", args)
	if err := checkPageOut(c.pageOut, c.opts.Force); err != nil { // prima di avviare lo scanner
		return err
	}
	warnDebug(c.debug)
	if err := scanAndPost(c); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "salvato", c.opts.OutPath)
	if c.pageOnly { // il file principale diventa la sola pagina dati (se non si trova, resta la scansione intera)
		return extract(c.opts.OutPath, c.debug, c.opts.OutPath, true)
	}
	return extract(c.opts.OutPath, c.debug, c.pageOut, c.opts.Force)
}

// extract legge la scansione. Ctrl+C annulla l'OCR in corso (uscita 130).
func extract(path, debugDir, pageOut string, force bool) error {
	ctx, stop := withInterrupt()
	defer stop()
	res, err := libreta.Extract(ctx, path, libreta.Options{DebugDir: debugDir})
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("interrotto: %w", ctx.Err())
		}
		return err
	}
	if pageOut != "" {
		if err := savePage(path, res.Rotation, pageOut, force); err != nil {
			// il ritaglio e' accessorio: i dati estratti si stampano comunque
			fmt.Fprintln(os.Stderr, "avviso:", err)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(toOutput(res))
}

// topMarginMM e' quanto si lascia sopra il titolo "REPUBLICA DE PANAMA" nella pagina dati salvata (-top-margin);
// negativo = pagina intera 125x88 mm, senza tagliare sopra il titolo.
var topMarginMM = 2.0

// savePage salva la sola pagina dati ritagliata. Non sovrascrive un file esistente senza force.
func savePage(path string, rotation int, out string, force bool) error {
	if _, err := os.Stat(out); err == nil && !force {
		return fmt.Errorf("pagina dati non salvata: %q esiste gia' (usa -force)", out)
	}
	img, ok, err := libreta.DataPage(path, rotation)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("pagina dati non ritagliata: righe MRZ non trovate")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "attenzione:", out, "contiene dati personali")
	return imgproc.SavePNG(imgproc.TrimAboveHeader(img, topMarginMM), out)
}
