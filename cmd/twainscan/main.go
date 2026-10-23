// Comando twainscan: PROVA di acquisizione TWAIN (PaperStream IP) con inizio e lunghezza dell'area. AVVIA LO SCANNER.
//
//	twainscan32.exe -from 90 -len 100 -dpi 300
//
// Serve a verificare con un documento vero se il motore rispetta l'alto del riquadro (ICAP_FRAMES), cosa che con
// WIA non succede. Scrive il PNG GREZZO (nessun ritaglio ne' lettura) in scans\twain-AAAAMMGG-hhmmss.png, senza mai
// sostituire un file esistente (salvo -force). Prima di avviare lo scanner chiede conferma, saltata solo da -yes (lo usa libretto).
// Una sola facciata, una sola pagina, nessun tentativo ripetuto. Costruire a 32 bit (GOARCH=386): e' installato
// solo il driver a 32 bit.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/twain"
)

func main() {
	from := flag.Float64("from", 0, "inizia a leggere dopo N mm dal bordo d'ingresso (0 = dall'inizio)")
	length := flag.Float64("len", 0, "lunghezza letta in mm a partire da -from (0 = fino al fondo del campo)")
	dpi := flag.Int("dpi", 300, "risoluzione")
	ignore := flag.Bool("ignora-sensore", false, "non rifiutare se il sensore dell'alimentatore dice 'vuoto' (libretto inserito dal percorso frontale)")
	width := flag.Float64("width", 0, "larghezza letta in mm, centrata (0 = tutto il campo)")
	gray := flag.Bool("gray", false, "scala di grigi")
	outPath := flag.String("o", "", "file PNG grezzo di uscita (predefinito: scans\\twain-AAAAMMGG-hhmmss.png)")
	yes := flag.Bool("yes", false, "non chiedere conferma (lo usa libretto, che l'ha gia' chiesta)")
	force := flag.Bool("force", false, "sostituisci il file di uscita se esiste")
	flag.Parse()

	opts := twain.Options{DPI: *dpi, Gray: *gray, StartMM: *from, HeightMM: *length, WidthMM: *width, IgnoreFeederLoaded: *ignore}
	fmt.Println(twain.ArchitectureHint())
	fmt.Printf("Area: inizio %.1f mm, lunghezza %.1f mm (0 = fino al fondo), %d dpi, una sola facciata.\n", *from, *length, *dpi)
	if !*yes {
		fmt.Print("Il documento e' gia' inserito? Digita SI per AVVIARE LO SCANNER: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil || strings.ToUpper(strings.TrimSpace(line)) != "SI" {
			fmt.Fprintln(os.Stderr, "annullato: lo scanner non e' stato avviato")
			os.Exit(2)
		}
	}

	out := *outPath
	if out == "" {
		out = filepath.Join("scans", "twain-"+time.Now().Format("20060102-150405")+".png")
	}
	if _, err := os.Stat(out); err == nil && !*force {
		fmt.Fprintln(os.Stderr, "errore: il file", out, "esiste gia' e non viene sovrascritto")
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		os.Exit(1)
	}
	img, info, err := twain.ScanInfo(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		switch {
		case twain.IsPaperEmpty(err):
			os.Exit(3)
		case twain.IsPaperJam(err):
			os.Exit(4)
		}
		os.Exit(1)
	}
	if err := imgproc.SavePNG(img, out); err != nil {
		fmt.Fprintln(os.Stderr, "errore nel salvataggio:", err)
		os.Exit(1)
	}
	b := img.Bounds()
	fmt.Printf("salvato %s: %dx%d px (%.1f x %.1f mm a %d dpi)", out, b.Dx(), b.Dy(),
		float64(b.Dx())/float64(*dpi)*25.4, float64(b.Dy())/float64(*dpi)*25.4, *dpi)
	if info.Recipe != "" {
		fmt.Printf("; area impostata con: %s", info.Recipe)
	}
	fmt.Println()
}
