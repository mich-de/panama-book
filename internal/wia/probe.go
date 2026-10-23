//go:build windows

package wia

import (
	"fmt"
	"strings"
	"time"

	ole "github.com/go-ole/go-ole"
)

// ProbeArea e' una diagnostica: scrive e rilegge SOLO proprieta' dell'item (risoluzione, start ed extent),
// senza Transfer e senza acquisire. Serve a capire come il driver lega start ed extent verticali.
// Restituisce le righe da stampare. Alla fine ripristina start=0 ed extent=massimo.
// Se il dispositivo e' occupato riprova (3 tentativi, attesa 3 s).
func ProbeArea(dpi, startPx, extentPx int) ([]string, error) {
	if dpi < minSaneDPI || dpi > maxSaneDPI {
		return nil, fmt.Errorf("risoluzione %d dpi non plausibile", dpi)
	}
	if startPx < 0 || extentPx <= 0 {
		return nil, fmt.Errorf("start %d e extent %d px non validi", startPx, extentPx)
	}
	var out []string
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		out = nil
		err = runWithTimeout(defaultTimeout, func() error {
			return withDevice(defaultDevice, func(dev, item *ole.IDispatch) error {
				out = probeArea(dev, item, dpi, startPx, extentPx)
				return nil
			})
		})
		if err == nil || !isRetryable(err) {
			break
		}
		time.Sleep(3 * time.Second)
	}
	return out, err
}

func probeArea(dev, item *ole.IDispatch, dpi, startPx, extentPx int) []string {
	var out []string
	names := []string{"Vertical Start Position", "Vertical Extent", "Horizontal Start Position", "Horizontal Extent"}
	snap := func(label string, setErr error) {
		var parts []string
		for _, n := range names {
			lo, hi := subRange(item, n)
			parts = append(parts, fmt.Sprintf("%s=%v [%d..%d]", n, getProp(item, n), lo, hi))
		}
		line := label + ": " + strings.Join(parts, "; ")
		if setErr != nil {
			line += "  ERRORE: " + setErr.Error()
		}
		out = append(out, line)
	}
	put := func(label, name string, v int) {
		snap(fmt.Sprintf("%s (%s=%d)", label, name, v), setProp(item, name, v))
	}
	maxOf := func(n string) int { _, hi := subRange(item, n); return hi }

	// come scanItem: stessa selezione dell'alimentatore, cosi' i limiti sono quelli della scansione reale
	_ = setProp(dev, "Document Handling Select", dpsDocumentHandlingFeeder)
	_ = setProp(dev, "Pages", 1)

	// proprieta' dell'item che potrebbero legare start ed extent (solo lettura)
	for _, p := range listProps(item) {
		l := strings.ToLower(p.Name)
		if strings.Contains(l, "size") || strings.Contains(l, "page") || strings.Contains(l, "sheet") ||
			strings.Contains(l, "bed") || strings.Contains(l, "extent") || strings.Contains(l, "start") {
			rng := ""
			if p.Min != "" {
				rng = fmt.Sprintf(" [%s..%s]", p.Min, p.Max)
			}
			if p.ReadOnly {
				rng += " (sola lettura)"
			}
			out = append(out, fmt.Sprintf("prop %s=%s%s", p.Name, p.Value, rng))
		}
	}

	// (A) risoluzione
	errA := setProp(item, "Horizontal Resolution", dpi)
	if errA == nil {
		errA = setProp(item, "Vertical Resolution", dpi)
	}
	snap(fmt.Sprintf("A risoluzione %d dpi", dpi), errA)
	// (B) start=0, extent
	put("B", "Vertical Start Position", 0)
	put("B", "Vertical Extent", extentPx)
	// (C) poi start
	put("C", "Vertical Start Position", startPx)
	// (D) poi extent di nuovo
	put("D", "Vertical Extent", extentPx)
	// (E) extent ridotto con lo stesso start
	put("E", "Vertical Extent", 900)
	// (F) ordine inverso da situazione pulita: start 0, extent massimo, poi start, poi extent
	put("F0 pulizia", "Vertical Start Position", 0)
	put("F0 pulizia", "Vertical Extent", maxOf("Vertical Extent"))
	put("F1", "Vertical Start Position", startPx)
	put("F2", "Vertical Extent", extentPx)
	// (G) ripristino
	put("G", "Vertical Start Position", 0)
	put("G", "Vertical Extent", maxOf("Vertical Extent"))
	return out
}
