package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/pdfout"
)

// maxPDFBytes: il gestionale accetta allegati fino a 1 MB. Si usa 1.000.000 byte (non 1.048.576) per stare sicuri
// anche se il gestionale conta in megabyte decimali.
const maxPDFBytes = 1_000_000

// cmdPDF converte una scansione in un PDF di una pagina entro un peso massimo, con la qualita' JPEG piu' alta possibile.
func cmdPDF(args []string) error {
	fs := flag.NewFlagSet("pdf", flag.ExitOnError)
	in := fs.String("i", "", "immagine da convertire (PNG o JPEG)")
	out := fs.String("o", "", "PDF di uscita (predefinito: come l'immagine, con estensione .pdf)")
	maxB := fs.Int("max", maxPDFBytes, "peso massimo del PDF in byte (0 = nessun limite)")
	dpi := fs.Float64("dpi", 300, "risoluzione dell'immagine, decide la misura della pagina")
	force := fs.Bool("force", false, "sostituisci il PDF se esiste")
	_ = fs.Parse(args)
	if *in == "" {
		usage()
	}
	if *out == "" {
		*out = strings.TrimSuffix(*in, filepath.Ext(*in)) + ".pdf"
	}
	if _, err := os.Stat(*out); err == nil && !*force {
		return fmt.Errorf("%q esiste gia' (usa -force)", *out)
	}
	img, err := imgproc.Load(*in)
	if err != nil {
		return fmt.Errorf("lettura immagine: %w", err)
	}
	pdf, info, err := pdfout.Encode(img, *dpi, *maxB)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, pdf, 0o600); err != nil {
		return errors.Join(errors.New("scrittura del PDF"), err)
	}
	fmt.Fprintf(os.Stderr, "attenzione: %s contiene dati personali\n", *out)
	fmt.Printf("%s: %d KB (limite %d KB), qualita' JPEG %d, immagine al %.0f%% (%dx%d px)\n",
		filepath.Base(*out), (info.Bytes+512)/1024, *maxB/1000, info.Quality, info.Scale*100, info.W, info.H)
	return nil
}
