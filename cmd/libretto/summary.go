package main

import (
	"fmt"
	"os"
	"path/filepath"

	"scanlibretto/internal/outsum"
)

// cmdSummary riassume in una riga un JSON prodotto da extract o read, senza stampare dati personali.
// L'elenco dei 14 campi attesi e dei 5 check digit e' in internal/outsum (condiviso con cmd/regress).
func cmdSummary(args []string) error {
	if len(args) != 1 {
		usage()
	}
	b, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	s, err := outsum.Parse(b)
	if err != nil {
		return fmt.Errorf("JSON non valido: %w", err)
	}
	mrz := "NO"
	if s.HasMRZ {
		mrz = "si"
	}
	fmt.Printf("%-40s rot=%d MRZ=%s check=%d/%d campi=%d/%d avvisi=%d\n", filepath.Base(args[0]), s.Rotation, mrz,
		s.ChecksOK(), outsum.NChecks, s.Found(), outsum.NFields, s.Warnings)
	if missing := s.Missing(); len(missing) > 0 {
		fmt.Println("    mancanti:", missing)
	}
	if !s.HasMRZ {
		fmt.Println("    MRZ non trovata: controlla che il libretto sia entrato dritto e centrato, con il bordo MRZ per primo, e che l'immagine mostri le due righe di <<<<.")
	}
	return nil
}
