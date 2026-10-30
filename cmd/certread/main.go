// Comando certread: legge un certificato di Panama (SAT, SD, SSO o CT) da un PDF o da un'immagine gia' acquisita e stampa il JSON.
//
//	certread file.pdf|file.png|file.jpg [altri file...]
//	certread -rows file.pdf         stampa le righe lette dall'OCR (per capire perche' un campo non viene letto)
//
// Un PDF viene reso in immagine (internal/pdfin) e letto con l'OCR di Windows come una scansione. Il JSON contiene dati personali:
// resta su disco, non va condiviso.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"scanlibretto/internal/certif"
	"scanlibretto/internal/ocr"
	"scanlibretto/internal/pdfin"
	"scanlibretto/internal/pdftext"
	"scanlibretto/internal/qrcode"
	"scanlibretto/internal/seafarer"
)

func main() {
	rows := flag.Bool("rows", false, "stampa le righe dell'OCR invece del JSON")
	amp := flag.Bool("amp", false, "ricerca sul sito AMP: certread -amp \"NOME COGNOME\" GG-MM-AAAA MARINEID")
	qr := flag.Bool("qr", false, "cerca solo il codice QR in un'immagine e stampa il suo testo")
	flag.Parse()
	if *amp {
		if flag.NArg() != 3 {
			fmt.Fprintln(os.Stderr, "uso: certread -amp \"NOME COGNOME\" GG-MM-AAAA MARINEID")
			os.Exit(2)
		}
		f, err := seafarer.SearchMarine(context.Background(), flag.Arg(0), flag.Arg(1), flag.Arg(2))
		if err != nil {
			fmt.Fprintln(os.Stderr, "errore:", err)
			os.Exit(1)
		}
		b, _ := json.MarshalIndent(f, "", "  ")
		fmt.Println(string(b))
		return
	}
	if *qr {
		for _, f := range flag.Args() {
			fh, err := os.Open(f)
			if err == nil {
				if im, _, err2 := image.Decode(fh); err2 == nil {
					u, e := qrcode.Decode(im)
					fmt.Printf("%s: %q %v\n", filepath.Base(f), u, e)
				}
				fh.Close()
			}
		}
		return
	}
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "uso: certread [-rows] file.pdf|file.png|file.jpg ...")
		os.Exit(2)
	}
	code := 0
	for _, f := range flag.Args() {
		if err := one(context.Background(), f, *rows); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", filepath.Base(f), err)
			code = 1
		}
	}
	os.Exit(code)
}

func one(ctx context.Context, path string, onlyRows bool) error {
	img := path
	if strings.EqualFold(filepath.Ext(path), ".pdf") {
		if data, err := os.ReadFile(path); err == nil {
			if rows, err := pdftext.Rows(data); err == nil { // PDF digitale: testo incorporato, niente OCR
				if imgs, err := pdftext.Images(data); err == nil {
					for _, im := range imgs {
						if u, err := qrcode.Decode(im); err == nil {
							fmt.Fprintln(os.Stderr, "QR:", u)
							break
						}
					}
				}
				return finish(rows, onlyRows)
			} else {
				fmt.Fprintln(os.Stderr, "testo del PDF non letto, si passa all'OCR:", err)
			}
		}
		tmp, err := os.CreateTemp("", "certread*.png")
		if err != nil {
			return err
		}
		tmp.Close()
		defer os.Remove(tmp.Name())
		if err := pdfin.RenderFirstPage(ctx, path, tmp.Name(), 0); err != nil {
			return err
		}
		img = tmp.Name()
	}
	res, err := ocr.Recognize(ctx, img)
	if err != nil {
		return err
	}
	return finish(certif.RowsFromOCR(res), onlyRows)
}

// finish stampa le righe oppure il JSON del certificato letto.
func finish(lines []string, onlyRows bool) error {
	if onlyRows {
		fmt.Println(strings.Join(lines, "\n"))
		return nil
	}
	c, err := certif.Parse(lines)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	fmt.Println(string(b))
	return nil
}
