package pdfout

import (
	"errors"
	"fmt"
	"os"

	"scanlibretto/internal/imgproc"
)

// WriteFile legge l'immagine in (PNG o JPEG), ne fa un PDF di una pagina entro maxBytes (0 = nessun limite) e lo scrive in out.
// Con force=false rifiuta un out esistente prima di fare altro. E' il corpo del comando "pdf" di libretto2.
func WriteFile(in, out string, maxBytes int, dpi float64, force bool) (Info, error) {
	if _, err := os.Stat(out); err == nil && !force {
		return Info{}, fmt.Errorf("%q esiste gia' (usa -force)", out)
	}
	img, err := imgproc.Load(in)
	if err != nil {
		return Info{}, fmt.Errorf("lettura immagine: %w", err)
	}
	pdf, info, err := Encode(img, dpi, maxBytes)
	if err != nil {
		return Info{}, err
	}
	if err := os.WriteFile(out, pdf, 0o600); err != nil {
		return Info{}, errors.Join(errors.New("scrittura del PDF"), err)
	}
	return info, nil
}
