package twain

import (
	"fmt"
	"sort"
	"strings"
)

// ProbeOptions controlla la diagnostica. Probe NON avvia mai il motore dello scanner (nessun MSG_ENABLEDS).
type ProbeOptions struct {
	// TrySetFrame prova a scrivere ICAP_FRAMES con un riquadro che parte a 90 mm dal bordo d'ingresso, rilegge il
	// valore e poi ripristina il predefinito: dice se il driver accetta (e quindi forse rispetta) l'alto del riquadro.
	// E' solo scrittura di proprieta': nessuna acquisizione. Spento di default.
	TrySetFrame bool
	// TryAll continua con tutte le ricette anche dopo la prima accettata (di default si ferma alla prima).
	TryAll bool
	// Layout abilita anche DAT_IMAGELAYOUT (DG_IMAGE: lettura del layout e prova di scrittura). E' solo il
	// layout del riquadro, non un'acquisizione, ma e' un DG_IMAGE: spento di default.
	Layout bool
}

// frameCaps sono le capability da mostrare in dettaglio (GET, GETDEFAULT, GETCURRENT, QUERYSUPPORT).
var frameCaps = []uint16{
	icapUnits, icapFrames, icapMaxFrames, icapPhysicalWidth, icapPhysicalHeigh, icapSupportedSize, icapUndefImgSize,
	icapExtFrameMode, icapExtFramesIndUn, icapExtFrameUnit, icapExtTrimSize, icapExtTrimSizeUnit,
	icapExtOverscan, 0x1149 /* ICAP_OVERSCAN */, icapExtAdfPhysH, icapExtAdfPhysW, capExtFrame,
	capExtPaperProtect, capExtManualFeeder, capExtSyncEject, capFeederLoaded, capDuplexEnabled, icapXResolution,
}

func flagsText(f uint32) string {
	var p []string
	for _, x := range []struct {
		bit  uint32
		name string
	}{{1, "GET"}, {2, "SET"}, {4, "GETDEFAULT"}, {8, "GETCURRENT"}, {16, "RESET"}} {
		if f&x.bit != 0 {
			p = append(p, x.name)
		}
	}
	if len(p) == 0 {
		return "-"
	}
	return strings.Join(p, "+")
}

// probe raccoglie le righe di diagnostica: sola lettura (e, se richiesto, scrittura di ICAP_FRAMES).
func (s *session) probe(match string, po ProbeOptions, out *[]string) error {
	p := func(format string, a ...interface{}) { *out = append(*out, fmt.Sprintf(format, a...)) }
	if err := s.openDSM(); err != nil {
		return err
	}
	defer s.closeDSM()
	list, err := s.listDS()
	if err != nil {
		return err
	}
	p("data source TWAIN installati: %d", len(list))
	for _, id := range list {
		pm, pn := identityProtocol(id)
		p("  - %q famiglia %q produttore %q protocollo %d.%d", identityProduct(id), identityFamily(id), identityMaker(id), pm, pn)
	}
	if err := s.openDS(match); err != nil {
		return err
	}
	defer s.closeDS()
	p("aperto: %q", identityProduct(s.ds))

	// elenco delle capability supportate
	sup, err := s.getCap(capSupportedCaps, msgGet)
	if err != nil {
		p("CAP_SUPPORTEDCAPS non leggibile: %v", err)
	} else {
		ids := make([]int, 0, len(sup.Items))
		for _, it := range sup.Items {
			ids = append(ids, int(it.U))
		}
		sort.Ints(ids)
		p("capability supportate: %d", len(ids))
		for _, id := range ids {
			u := uint16(id)
			flags := "?"
			if q, err := s.getCap(u, msgQuerySupport); err == nil && len(q.Items) == 1 {
				flags = flagsText(q.Items[0].U)
			}
			cur := "(non leggibile)"
			if c, err := s.getCap(u, msgGetCurrent); err == nil {
				cur = c.String()
			}
			n := capNames[u]
			if n == "" {
				n = "?"
			}
			p("  0x%04X %-38s %-26s %s", u, n, flags, cur)
		}
	}

	// dettaglio delle capability che riguardano area, riquadro e alimentazione
	p("--- dettaglio area / riquadro / alimentazione ---")
	for _, id := range frameCaps {
		p("%s", capLabel(id))
		for _, m := range []struct {
			msg  uint16
			name string
		}{{msgQuerySupport, "supporto"}, {msgGet, "valori"}, {msgGetDefault, "predefinito"}, {msgGetCurrent, "corrente"}} {
			c, err := s.getCap(id, m.msg)
			switch {
			case err != nil:
				p("    %-11s n/d: %v", m.name, shortErr(err))
			case m.msg == msgQuerySupport && len(c.Items) == 1:
				p("    %-11s %s", m.name, flagsText(c.Items[0].U))
			default:
				p("    %-11s %s", m.name, c)
			}
		}
	}
	// riquadro dell'immagine (DAT_IMAGELAYOUT, sola lettura): solo con ProbeOptions.Layout
	if po.Layout {
		for _, m := range []struct {
			msg  uint16
			name string
		}{{msgGetDefault, "predefinito"}, {msgGet, "corrente"}} {
			b := make([]byte, imageLayoutSize)
			if _, err := s.call("lettura del layout dell'immagine", s.ds, dgImage, datImageLayout, m.msg, b); err != nil {
				p("DAT_IMAGELAYOUT %s n/d: %v", m.name, shortErr(err))
				continue
			}
			f := decodeFrame(b)
			p("DAT_IMAGELAYOUT %s: sinistra %.3f alto %.3f destra %.3f basso %.3f (pollici)", m.name, f.Left, f.Top, f.Right, f.Bottom)
		}
	}

	if po.TrySetFrame {
		s.tryFrames(po, p)
	}
	return nil
}

func shortErr(err error) string {
	m := err.Error()
	if len(m) > 120 {
		m = m[:120] + "..."
	}
	return m
}

// tryFrames prova le ricette di ICAP_FRAMES (vedi frames.go) con alto 90 mm e lunghezza 100 mm, stampando ogni
// esito; dopo ogni prova rilegge ICAP_FRAMES e ripristina il predefinito. Solo proprieta': nessuna acquisizione.
func (s *session) tryFrames(po ProbeOptions, p func(string, ...interface{})) {
	p("--- prova di scrittura di ICAP_FRAMES (solo proprieta', nessuna scansione) ---")
	dpi := 300.0
	if c, err := s.getCap(icapXResolution, msgGetCurrent); err == nil {
		if v, ok := c.Current(); ok && v > 0 {
			dpi = v
		}
	}
	first := ""
	s.captureFrameDefault()
	for i, r := range recipes(s.customSizes()) {
		if r.layoutSet && !po.Layout {
			p("prova %d: %s: saltata (serve -layout: usa DG_IMAGE)", i+1, r.name)
			continue
		}
		res := s.applyRecipe(r, 90, 0, 100, dpi, po.Layout)
		p("prova %d: %s", i+1, r.name)
		p("    richiesto: %s", fmtFrame(res.req, r.unit))
		if res.haveGot {
			p("    riletto:   %s", fmtFrame(res.got, r.unit))
		}
		if res.err != nil {
			p("    ESITO: NO: %s", shortErr(res.err))
		} else {
			p("    ESITO: SI: il driver ha accettato e riletto lo stesso riquadro con alto = 90 mm")
			if first == "" {
				first = r.name
			}
		}
		s.resetCaps(res.touched)
		if c, err := s.getCap(icapFrames, msgGetCurrent); err == nil {
			if f, ok := c.CurrentFrame(); ok {
				u := uint16(unitInch)
				if cu, err := s.getCap(icapUnits, msgGetCurrent); err == nil {
					if v, ok := cu.Current(); ok {
						u = uint16(v)
					}
				}
				p("    dopo il ripristino: %s", fmtFrame(f, u))
			}
		}
		if res.err == nil && !po.TryAll {
			break
		}
	}
	if first == "" {
		p("RISULTATO: nessuna ricetta ha fatto accettare l'alto del riquadro al driver")
	} else {
		p("RISULTATO: prima ricetta accettata: %s (accettato non vuol dire rispettato dal motore: serve una scansione di prova)", first)
	}
}
