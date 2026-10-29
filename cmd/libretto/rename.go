package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"scanlibretto/internal/filename"
)

// renameExts sono le estensioni dei file di una scansione che viaggiano insieme con lo stesso nome.
var renameExts = []string{".png", ".json", ".pdf", ".err"}

// cmdRename rinomina i file di una scansione (immagine, JSON, PDF, log) in "NOME - COGNOME - NUMERO LIBRETTO",
// usando i dati letti dall'MRZ. Si rinomina solo se l'MRZ e' valida (tutti i check digit) e la riga dei nomi non e'
// incerta: altrimenti i file restano col nome dato dall'operatore. Il nome contiene solo lettere, cifre e spazi.
func cmdRename(args []string) error {
	fs := flag.NewFlagSet("rename", flag.ExitOnError)
	js := fs.String("json", "", "JSON prodotto da extract (i file con lo stesso nome base vengono rinominati)")
	dry := fs.Bool("n", false, "non rinominare: stampa solo il nuovo nome")
	_ = fs.Parse(args)
	if *js == "" {
		usage()
	}
	b, err := os.ReadFile(*js)
	if err != nil {
		return err
	}
	var o output
	if err := json.Unmarshal(b, &o); err != nil {
		return fmt.Errorf("JSON non valido: %w", err)
	}
	base, why := renameBase(o)
	if base == "" {
		fmt.Fprintln(os.Stderr, "nessuna rinomina:", why)
		return nil
	}
	dir := filepath.Dir(*js)
	old := strings.TrimSuffix(filepath.Base(*js), filepath.Ext(*js))
	if old == base {
		fmt.Println(base)
		return nil
	}
	final := filename.Unique(dir, base, renameExts...)
	if *dry {
		fmt.Println(final)
		return nil
	}
	if err := renameAll(dir, old, final); err != nil {
		return err
	}
	fmt.Println(final)
	return nil
}

// renameBase calcola il nome base dai dati del JSON; se non si puo' (o non e' sicuro) restituisce "" e il motivo.
func renameBase(o output) (string, string) {
	if o.MRZ == nil {
		return "", "MRZ non trovata"
	}
	if !o.MRZ.Checks.AllOK() {
		return "", "i check digit dell'MRZ non sono tutti validi"
	}
	for _, w := range o.Warnings {
		if w.Code == "MRZ_LINE1_UNCERTAIN" {
			return "", "la riga dei nomi dell'MRZ e' incerta"
		}
	}
	book := o.Fields["seamans_book_mrz"]
	if book == "" {
		book = o.Fields["seamans_book"]
	}
	base, ok := filename.Base(filename.Parts{Given: o.Fields["given_names"], Surname: o.Fields["surname"], Book: book})
	if !ok {
		return "", "mancano nome, cognome o numero libretto"
	}
	return base, ""
}

// renameAll rinomina old+ext in final+ext per ogni estensione presente; se un passo fallisce rimette a posto i precedenti.
func renameAll(dir, old, final string) error {
	var done []string
	for _, e := range renameExts {
		from := filepath.Join(dir, old+e)
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if err := os.Rename(from, filepath.Join(dir, final+e)); err != nil {
			for _, d := range done {
				_ = os.Rename(filepath.Join(dir, final+d), filepath.Join(dir, old+d))
			}
			return errors.Join(fmt.Errorf("rinomina di %s", old+e), err)
		}
		done = append(done, e)
	}
	return nil
}
