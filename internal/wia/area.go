package wia

import "fmt"

// areaProps e' la parte minima dell'item WIA che serve a limitare l'area: permette di provare
// l'ordine delle scritture con un driver finto nei test (nessuna acquisizione, nessun COM).
type areaProps interface {
	Get(name string) (int, error) // valore corrente
	Set(name string, v int) error // scrittura
	Range(name string) (lo, hi int)
}

// applyArea restringe l'area di scansione (origine in alto, larghezza centrata).
//
// Comportamento del driver (misurato): ogni modifica di uno start fa RICALCOLARE al driver il
// corrispondente extent (valore imprevedibile) e il massimo dell'extent diventa (campo - start);
// un extent impostato DOPO lo start viene accettato. Quindi, per ogni asse, l'ordine e' sempre:
// start prima, poi lettura del nuovo massimo dell'extent, poi extent; infine si rileggono tutti i valori.
// Va chiamata dopo aver impostato la risoluzione (il driver ricalcola i limiti). Ogni scostamento tra
// valore richiesto e letto annulla la scansione, prima di qualunque acquisizione.
func applyArea(p areaProps, opts Options) error {
	vertical := opts.HeightMM > 0 || opts.StartMM > 0
	horizontal := opts.WidthMM > 0

	check := func(name string, want int) error {
		got, err := p.Get(name)
		if err != nil || got != want {
			return fmt.Errorf("area di scansione: il driver non ha accettato %s=%d (letto %d): scansione annullata", name, want, got)
		}
		return nil
	}
	set := func(name string, want int) error {
		if err := p.Set(name, want); err != nil {
			return fmt.Errorf("area di scansione: %s: %w", name, err)
		}
		return check(name, want)
	}
	setStart := func(name string, want int) error {
		if lo, hi := p.Range(name); hi > 0 && (want < lo || want > hi) {
			return fmt.Errorf("area di scansione: %s=%d fuori dall'intervallo del driver (%d-%d): scansione annullata", name, want, lo, hi)
		}
		return set(name, want)
	}

	// start verticale a 0 sempre per primo (anche senza offset: altera l'extent), poi si legge il massimo
	if vertical {
		if err := set("Vertical Start Position", 0); err != nil {
			return err
		}
	}
	_, maxW := p.Range("Horizontal Extent")
	_, maxH := p.Range("Vertical Extent")
	startX, startY, extX, extY, err := areaPixels(opts.StartMM, opts.WidthMM, opts.HeightMM, opts.DPI, maxW, maxH)
	if err != nil {
		return err
	}

	if vertical {
		if startY > 0 {
			if err := setStart("Vertical Start Position", startY); err != nil {
				return err
			}
		}
		// massimo riletto DOPO lo start: il driver lo porta a (campo - start)
		if _, hi := p.Range("Vertical Extent"); hi > 0 && extY > hi {
			extY = hi
		}
		if err := set("Vertical Extent", extY); err != nil {
			return err
		}
	}
	if horizontal {
		if err := setStart("Horizontal Start Position", startX); err != nil {
			return err
		}
		if _, hi := p.Range("Horizontal Extent"); hi > 0 && extX > hi {
			return fmt.Errorf("area di scansione: con Horizontal Start Position=%d la larghezza massima e' %d px, richiesti %d: scansione annullata", startX, hi, extX)
		}
		if err := set("Horizontal Extent", extX); err != nil {
			return err
		}
	}

	// riletture finali di tutto
	if vertical {
		if err := check("Vertical Start Position", startY); err != nil {
			return err
		}
		if err := check("Vertical Extent", extY); err != nil {
			return err
		}
	}
	if horizontal {
		if err := check("Horizontal Start Position", startX); err != nil {
			return err
		}
		if err := check("Horizontal Extent", extX); err != nil {
			return err
		}
	}
	return nil
}
