// Comando twainprobe: diagnostica TWAIN dello scanner fi-800R (PaperStream IP). NON scansiona.
//
//	twainprobe32.exe            (GOARCH=386: usa il DSM e il driver a 32 bit, oggi gli unici installati)
//	twainprobe64.exe            (GOARCH=amd64: serve 'PaperStream IP (TWAIN x64)', altrimenti spiega cosa manca)
//
// Apre il DSM e il data source, stampa le capability (incluso ICAP_FRAMES: l'alto del riquadro e' supportato?)
// e chiude. Con -try-frame prova anche a SCRIVERE ICAP_FRAMES (alto a 90 mm) con varie configurazioni
// (formato carta, unita', capability proprie) e a rileggerlo, ripristinando il predefinito: sono solo proprieta',
// non parte nessuna scansione. -all prova tutte le ricette; -layout aggiunge DAT_IMAGELAYOUT (DG_IMAGE).
// Nessun documento va inserito.
package main

import (
	"flag"
	"fmt"
	"os"

	"scanlibretto/internal/twain"
)

func main() {
	dev := flag.String("device", "fi-800R", "parte del nome del data source")
	try := flag.Bool("try-frame", false, "prova a scrivere ICAP_FRAMES con alto 90 mm e a rileggerlo (nessuna scansione)")
	all := flag.Bool("all", false, "con -try-frame: prova tutte le ricette anche dopo la prima accettata")
	layout := flag.Bool("layout", false, "abilita DAT_IMAGELAYOUT (DG_IMAGE: lettura/scrittura del layout, nessuna acquisizione)")
	flag.Parse()

	fmt.Println(twain.ArchitectureHint())
	lines, err := twain.Probe(*dev, twain.ProbeOptions{TrySetFrame: *try, TryAll: *all, Layout: *layout})
	for _, l := range lines {
		fmt.Println(l)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		os.Exit(1)
	}
}
