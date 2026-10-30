// Comando libretto2 (versione 1.0.2): scansione di un foglio A4 con lo scanner Epson WorkForce DS-1630 e ritaglio
// dell'intero passaporto aperto (informativa + pagina dati, 125x176 mm), come nei facsimili AMP di Panama.
//
//	libretto2 info                          diagnostica dello scanner (non scansiona)
//	libretto2 scan -o libretto.png          scansiona un A4 e salva il solo passaporto ritagliato
//	libretto2 read -o libretto.png          scansiona, ritaglia e stampa il JSON dei dati (MRZ + campi stampati)
//	libretto2 read -i foglio.png            come read, da un'immagine gia' acquisita (nessuno scanner)
//	libretto2 crop -i foglio.png -o out.png ritaglia il passaporto da un'immagine gia' acquisita
//
// La v1 (cmd/libretto, fi-800R) non e' toccata: libretto2 riusa solo i pacchetti internal/wia e internal/imgproc.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/libreta"
	"scanlibretto/internal/outjson"
	"scanlibretto/internal/pdfout"
	"scanlibretto/internal/wia"
)

// Codici di uscita: come la v1.
const (
	exitError   = 1
	exitUsage   = 2
	exitNoPaper = 3
	exitJam     = 4
)

const (
	defaultDevice = "auto" // DS-1630 se c'e', altrimenti fi-800R (vedi resolveDevice)
	defaultMargin = 2.0    // mm di carta lasciati attorno al libretto con -canvas book
	defaultAbove  = 5.0    // mm di piega e informativa lasciati sopra la pagina dati (bordo alto)
	defaultCanvas = "book" // solo il libretto con defaultMargin attorno; per il riquadro 3:4 di 140x187 mm usa -canvas 140x187
)

// canvasSpec e' il valore di -canvas ("LxA" in mm oppure "book" = solo il libretto con -margin attorno).
var canvasSpec = defaultCanvas

func canvasFlag(fs *flag.FlagSet) {
	fs.StringVar(&canvasSpec, "canvas", defaultCanvas, "misura del risultato in mm, es. 140x187 (proporzione 3:4, bordo bianco largo); book (predefinito) = libretto con -margin attorno")
}

// frameFromSpec traduce -canvas in una cornice.
func frameFromSpec(spec string, marginMM float64) (imgproc.BookFrame, error) {
	if spec == "book" || spec == "" {
		return imgproc.BookFrame{MarginMM: marginMM}, nil
	}
	var w, h float64
	if n, err := fmt.Sscanf(spec, "%fx%f", &w, &h); n != 2 || err != nil || w < 126 || h < 177 || w > 400 || h > 400 {
		return imgproc.BookFrame{}, fmt.Errorf("-canvas %q non valido: usa LxA in mm (almeno 126x177, il libretto e' 125x176) oppure book", spec)
	}
	return imgproc.BookFrame{CanvasW: w, CanvasH: h}, nil
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "info":
		err = cmdInfo(os.Args[2:])
	case "scan":
		err = cmdScan(os.Args[2:])
	case "crop":
		err = cmdCrop(os.Args[2:])
	case "sheet": // foglio A4 intero, senza ritaglio: per i certificati (SAT, SD, SSO, CT)
		err = cmdSheet(os.Args[2:])
	case "read":
		err = cmdRead(os.Args[2:])
	case "extract": // compatibile con la GUI panamabook (stessi argomenti di libretto extract)
		err = cmdExtract(os.Args[2:])
	case "pdf": // compatibile con la GUI panamabook (stessi argomenti di libretto pdf)
		err = cmdPDF(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		os.Exit(exitCode(err))
	}
}

func exitCode(err error) int {
	switch {
	case wia.IsPaperEmpty(err):
		return exitNoPaper
	case wia.IsPaperJam(err):
		return exitJam
	}
	return exitError
}

func usage() {
	fmt.Fprint(os.Stderr, `libretto2 1.0.2 - Epson DS-1630, passaporto intero (informativa + pagina dati)
uso:
  libretto2 info  [-device DS-1630]
  libretto2 scan  [-o file.png] [-dpi 300] [-gray] [-margin 2] [-raw foglio.png] [-yes] [-force]
  libretto2 sheet [-o foglio.png] [-dpi 300] [-gray] [-source auto] [-yes] [-force]   scansiona un A4 e lo salva intero, senza ritaglio (certificati)
  libretto2 crop  -i foglio.png [-o file.png] [-margin 2] [-force]
  libretto2 pdf  -i immagine.png [-o file.pdf] [-max 1000000] [-dpi 300] [-force]   PDF di una pagina entro 1 MB (la GUI lo cerca come «libretto pdf»)
  libretto2 extract -i foglio.png [-page-out pagina-dati.png] [-force]   come la v1: JSON su stdout; -page-out = sola pagina dati (usato dalla GUI)
  libretto2 read  [-i foglio.png] [-o file.png] [-dpi 300] [-margin 2] [-yes] [-force]   scansiona (o legge -i), ritaglia, JSON su stdout

  -o       file di uscita (predefinito: scans\book-AAAAMMGG-hhmmss.png): CONTIENE DATI PERSONALI
  -device  parte del nome dello scanner WIA (predefinito DS-1630)
  -dpi     risoluzione (predefinito 300)
  -canvas  misura del risultato in mm (es. 140x187 = proporzione 3:4 con bordo bianco largo); book (predefinito) = solo libretto con -margin attorno
  -margin  con -canvas book: mm di carta lasciati attorno al libretto (predefinito 2)
  -raw     salva anche il foglio A4 grezzo, prima del ritaglio
  -yes     non chiedere conferma prima di avviare lo scanner
  -force   consente di sostituire un file esistente
`)
	os.Exit(exitUsage)
}

// resolveDevice traduce -device: "auto" sceglie lo scanner collegato tra quelli noti (Epson DS-1630, poi Fujitsu fi-800R); un
// altro valore e' una parte del nome e si usa com'e'.
func resolveDevice(match string) (string, error) {
	if match != "auto" {
		return match, nil
	}
	list, err := wia.List()
	if err != nil {
		return "", err
	}
	for _, want := range []string{"DS-1630", "fi-800R"} {
		for _, d := range list {
			if strings.Contains(strings.ToLower(d.Name), strings.ToLower(want)) {
				if want != "DS-1630" {
					fmt.Fprintf(os.Stderr, "attenzione: Epson DS-1630 non trovato, uso %s\n", d.Name)
				}
				return want, nil
			}
		}
	}
	names := make([]string, 0, len(list))
	for _, d := range list {
		names = append(names, d.Name)
	}
	return "", fmt.Errorf("nessuno scanner noto collegato (cercati DS-1630 e fi-800R; trovati: %q): usa -device NOME", names)
}

func cmdInfo(args []string) error {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	dev := fs.String("device", defaultDevice, "parte del nome dello scanner")
	_ = fs.Parse(args)
	match, err := resolveDevice(*dev)
	if err != nil {
		return err
	}
	i, err := wia.Describe(match)
	if err != nil {
		return err
	}
	fmt.Printf("%s (%s) driver %s porta %s\n", i.Name, i.Manufacturer, i.DriverVersion, i.Port)
	fmt.Printf("risoluzione WIA: %d-%d dpi\n", i.DPIMin, i.DPIMax)
	fmt.Printf("Document Handling: capacita'=%d (1 alimentatore, 2 piano di vetro, 4 duplex) selezione=%d stato=%d\n",
		i.HandlingCapabilities, i.HandlingSelect, i.HandlingStatus)
	return nil
}

// scanFlags legge le opzioni comuni a scan e read.
func scanFlags(name string, args []string) (o wia.Options, margin float64, raw string, yes bool, in string) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.StringVar(&o.DeviceMatch, "device", defaultDevice, "parte del nome dello scanner")
	fs.IntVar(&o.DPI, "dpi", 300, "risoluzione")
	fs.BoolVar(&o.Gray, "gray", false, "scala di grigi")
	fs.StringVar(&o.Source, "source", wia.SourceAuto, "da dove scansionare: auto (caricatore, se vuoto il piano di vetro), feeder o flatbed")
	fs.StringVar(&o.OutPath, "o", "", "file PNG di uscita (predefinito: scans\book-AAAAMMGG-hhmmss.png)")
	fs.BoolVar(&o.Force, "force", false, "sostituisci il file di uscita se esiste")
	fs.Float64Var(&margin, "margin", defaultMargin, "mm di carta attorno al libretto (solo con -canvas book)")
	canvasFlag(fs)
	fs.StringVar(&raw, "raw", "", "salva qui anche il foglio grezzo")
	fs.BoolVar(&yes, "yes", false, "non chiedere conferma")
	if name == "read" {
		fs.StringVar(&in, "i", "", "immagine gia' acquisita: nessuno scanner")
	}
	// opzioni della v1 accettate e ignorate: la GUI panamabook le passa a libretto.exe (-libretto libretto2.exe)
	fs.String("driver", "", "ignorato (la v2 usa solo WIA)")
	fs.Float64("len", 0, "ignorato (la v2 scansiona il foglio A4 intero)")
	fs.Float64("from", 0, "ignorato")
	fs.Float64("width", 0, "ignorato")
	fs.Int("rotate", 0, "ignorato")
	fs.Float64("top-margin", 0, "ignorato")
	_ = fs.Parse(args)
	return
}

// acquireBook porta in o.OutPath il passaporto ritagliato: da in (se dato) oppure scansionando un A4.
// Restituisce anche il percorso del foglio non ritagliato (src), da cui read legge i dati: l'OCR sul foglio intero
// da' risultati migliori che sul ritaglio. cleanup elimina il file temporaneo, se ce n'e' uno.
func acquireBook(o wia.Options, margin float64, raw string, yes bool, in string) (out, src string, cleanup func(), err error) {
	cleanup = func() {}
	if in != "" {
		img, err := imgproc.Load(in)
		if err != nil {
			return "", "", cleanup, fmt.Errorf("lettura immagine: %w", err)
		}
		if o.OutPath == "" {
			o.OutPath = in[:len(in)-len(filepath.Ext(in))] + "-book.png"
		}
		if _, err := os.Stat(o.OutPath); err == nil && !o.Force {
			return "", "", cleanup, fmt.Errorf("%q esiste gia' (usa -force)", o.OutPath)
		}
		return o.OutPath, in, cleanup, saveBook(img, o.OutPath, margin)
	}
	if o.OutPath == "" {
		o.OutPath = filepath.Join("scans", "book-"+time.Now().Format("20060102-150405")+".png")
	}
	if err := os.MkdirAll(filepath.Dir(o.OutPath), 0o700); err != nil {
		return "", "", cleanup, fmt.Errorf("cartella di uscita: %w", err)
	}
	if err := confirmScan(yes); err != nil {
		return "", "", cleanup, err
	}
	match, err := resolveDevice(o.DeviceMatch)
	if err != nil {
		return "", "", cleanup, err
	}
	o.DeviceMatch = match
	// foglio intero: nessuna area limitata (A4 sta nel campo dell'alimentatore)
	if err := wia.Scan(o); err != nil {
		if wia.IsPaperEmpty(err) {
			err = fmt.Errorf("%w (suggerimento: appoggia il foglio A4 sul vetro oppure inseriscilo nel caricatore, e riprova)", err)
		}
		return "", "", cleanup, err
	}
	img, err := imgproc.Load(o.OutPath)
	if err != nil {
		return "", "", cleanup, fmt.Errorf("lettura della scansione: %w", err)
	}
	if imgproc.IsBlank(img) {
		return "", "", cleanup, errors.New("scansione vuota: nessun documento rilevato")
	}
	if raw == "" { // il foglio serve comunque a read: file temporaneo accanto all'uscita
		tmp, err := os.CreateTemp(filepath.Dir(o.OutPath), "foglio-*.png")
		if err != nil {
			return "", "", cleanup, err
		}
		tmp.Close()
		raw = tmp.Name()
		cleanup = func() { os.Remove(raw) }
	} else {
		fmt.Fprintln(os.Stderr, "attenzione:", raw, "contiene dati personali")
	}
	if err := imgproc.SavePNG(img, raw); err != nil {
		cleanup()
		return "", "", func() {}, fmt.Errorf("foglio grezzo non salvato: %w", err)
	}
	return o.OutPath, raw, cleanup, saveBook(img, o.OutPath, margin)
}

// cmdSheet scansiona un foglio A4 e lo salva cosi' com'e', senza cercare il libretto: serve per i certificati.
func cmdSheet(args []string) error {
	o, _, _, yes, _ := scanFlags("sheet", args)
	if o.OutPath == "" {
		o.OutPath = filepath.Join("scans", "foglio-"+time.Now().Format("20060102-150405")+".png")
	}
	if err := os.MkdirAll(filepath.Dir(o.OutPath), 0o700); err != nil {
		return fmt.Errorf("cartella di uscita: %w", err)
	}
	if err := confirmScan(yes); err != nil {
		return err
	}
	match, err := resolveDevice(o.DeviceMatch)
	if err != nil {
		return err
	}
	o.DeviceMatch = match
	if err := wia.Scan(o); err != nil {
		if wia.IsPaperEmpty(err) {
			err = fmt.Errorf("%w (suggerimento: appoggia il foglio A4 sul vetro oppure inseriscilo nel caricatore, e riprova)", err)
		}
		return err
	}
	img, err := imgproc.Load(o.OutPath)
	if err != nil {
		return fmt.Errorf("lettura della scansione: %w", err)
	}
	if imgproc.IsBlank(img) {
		_ = os.Remove(o.OutPath)
		return errors.New("scansione vuota: nessun documento rilevato")
	}
	fmt.Println("salvato", o.OutPath)
	return nil
}

func cmdScan(args []string) error {
	o, margin, raw, yes, _ := scanFlags("scan", args)
	out, _, cleanup, err := acquireBook(o, margin, raw, yes, "")
	defer cleanup()
	if err != nil {
		return err
	}
	fmt.Println("salvato", out)
	return nil
}

// cmdRead acquisisce e ritaglia il passaporto, poi legge MRZ e campi stampati e stampa il JSON (schema della v1).
func cmdRead(args []string) error {
	o, margin, raw, yes, in := scanFlags("read", args)
	out, src, cleanup, err := acquireBook(o, margin, raw, yes, in)
	defer cleanup()
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "salvato", out)
	return extractJSON(src, "", "", 0, 0)
}

// extractJSON legge MRZ e campi stampati da src e stampa il JSON (schema della v1) su stdout. Con pageOut salva anche la
// SOLA PAGINA DATI (125x88 mm, dritta, topMarginMM sopra il titolo): e' la "pagina dati" della GUI, scelta in alternativa alla
// pagina intera. Se l'MRZ non si trova la pagina non si salva.
func extractJSON(src, debugDir, pageOut string, topMarginMM, aboveMM float64) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	res, err := libreta.Extract(ctx, src, libreta.Options{DebugDir: debugDir})
	if err != nil {
		return err
	}
	if pageOut != "" {
		if err := saveDataPage(src, res.Rotation, pageOut, topMarginMM, aboveMM); err != nil {
			fmt.Fprintln(os.Stderr, "avviso:", err) // accessorio: i dati si stampano comunque
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(outjson.From(res))
}

// saveDataPage salva la sola pagina dati ritagliata dall'MRZ (stessa logica della v1: libreta.DataPage + TrimAboveHeader).
// Con topMarginMM < 0 (pagina intera) la pagina sale con aboveMM in piu' sopra, fino al bordo alto.
func saveDataPage(src string, rotation int, out string, topMarginMM, aboveMM float64) error {
	if topMarginMM >= 0 {
		aboveMM = 0
	}
	img, ok, err := libreta.DataPageAbove(src, rotation, aboveMM)
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

// cmdExtract e' "libretto extract" della v1: legge un'immagine gia' acquisita. -page-out salva la sola pagina dati.
func cmdExtract(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	in := fs.String("i", "", "immagine da leggere")
	debug := fs.String("debug", "", "cartella per immagini e testo OCR intermedi (contengono dati personali)")
	pageOut := fs.String("page-out", "", "salva qui la sola pagina dati ritagliata (PNG, contiene dati personali)")
	force := fs.Bool("force", false, "sostituisci il file di -page-out se esiste")
	top := fs.Float64("top-margin", -1, "mm lasciati sopra il titolo REPUBLICA DE PANAMA nella pagina dati (-1 = pagina intera 125x88)")
	above := fs.Float64("above", defaultAbove, "mm in piu' sopra la pagina dati fino al bordo alto (solo con -top-margin -1)")
	_ = fs.Parse(args)
	if *in == "" {
		usage()
	}
	if *pageOut != "" {
		if _, err := os.Stat(*pageOut); err == nil && !*force {
			return fmt.Errorf("%q esiste gia' (usa -force)", *pageOut)
		}
	}
	return extractJSON(*in, *debug, *pageOut, *top, *above)
}

func cmdCrop(args []string) error {
	fs := flag.NewFlagSet("crop", flag.ExitOnError)
	in := fs.String("i", "", "immagine del foglio A4 gia' acquisita")
	out := fs.String("o", "", "file PNG di uscita (predefinito: <ingresso>-book.png)")
	margin := fs.Float64("margin", defaultMargin, "mm di carta attorno al libretto (solo con -canvas book)")
	canvasFlag(fs)
	force := fs.Bool("force", false, "sostituisci il file di uscita se esiste")
	_ = fs.Parse(args)
	if *in == "" {
		usage()
	}
	if *out == "" {
		*out = (*in)[:len(*in)-len(filepath.Ext(*in))] + "-book.png"
	}
	if _, err := os.Stat(*out); err == nil && !*force {
		return fmt.Errorf("%q esiste gia' (usa -force)", *out)
	}
	img, err := imgproc.Load(*in)
	if err != nil {
		return fmt.Errorf("lettura immagine: %w", err)
	}
	if err := saveBook(img, *out, *margin); err != nil {
		return err
	}
	fmt.Println("salvato", *out)
	return nil
}

// saveBook ritaglia il passaporto intero (orientamento e raddrizzamento automatici) e salva il PNG. Se non trova
// l'MRZ o il libretto non sta nel foglio, ripiega sul ritaglio generico al documento e lo dice su stderr.
func saveBook(img image.Image, out string, marginMM float64) error {
	frame, err := frameFromSpec(canvasSpec, marginMM)
	if err != nil {
		return err
	}
	book, rot, ok := imgproc.CropBookOrientedFrame(img, frame)
	if !ok {
		fmt.Fprintln(os.Stderr, "avviso: passaporto non riconosciuto (MRZ non trovata o libretto tagliato): ritaglio generico al documento")
		book, _ = imgproc.AutoCropInfo(img)
		book = imgproc.TrimWhite(book)
	} else if rot != 0 {
		fmt.Fprintf(os.Stderr, "foglio ruotato di %d gradi\n", rot)
	}
	fmt.Fprintln(os.Stderr, "attenzione:", out, "contiene dati personali")
	return imgproc.SavePNG(book, out)
}

// confirmScan chiede all'operatore di confermare che il foglio e' nel caricatore; senza terminale serve -yes.
func confirmScan(yes bool) error {
	if yes {
		return nil
	}
	st, err := os.Stdin.Stat()
	if err != nil || st.Mode()&os.ModeCharDevice == 0 {
		return errors.New("input non interattivo: per avviare lo scanner senza conferma usa -yes")
	}
	fmt.Fprint(os.Stderr, "Foglio A4 nel caricatore? Premi Invio per scansionare, Ctrl+C per annullare: ")
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		return fmt.Errorf("conferma non letta: %w", err)
	}
	return nil
}

// cmdPDF converte un'immagine in un PDF di una pagina entro un peso massimo (il gestionale accetta allegati fino a 1 MB:
// si usano 1.000.000 byte per stare sicuri). Stessi argomenti e stessa riga di esito di "libretto pdf" della v1.
func cmdPDF(args []string) error {
	fs := flag.NewFlagSet("pdf", flag.ExitOnError)
	in := fs.String("i", "", "immagine da convertire (PNG o JPEG)")
	out := fs.String("o", "", "PDF di uscita (predefinito: come l'immagine, con estensione .pdf)")
	maxB := fs.Int("max", 1_000_000, "peso massimo del PDF in byte (0 = nessun limite)")
	dpi := fs.Float64("dpi", 300, "risoluzione dell'immagine, decide la misura della pagina")
	force := fs.Bool("force", false, "sostituisci il PDF se esiste")
	_ = fs.Parse(args)
	if *in == "" {
		usage()
	}
	if *out == "" {
		*out = strings.TrimSuffix(*in, filepath.Ext(*in)) + ".pdf"
	}
	info, err := pdfout.WriteFile(*in, *out, *maxB, *dpi, *force)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "attenzione: %s contiene dati personali\n", *out)
	fmt.Printf("%s: %d KB (limite %d KB), qualita' JPEG %d, immagine al %.0f%% (%dx%d px)\n",
		filepath.Base(*out), (info.Bytes+512)/1024, *maxB/1000, info.Quality, info.Scale*100, info.W, info.H)
	return nil
}
