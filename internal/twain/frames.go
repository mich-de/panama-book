package twain

import (
	"fmt"
	"math"
	"strings"
)

// Il data source PaperStream IP accetta un ICAP_FRAMES solo se rientra nel formato carta corrente
// (ICAP_SUPPORTEDSIZES; predefinito A4 = 8,268 x 11,693 pollici): un riquadro piu' largo viene rifiutato con
// TWCC_BADVALUE. Le "ricette" qui sotto sono sequenze di impostazioni di proprieta' (mai di acquisizione) che si
// provano in ordine finche' il driver accetta E rilegge lo stesso riquadro. Servono sia a Probe (che le prova e
// le stampa) sia a Scan (che usa la prima che funziona).

// capStep e' una capability da impostare (valore numerico) prima del riquadro.
type capStep struct {
	id uint16
	ty uint16
	v  float64
}

type fieldSource int

const (
	fieldFromFrame fieldSource = iota // campo = riquadro corrente dopo le impostazioni (A4 predefinito)
	fieldPhysical                     // campo = ICAP_PHYSICALWIDTH/HEIGHT (8,5 x 14 pollici)
)

// formati ICAP_SUPPORTEDSIZES (TWSS_*)
const (
	twssNone    = 0
	twssUSLegal = 4
	twssMaxSize = 53
)

const (
	unitCm     = 1
	unitPixels = 5
)

type recipe struct {
	name      string
	pre       []capStep
	unit      uint16      // ICAP_UNITS e unita' del riquadro
	field     fieldSource // da dove si legge il campo
	frameCap  uint16      // ICAP_FRAMES oppure ICAP_EXT_FRAMES_INDUNIT
	narrowMM  float64     // se > 0 forza questa larghezza (centrata), ignorando WidthMM
	layoutSet bool        // imposta DAT_IMAGELAYOUT (DG_IMAGE): solo con ProbeOptions.Layout
}

// recipes elenca le ricette nell'ordine di prova. customSizes sono i valori >= 32768 di ICAP_SUPPORTEDSIZES.
func recipes(customSizes []uint16) []recipe {
	r := []recipe{
		{name: "riquadro nel formato predefinito (A4), pollici", unit: unitInch, field: fieldFromFrame, frameCap: icapFrames},
		{name: "ICAP_SUPPORTEDSIZES=NONE (0), campo fisico, pollici", pre: []capStep{{icapSupportedSize, tyUint16, twssNone}}, unit: unitInch, field: fieldPhysical, frameCap: icapFrames},
		{name: "ICAP_SUPPORTEDSIZES=NONE (0), larghezza del formato predefinito", pre: []capStep{{icapSupportedSize, tyUint16, twssNone}}, unit: unitInch, field: fieldFromFrame, frameCap: icapFrames},
		{name: "ICAP_SUPPORTEDSIZES=MAXSIZE (53), campo fisico", pre: []capStep{{icapSupportedSize, tyUint16, twssMaxSize}}, unit: unitInch, field: fieldPhysical, frameCap: icapFrames},
		{name: "ICAP_SUPPORTEDSIZES=USLEGAL (4), campo fisico", pre: []capStep{{icapSupportedSize, tyUint16, twssUSLegal}}, unit: unitInch, field: fieldPhysical, frameCap: icapFrames},
		{name: "carta personalizzata 8,5x14 (ICAP_EXT_PAPERWIDTH/LENGTH)", pre: []capStep{{icapExtPaperWidth, tyFix32, 8.5}, {icapExtPaperLength, tyFix32, 14}}, unit: unitInch, field: fieldPhysical, frameCap: icapFrames},
	}
	for _, sz := range customSizes {
		r = append(r, recipe{
			name: fmt.Sprintf("formato personalizzato %d + carta 8,5x14", sz),
			pre:  []capStep{{icapSupportedSize, tyUint16, float64(sz)}, {icapExtPaperWidth, tyFix32, 8.5}, {icapExtPaperLength, tyFix32, 14}},
			unit: unitInch, field: fieldPhysical, frameCap: icapFrames,
		})
	}
	r = append(r,
		recipe{name: "unita' centimetri", unit: unitCm, field: fieldFromFrame, frameCap: icapFrames},
		recipe{name: "unita' pixel", unit: unitPixels, field: fieldFromFrame, frameCap: icapFrames},
		recipe{name: "ICAP_SUPPORTEDSIZES=NONE, unita' centimetri", pre: []capStep{{icapSupportedSize, tyUint16, twssNone}}, unit: unitCm, field: fieldPhysical, frameCap: icapFrames},
		recipe{name: "ICAP_EXT_FRAMES_INDUNIT (riquadro in unita' indipendenti)", pre: []capStep{{icapExtFrameUnit, tyUint16, unitInch}}, unit: unitInch, field: fieldFromFrame, frameCap: icapExtFramesIndUn},
		recipe{name: "ICAP_EXT_FRAMEMODE=1 + riquadro", pre: []capStep{{icapExtFrameMode, tyUint16, 1}}, unit: unitInch, field: fieldFromFrame, frameCap: icapFrames},
		recipe{name: "CAP_EXT_FRAME=1 + riquadro", pre: []capStep{{capExtFrame, tyUint16, 1}}, unit: unitInch, field: fieldFromFrame, frameCap: icapFrames},
		recipe{name: "riquadro piu' stretto (190 mm, centrato), formato predefinito", unit: unitInch, field: fieldFromFrame, frameCap: icapFrames, narrowMM: 190},
		recipe{name: "NONE + riquadro piu' stretto (190 mm)", pre: []capStep{{icapSupportedSize, tyUint16, twssNone}}, unit: unitInch, field: fieldFromFrame, frameCap: icapFrames, narrowMM: 190},
		recipe{name: "DAT_IMAGELAYOUT (DG_IMAGE), formato predefinito", unit: unitInch, field: fieldFromFrame, frameCap: icapFrames, layoutSet: true},
		recipe{name: "DAT_IMAGELAYOUT (DG_IMAGE) dopo NONE", pre: []capStep{{icapSupportedSize, tyUint16, twssNone}}, unit: unitInch, field: fieldPhysical, frameCap: icapFrames, layoutSet: true},
	)
	return r
}

const (
	icapExtPaperWidth  = 0x8002
	icapExtPaperLength = 0x8003
)

// unitScale: fattore per passare da pollici all'unita' del riquadro (i pixel dipendono dalla risoluzione).
func unitScale(unit uint16, dpi float64) float64 {
	switch unit {
	case unitCm:
		return mmPerInch / 10
	case unitPixels:
		return dpi
	}
	return 1
}

func frameTolerance(unit uint16) float64 {
	switch unit {
	case unitCm:
		return 0.01
	case unitPixels:
		return 1.5
	}
	return 0.003
}

func scaleFrame(f Frame, k float64) Frame {
	return Frame{f.Left * k, f.Top * k, f.Right * k, f.Bottom * k}
}

func roundFrame(f Frame) Frame {
	return Frame{math.Round(f.Left), math.Round(f.Top), math.Round(f.Right), math.Round(f.Bottom)}
}

func fmtFrame(f Frame, unit uint16) string {
	u := map[uint16]string{unitInch: "pollici", unitCm: "cm", unitPixels: "pixel"}[unit]
	return fmt.Sprintf("sinistra %.3f alto %.3f destra %.3f basso %.3f (%s)", f.Left, f.Top, f.Right, f.Bottom, u)
}

// recipeResult descrive l'esito di una ricetta.
type recipeResult struct {
	touched []uint16 // capability modificate (da ripristinare con resetCaps se la ricetta non va tenuta)
	req     Frame    // riquadro richiesto, nell'unita' della ricetta
	got     Frame    // riquadro riletto
	haveGot bool
	err     error
}

// applyRecipe imposta le capability della ricetta e poi il riquadro, e rilegge. Solo proprieta': nessuna
// acquisizione (salvo r.layoutSet, che usa DAT_IMAGELAYOUT ed e' consentita solo se allowLayout).
func (s *session) applyRecipe(r recipe, startMM, widthMM, heightMM, dpi float64, allowLayout bool) (res recipeResult) {
	touch := func(id uint16) { res.touched = append(res.touched, id) }
	if r.layoutSet && !allowLayout {
		res.err = fmt.Errorf("prova con DAT_IMAGELAYOUT (DG_IMAGE) non abilitata")
		return
	}
	for _, st := range r.pre {
		touch(st.id)
		if err := s.setVerify(st.id, st.ty, st.v); err != nil {
			res.err = err
			return
		}
	}
	touch(icapUnits)
	if err := s.setVerify(icapUnits, tyUint16, float64(r.unit)); err != nil {
		res.err = err
		return
	}
	scale := unitScale(r.unit, dpi)

	// campo in mm
	var fw, fh float64
	switch r.field {
	case fieldPhysical:
		fw, fh = s.fieldMM(scale)
	default:
		c, err := s.getCap(icapFrames, msgGetCurrent)
		if err != nil {
			res.err = err
			return
		}
		f, ok := c.CurrentFrame()
		if !ok {
			res.err = fmt.Errorf("ICAP_FRAMES non leggibile (%v)", c)
			return
		}
		fw, fh = unitsToMM(f.Right-f.Left, scale), unitsToMM(f.Bottom-f.Top, scale)
	}
	w := widthMM
	if r.narrowMM > 0 {
		w = r.narrowMM
	}
	inch, err := frameForArea(startMM, w, heightMM, fw, fh)
	if err != nil {
		res.err = err
		return
	}
	req := scaleFrame(inch, scale)
	if r.unit == unitPixels {
		req = roundFrame(req)
	}
	res.req = req

	touch(r.frameCap)
	if r.frameCap != icapFrames {
		touch(icapFrames)
	}
	if r.layoutSet {
		b := make([]byte, imageLayoutSize)
		copy(b, req.encode())
		if _, err := s.call("impostazione di DAT_IMAGELAYOUT", s.ds, dgImage, datImageLayout, msgSet, b); err != nil {
			res.err = err
			return
		}
	} else if err := s.putCap(r.frameCap, tyFrame, 0, &req); err != nil {
		res.err = err
		return
	}
	// rilettura: sempre da ICAP_FRAMES (e' quello che il motore usa); per la variante EXT anche il suo valore
	c, err := s.getCap(icapFrames, msgGetCurrent)
	if err != nil {
		res.err = err
		return
	}
	got, ok := c.CurrentFrame()
	if !ok {
		res.err = fmt.Errorf("ICAP_FRAMES non leggibile dopo l'impostazione (%v)", c)
		return
	}
	// ICAP_FRAMES e' riletto nell'unita' corrente di ICAP_UNITS, uguale a quella della ricetta
	res.got, res.haveGot = got, true
	if !framesClose(req, got, frameTolerance(r.unit)) {
		res.err = fmt.Errorf("riletto un riquadro diverso da quello richiesto")
	}
	return
}

// captureFrameDefault memorizza il riquadro predefinito (GETDEFAULT) per poterlo ripristinare: MSG_RESET di
// ICAP_FRAMES, sul driver reale, lascia il riquadro appena impostato.
func (s *session) captureFrameDefault() {
	if s.frameDef != nil {
		return
	}
	if c, err := s.getCap(icapFrames, msgGetDefault); err == nil {
		if f, ok := c.CurrentFrame(); ok {
			s.frameDef = &f
		}
	}
}

// resetCaps riporta al predefinito le capability toccate (MSG_RESET), in ordine inverso; gli errori si ignorano.
// Se ICAP_FRAMES resta diverso dal predefinito lo reimposta esplicitamente.
func (s *session) resetCaps(ids []uint16) {
	defer s.restoreFrameDefault()
	for i := len(ids) - 1; i >= 0; i-- {
		buf := s.l.encodeCapability(ids[i], conDontCare, 0)
		if _, err := s.call("ripristino di "+capLabel(ids[i]), s.ds, dgControl, datCapability, msgReset, buf); err == nil {
			if _, _, h := s.l.decodeCapability(buf); h != 0 {
				s.t.Free(h)
			}
		}
	}
}

// customSizes legge i valori >= 32768 (formati propri del driver) di ICAP_SUPPORTEDSIZES.
func (s *session) customSizes() []uint16 {
	c, err := s.getCap(icapSupportedSize, msgGet)
	if err != nil {
		return nil
	}
	var out []uint16
	for _, it := range c.Items {
		if it.U >= 32768 && it.U < 65536 {
			out = append(out, uint16(it.U))
		}
	}
	return out
}

// setAreaFrame prova le ricette in ordine e TIENE la prima che il driver accetta e rilegge identica
// (senza ripristinare). Se nessuna funziona restituisce un errore con l'esito di ognuna, prima di qualunque
// acquisizione. La ricetta con DAT_IMAGELAYOUT non si usa in Scan.
func (s *session) setAreaFrame(o Options) error {
	var fails []string
	s.captureFrameDefault()
	for _, r := range recipes(s.customSizes()) {
		if r.layoutSet {
			continue
		}
		res := s.applyRecipe(r, o.StartMM, o.WidthMM, o.HeightMM, float64(o.DPI), false)
		if res.err == nil {
			s.recipeUsed = r.name
			return nil
		}
		s.resetCaps(res.touched)
		fails = append(fails, fmt.Sprintf("%s: %s", r.name, shortErr(res.err)))
	}
	return fmt.Errorf("twain: il driver non ha accettato nessun riquadro con l'area richiesta (alto %.1f mm, lunghezza %.1f mm, larghezza %.1f mm); provato: %s",
		o.StartMM, o.HeightMM, o.WidthMM, strings.Join(fails, " | "))
}

func (s *session) restoreFrameDefault() {
	if s.frameDef == nil {
		return
	}
	c, err := s.getCap(icapFrames, msgGetCurrent)
	if err != nil {
		return
	}
	if cur, ok := c.CurrentFrame(); ok && !framesClose(cur, *s.frameDef, 0.003) {
		_ = s.putCap(icapFrames, tyFrame, 0, s.frameDef)
	}
}
