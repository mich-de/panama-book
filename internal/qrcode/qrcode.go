// Package qrcode legge i codici QR (versioni 1-10, la grandezza dei QR dei certificati di Panama) in Go puro, senza
// dipendenze: trova i tre quadrati di riferimento, campiona la griglia, legge il formato, toglie la maschera e corregge gli errori
// con Reed-Solomon. Serve per il controllo doppio dei certificati: il QR contiene il link al servizio di verifica dell'AMP.
//
// Non e' un lettore generale: niente QR capovolti (specchiati), niente prospettiva forte, solo i modi numerico, alfanumerico e byte.
// Per le immagini pulite (PDF, scansioni a 300 dpi) basta; se non legge restituisce ErrNotFound o ErrUnsupported e il programma
// continua senza il controllo doppio.
package qrcode

import (
	"errors"
	"image"
)

var (
	// ErrNotFound: nessun codice QR trovato (o illeggibile).
	ErrNotFound = errors.New("qr: codice non trovato")
	// ErrUnsupported: il codice c'e' ma e' di un tipo che non si legge qui (versione oltre la 10, modo kanji...).
	ErrUnsupported = errors.New("qr: tipo di codice non supportato")
)

// Decode cerca un codice QR nell'immagine e ne restituisce il testo. Prima guarda l'immagine intera; se non basta (foglio A4 con
// il QR piccolo su uno sfondo che cambia, come sul libretto sotto l'ologramma) la guarda a finestre sovrapposte, ognuna con la
// sua soglia di bianco e nero.
func Decode(img image.Image) (string, error) {
	g := toGray(img)
	if g == nil {
		return "", ErrNotFound
	}
	var firstErr error = ErrNotFound
	try := func(rc image.Rectangle) (string, bool) {
		bin := g.bitmap(rc)
		if bin == nil {
			return "", false
		}
		for _, t := range triples(findFinders(bin)) {
			vs := versions(t)
			if len(vs) == 0 {
				firstErr = ErrUnsupported
				continue
			}
			for _, v := range vs {
				m, err := sample(bin, t, v)
				if err != nil {
					continue
				}
				for _, mat := range [][][]bool{sampleAdaptive(bin, t, v), m} {
					txt, err := decodeMatrix(mat)
					if err == nil {
						return txt, true
					}
					if !errors.Is(err, ErrNotFound) {
						firstErr = err
					}
				}
			}
		}
		return "", false
	}
	if txt, ok := try(image.Rect(0, 0, g.w, g.h)); ok {
		return txt, nil
	}
	for _, win := range []int{360, 600} {
		if win >= g.w && win >= g.h {
			break
		}
		step := win / 2
		for y := 0; y < g.h; y += step {
			for x := 0; x < g.w; x += step {
				if txt, ok := try(image.Rect(x, y, x+win, y+win)); ok {
					return txt, nil
				}
			}
		}
	}
	return "", firstErr
}
