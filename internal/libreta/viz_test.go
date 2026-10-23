package libreta

// Test unitari della lettura dei campi stampati (VIZ): nessun OCR, solo ocr.Result sintetici con riquadri
// costruiti a mano (etichette storpiate, valori sotto, colonne vicine) e immagini sintetiche per la geometria.

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"scanlibretto/internal/mrz"
	"scanlibretto/internal/ocr"
)

// ---- utilita' ----------------------------------------------------------------------------------------------

// vizWordSpec e' una parola sintetica: testo, angolo in alto a sinistra e altezza (la larghezza segue il testo).
type vizWordSpec struct {
	text string
	x, y float64
	h    float64
}

func vizResult(w, h int, specs []vizWordSpec) *ocr.Result {
	r := &ocr.Result{Width: w, Height: h}
	for _, s := range specs {
		bw := float64(len([]rune(s.text))) * s.h * 0.6
		r.Lines = append(r.Lines, ocr.Line{Text: s.text, Words: []ocr.Word{{Text: s.text, X: s.x, Y: s.y, W: bw, H: s.h}}})
	}
	return r
}

func vizRun(res *ocr.Result) *Result {
	out := &Result{Fields: map[string]string{}}
	extractVIZ(res, out)
	return out
}

func vizWant(t *testing.T, out *Result, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if out.Fields[k] != v {
			t.Errorf("campo %s = %q, atteso %q", k, out.Fields[k], v)
		}
	}
}

// ---- parser dei valori -------------------------------------------------------------------------------------

func TestVIZDates(t *testing.T) {
	cases := []struct {
		in, key, want string
	}{
		{"15-02-1990", "data_nascita", "15-02-1990"},
		{"• 24-11-2025", "data_scadenza", "24-11-2025"},
		{"15-02-1S90", "data_nascita", "15-02-1990"},   // S letta al posto di 9: 1590 non e' plausibile
		{"O9-O3-2O26", "data_emissione", "09-03-2026"}, // lettere O al posto di zeri
		{"09/03/2026", "data_emissione", "09-03-2026"},
		{"09032026", "data_emissione", "09-03-2026"}, // trattini persi
		{"31-02-2026", "data_emissione", ""},         // giorno inesistente
		{"0242-2020", "data_emissione", ""},          // trattino fuso con una cifra: mese 42
		{"13-03-1850", "data_nascita", ""},           // anno fuori intervallo
	}
	for _, c := range cases {
		d, ok := vizPickDate(vizDates(c.in), c.key)
		got := ""
		if ok {
			got = d.printed()
		}
		if got != c.want {
			t.Errorf("vizDates(%q) = %q, atteso %q", c.in, got, c.want)
		}
	}
	if d, _ := vizPickDate(vizDates("04-03-2031"), "data_scadenza"); d.iso() != "2031-03-04" {
		t.Errorf("ISO = %q", d.iso())
	}
}

func TestVIZDateSplitOnWords(t *testing.T) {
	ws := []word{{"09-03", box{10, 10, 60, 20}}, {"2026", box{80, 10, 40, 20}}}
	d, ok := vizFindDate(ws, "data_emissione")
	if !ok || d.printed() != "09-03-2026" {
		t.Errorf("data spezzata su due parole: %v %v", d, ok)
	}
}

func TestVIZNumberFormats(t *testing.T) {
	if v, ok := vizLibretto("PA0123401"); !ok || v != "PA0123401" {
		t.Errorf("libretto 2026: %q %v", v, ok)
	}
	if v, ok := vizLibretto("P0123456"); !ok || v != "P0123456" {
		t.Errorf("libretto 2020: %q %v", v, ok)
	}
	if v, ok := vizLibretto("PAO123AO1"); !ok || v != "PA0123401" { // O e A al posto di 0 e 4
		t.Errorf("libretto con cifre storpiate: %q %v", v, ok)
	}
	if _, ok := vizLibretto("Libreta"); ok {
		t.Error("una parola non e' un numero libretto")
	}
	if v, ok := vizNum9("500321654"); !ok || v != "500321654" {
		t.Errorf("num9: %q %v", v, ok)
	}
	if _, ok := vizNum9("50032165"); ok {
		t.Error("8 cifre non sono un numero a 9 cifre")
	}
	if _, ok := vizNum9("PANAMANIAN"); ok {
		t.Error("una parola non e' un numero a 9 cifre")
	}
	for in, want := range map[string]string{"YB1234567": "YB1234567", "C01234567": "C01234567", "4-123-4567": "4-123-4567"} {
		if v, ok := vizIdent(in); !ok || v != want {
			t.Errorf("identificazione %q -> %q %v", in, v, ok)
		}
	}
	for in, want := range map[string]string{"SUPPORT": "SUPPORT", "SUPP0RT": "SUPPORT", "OPERATONAL": "OPERATIONAL", "MANAGEMENT": "MANAGEMENT"} {
		if v, ok := vizLevel(in); !ok || v != want {
			t.Errorf("livello %q -> %q %v", in, v, ok)
		}
	}
	if _, ok := vizLevel("ORDINARY"); ok {
		t.Error("ORDINARY non e' un livello")
	}
}

func TestVIZLabelSimilarity(t *testing.T) {
	cases := []struct {
		text, key string
		min       float64
	}{
		{"Nacionalidad/ Nationality", "nacionalidad", 0.95},
		{"Noeionaeuad", "nacionalidad", 0.45}, // storpiata dall'OCR
		{"Re$orwV R*nal", "regionale", 0.45},  // idem
		{"Fecha de emisión/ Date of issue", "data_emissione", 0.95},
		{"Id Seatarets id", "id_marino", 0.45},
	}
	for _, c := range cases {
		if s := vizLabelSim(c.text, c.key); s < c.min {
			t.Errorf("somiglianza(%q, %s) = %.2f, attesa >= %.2f", c.text, c.key, s, c.min)
		}
	}
	// un valore non deve sembrare un'etichetta
	for _, v := range []string{"PANAMANIAN", "ORDINARY SEAMAN", "PANAMA DR", "SANTIAGO", "NAPOLES"} {
		if vizIsLabelish(v) {
			t.Errorf("%q scambiato per etichetta", v)
		}
	}
	if !vizIsLabelish("Nacionalidad/ Nationality") {
		t.Error("etichetta bilingue non riconosciuta")
	}
}

// ---- modo pagina intera ------------------------------------------------------------------------------------

// vizPaginaB riproduce la disposizione del facsimile 2026 B (coordinate dell'OCR a 3 pixel per pixel originale):
// colonna centrale a x=950, colonna destra a x=1780, etichette storpiate, rumore dell'ologramma al centro.
func vizPaginaB() []vizWordSpec {
	return []vizWordSpec{
		{"Libreta", 1608, 2031, 26}, {"PA0123401", 1805, 2059, 28},
		{"ROBERT", 950, 2124, 30}, {"ANTONIO", 1120, 2124, 30},
		{"NoeionaEUad", 950, 2155, 20}, {"PANAMANIAN", 950, 2182, 30},
		{"Identificacion/", 1740, 2155, 20}, {"8-123-45", 1841, 2183, 28},
		{"PANAMA", 1180, 2244, 30}, // filigrana al centro, non e' un valore
		{"09-03-2026", 950, 2302, 28}, {"01-01-2000", 1810, 2303, 28},
		{"Fedw", 950, 2333, 20}, {"Re$orwV", 1778, 2334, 20}, {"R*nal", 1900, 2334, 20},
		{"04-03-2031", 950, 2360, 28}, {"PANAMA", 1780, 2362, 30}, {"DR", 1930, 2362, 30},
		{"Nivel/Level", 950, 2390, 18}, {"SUPPORT", 954, 2414, 30},
		{"Ca•ooi", 954, 2470, 18}, {"Capactty", 1060, 2470, 18},
		{"ORDINARY", 954, 2496, 28}, {"SEAMAN", 1130, 2496, 28},
		{"Id", 958, 2595, 18}, {"Id", 1000, 2595, 18}, {"510123456", 958, 2625, 28},
		{"MARIiE", 1337, 2403, 28}, // altro rumore dell'ologramma
	}
}

func TestVIZExtractPageSynthetic(t *testing.T) {
	out := vizRun(vizResult(2038, 3026, vizPaginaB()))
	vizWant(t, out, map[string]string{
		"numero_libretto":    "PA0123401",
		"identificazione":    "8-123-45",
		"data_emissione":     "09-03-2026",
		"data_scadenza":      "04-03-2031",
		"data_nascita":       "01-01-2000",
		"data_emissione_iso": "2026-03-09",
		"data_scadenza_iso":  "2031-03-04",
		"data_nascita_iso":   "2000-01-01",
		"livello":            "SUPPORT",
		"cargo":              "ORDINARY SEAMAN",
		"id_marino":          "510123456",
		"regionale":          "PANAMA DR",
		"nacionalidad":       "PANAMANIAN",
	})
	if out.Fields["refrendo"] != "" {
		t.Errorf("il refrendo non c'e' in questa pagina, letto %q", out.Fields["refrendo"])
	}
}

func TestVIZExtractPageTwoNineDigitNumbers(t *testing.T) {
	specs := vizPaginaB()
	// modello 2026 con refrendo: secondo numero a 9 cifre nella colonna destra, sotto l'identificazione
	specs = append(specs, vizWordSpec{"207123456", 1790, 2250, 28})
	out := vizRun(vizResult(2038, 3026, specs))
	vizWant(t, out, map[string]string{"refrendo": "207123456", "id_marino": "510123456"})
}

func TestVIZDatesChronologicalOrder(t *testing.T) {
	// etichette illeggibili e disposte a caso: l'ordine cronologico decide nascita < emissione < scadenza
	specs := []vizWordSpec{
		{"04-03-2031", 100, 100, 28}, {"01-01-2000", 400, 200, 28}, {"09-03-2026", 100, 300, 28},
	}
	out := vizRun(vizResult(1000, 1000, specs))
	vizWant(t, out, map[string]string{
		"data_nascita_iso": "2000-01-01", "data_emissione_iso": "2026-03-09", "data_scadenza_iso": "2031-03-04",
	})
}

func TestVIZShortValidity(t *testing.T) {
	// modello 2026: la validita' puo' essere di pochi mesi, non si assumono 5 anni
	specs := []vizWordSpec{
		{"16-09-2026", 100, 100, 28}, {"17-02-2027", 100, 200, 28}, {"01-01-2000", 400, 150, 28},
	}
	out := vizRun(vizResult(1000, 1000, specs))
	vizWant(t, out, map[string]string{
		"data_nascita_iso": "2000-01-01", "data_emissione_iso": "2026-09-16", "data_scadenza_iso": "2027-02-17",
	})
}

func TestVIZEmptyAndNoise(t *testing.T) {
	out := vizRun(&ocr.Result{Width: 500, Height: 500})
	if len(out.Fields) != 0 {
		t.Errorf("immagine vuota: campi %v", out.Fields)
	}
	noise := vizResult(500, 500, []vizWordSpec{{"%%%", 10, 10, 20}, {"|", 50, 10, 20}, {"1", 80, 10, 20}})
	_ = vizRun(noise) // non deve andare in crash
}

// ---- modo foglio -------------------------------------------------------------------------------------------

// vizBandIndex restituisce l'indice della fascia del campo.
func vizBandIndex(key string) int {
	for i, b := range vizBands {
		if b.key == key {
			return i
		}
	}
	return -1
}

// vizSheetWord colloca una parola nella fascia (campo, variante) del foglio, alla posizione attesa del valore.
func vizSheetWord(key string, variant int, text string) vizWordSpec {
	i := vizBandIndex(key)
	b := vizBands[i]
	v := vizVariants[variant]
	up, _ := vizWindow(b, v)
	y := float64((variant*len(vizBands)+i)*vizBandH+vizBandPad) + up*v.unit
	return vizWordSpec{text, 40, y - 14, 28}
}

func vizSheetResult(specs []vizWordSpec) *ocr.Result {
	return vizResult(vizSheetW, vizSheetH(), specs)
}

func TestVIZSheetVotingMajority(t *testing.T) {
	var specs []vizWordSpec
	// la variante 0 sbaglia una cifra, le altre due concordano
	specs = append(specs, vizSheetWord("data_scadenza", 0, "04-03-2037"))
	specs = append(specs, vizSheetWord("data_scadenza", 1, "04-03-2031"))
	specs = append(specs, vizSheetWord("data_scadenza", 2, "04-03-2031"))
	specs = append(specs, vizSheetWord("id_marino", 0, "510123456"))
	specs = append(specs, vizSheetWord("livello", 1, "SUPP0RT"))
	specs = append(specs, vizSheetWord("regionale", 2, "PANAMA DR"))
	specs = append(specs, vizSheetWord("nacionalidad", 0, "TALIAN")) // storpiata: vale per ITALIAN
	out := vizRun(vizSheetResult(specs))
	vizWant(t, out, map[string]string{
		"data_scadenza": "04-03-2031", "data_scadenza_iso": "2031-03-04",
		"id_marino": "510123456", "livello": "SUPPORT", "regionale": "PANAMA DR", "nacionalidad": "ITALIAN",
	})
}

func TestVIZSheetLibrettoPrefixPlusDigits(t *testing.T) {
	// l'OCR del campo intero storpia il gruppo "44" (variante 0 e 1), le sole cifre sono lette bene
	var specs []vizWordSpec
	specs = append(specs, vizSheetWord("numero_libretto", 0, "PA012U17"))
	specs = append(specs, vizSheetWord("numero_libretto", 1, "PA012U17"))
	specs = append(specs, vizSheetWord("libdig", 0, "0124417"))
	specs = append(specs, vizSheetWord("libdig", 2, "0124417"))
	out := vizRun(vizSheetResult(specs))
	vizWant(t, out, map[string]string{"numero_libretto": "PA0124417"})
}

func TestVIZSheetTypeAndCountryOnSameLine(t *testing.T) {
	// tipo e codice paese sono letti dalla stessa riga ("SI   PAN"); valori storpiati vicini a quelli noti
	mk := func(variant int, first, last string) []vizWordSpec {
		a := vizSheetWord("tipo", variant, first)
		b := vizSheetWord("tipo", variant, last)
		b.x = 400
		return []vizWordSpec{a, b}
	}
	var specs []vizWordSpec
	specs = append(specs, mk(0, "31", "PAN")...)
	specs = append(specs, mk(1, "SI", "paw")...)
	specs = append(specs, mk(2, "SI", "PAN")...)
	out := vizRun(vizSheetResult(specs))
	vizWant(t, out, map[string]string{"tipo": "SI", "paese": "PAN"})
}

func TestVIZSheetIgnoresLabelsAndPicksValue(t *testing.T) {
	// la fascia contiene l'etichetta (storpiata) sopra e il valore sotto, e un residuo della riga precedente
	i := vizBandIndex("cargo")
	base := float64(i*vizBandH + vizBandPad)
	specs := []vizWordSpec{
		{"RIGA", 40, base + 0, 14},             // residuo piccolo in cima
		{"Cargo/ Capacity", 40, base + 20, 18}, // etichetta
		{"ORDINARY SEAMAN", 40, base + 42, 30}, // valore
	}
	out := vizRun(vizSheetResult(specs))
	vizWant(t, out, map[string]string{"cargo": "ORDINARY SEAMAN"})
}

// ---- finalizeVIZ --------------------------------------------------------------------------------------------

func vizMRZData() *mrz.Data {
	return &mrz.Data{
		DocNumber: "PA0123401", Nationality: "PAN", Sex: "M", BirthDate: "2000-01-01", ExpiryDate: "2031-03-04",
		Optional: "PANAMA",
	}
}

func vizFullFields() map[string]string {
	return map[string]string{
		"tipo": "SI", "paese": "PAN", "nacionalidad": "PANAMANIAN", "luogo_nascita": "PANAMA",
		"data_emissione": "09-03-2026", "data_scadenza": "04-03-2031", "data_nascita": "01-01-2000",
		"livello": "SUPPORT", "cargo": "ORDINARY SEAMAN", "numero_libretto": "PA0123401",
		"identificazione": "8-123-45", "refrendo": "207123456", "id_marino": "510123456", "regionale": "PANAMA DR",
		"sesso_viz": "M",
	}
}

func TestVIZFinalizeAllConsistent(t *testing.T) {
	r := &Result{MRZ: vizMRZData(), Fields: vizFullFields()}
	finalizeVIZ(r)
	if len(r.Warnings) != 0 {
		t.Errorf("nessun avviso atteso, trovati %v", r.Warnings)
	}
	if r.Fields["data_nascita_iso"] != "2000-01-01" || r.Fields["data_scadenza_iso"] != "2031-03-04" || r.Fields["data_emissione_iso"] != "2026-03-09" {
		t.Errorf("date ISO non normalizzate: %v", r.Fields)
	}
	if _, ok := r.Fields["sesso_viz"]; ok {
		t.Error("la chiave di lavoro sesso_viz non deve restare nei campi")
	}
}

func TestVIZFinalizeDiscrepancies(t *testing.T) {
	f := vizFullFields()
	f["data_nascita"] = "01-01-2009"
	f["data_scadenza"] = "04-03-2030"
	f["sesso_viz"] = "F"
	f["nacionalidad"] = "ITALIAN"
	f["regionale"] = "COLON"
	r := &Result{MRZ: vizMRZData(), Fields: f}
	finalizeVIZ(r)
	joined := strings.Join(r.Warnings, " | ")
	for _, want := range []string{"data di nascita stampata diversa", "data di scadenza stampata diversa", "sesso stampato diverso",
		"nazionalita' stampata diversa", "regionale stampato diverso"} {
		if !strings.Contains(joined, want) {
			t.Errorf("avviso mancante %q in %q", want, joined)
		}
	}
	// gli avvisi non riportano valori personali
	for _, secret := range []string{"2009", "2030", "ITALIAN", "COLON", "01-01", "PA0123401", "510123456"} {
		if strings.Contains(joined, secret) {
			t.Errorf("l'avviso contiene un valore personale (%s): %q", secret, joined)
		}
	}
}

func TestVIZFinalizeMissingAndOrder(t *testing.T) {
	f := vizFullFields()
	delete(f, "cargo")
	delete(f, "refrendo") // il numero e' "PA...": il refrendo ci dovrebbe essere
	f["data_scadenza"] = "01-01-2020"
	r := &Result{Fields: f} // senza MRZ
	finalizeVIZ(r)
	joined := strings.Join(r.Warnings, " | ")
	if !strings.Contains(joined, "cargo") || !strings.Contains(joined, "refrendo") {
		t.Errorf("campi non letti non segnalati: %q", joined)
	}
	if !strings.Contains(joined, "scadenza stampata non successiva") {
		t.Errorf("scadenza anteriore all'emissione non segnalata: %q", joined)
	}
	// modello 2020 ("P"+cifre): niente refrendo atteso
	g := vizFullFields()
	g["numero_libretto"] = "P0123456"
	delete(g, "refrendo")
	r2 := &Result{Fields: g}
	finalizeVIZ(r2)
	if strings.Contains(strings.Join(r2.Warnings, " "), "refrendo") {
		t.Errorf("il modello 2020 non ha il refrendo: %v", r2.Warnings)
	}
}

func TestVIZFinalizeNoDuplicateNumberWarning(t *testing.T) {
	f := vizFullFields()
	f["numero_libretto"] = "PA0123402"
	r := &Result{MRZ: vizMRZData(), Fields: f, Warnings: []string{"numero libretto stampato (PA0123402) diverso dall'MRZ (PA0123401)"}}
	finalizeVIZ(r)
	n := 0
	for _, w := range r.Warnings {
		if strings.HasPrefix(w, "numero libretto stampato") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("avviso sul numero duplicato: %v", r.Warnings)
	}
}

// ---- geometria su immagini sintetiche ----------------------------------------------------------------------

// vizDrawMRZ disegna due righe di 44 "caratteri" a tratti sottili (3 aste + traversa) su fondo grigio chiaro,
// con una pendenza opzionale.
func vizDrawMRZ(w, h, x0, y0, pitch int, slope float64) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = 205
	}
	for line := 0; line < 2; line++ {
		for c := 0; c < 44; c++ {
			x := x0 + c*pitch
			dy := int(slope * float64(x-x0))
			top := y0 + line*(pitch*3) + dy
			for _, ax := range []int{0, 3, 6} {
				for y := top; y < top+22; y++ {
					g.SetGray(x+ax, y, color.Gray{40})
					g.SetGray(x+ax+1, y, color.Gray{40})
				}
			}
			for xx := x; xx < x+8; xx++ {
				g.SetGray(xx, top, color.Gray{40})
				g.SetGray(xx, top+11, color.Gray{40})
			}
		}
	}
	return g
}

func TestVIZLocateMRZSynthetic(t *testing.T) {
	g := vizDrawMRZ(1000, 1400, 120, 1250, 16, 0)
	geo, ok := vizLocateMRZ(g)
	if !ok {
		t.Fatal("MRZ sintetica non trovata")
	}
	if absf(geo.x0-120) > 8 || absf(geo.w-(43*16+8)) > 20 || absf(geo.y0-1250) > 4 {
		t.Errorf("geometria imprecisa: x0=%.0f w=%.0f y0=%.0f", geo.x0, geo.w, geo.y0)
	}
	if absf(geo.skew) > 0.004 {
		t.Errorf("scansione dritta ma pendenza stimata %.4f", geo.skew)
	}
}

func TestVIZLocateMRZSkew(t *testing.T) {
	g := vizDrawMRZ(1000, 1400, 120, 1250, 16, 0.02)
	geo, ok := vizLocateMRZ(g)
	if !ok {
		t.Fatal("MRZ inclinata non trovata")
	}
	if absf(geo.skew-0.02) > 0.006 {
		t.Errorf("pendenza stimata %.4f, attesa circa 0.020", geo.skew)
	}
}

func TestVIZPrepareImage(t *testing.T) {
	g := vizDrawMRZ(1000, 1400, 120, 1250, 16, 0)
	out := prepareVIZImage(g)
	b := out.Bounds()
	if b.Dx() != vizSheetW || b.Dy() != vizSheetH() {
		t.Errorf("foglio %dx%d, atteso %dx%d", b.Dx(), b.Dy(), vizSheetW, vizSheetH())
	}
	// il foglio viene riconosciuto da extractVIZ dalle dimensioni; svuota i ritagli conservati
	_ = vizTakeStash(vizSheetW, vizSheetH())

	// senza MRZ: ripiego, immagine ingrandita
	blank := image.NewGray(image.Rect(0, 0, 400, 300))
	for i := range blank.Pix {
		blank.Pix[i] = 230
	}
	o2 := prepareVIZImage(blank)
	if o2.Bounds().Dx() != 400*3 {
		t.Errorf("ripiego: larghezza %d, attesa %d", o2.Bounds().Dx(), 400*3)
	}
}

func TestVIZSheetLayoutConstants(t *testing.T) {
	if h := vizSheetH(); h > 10000 {
		t.Errorf("foglio alto %d: oltre il limite dell'OCR di Windows (10000)", h)
	}
	for _, b := range vizBands {
		for _, v := range vizVariants {
			up, down := vizWindow(b, v)
			if px := (up + down) * v.unit; px > float64(vizBandH-vizBandPad) {
				t.Errorf("fascia %s: ritaglio alto %.0f px, oltre la fascia di %d", b.key, px, vizBandH-vizBandPad)
			}
		}
	}
}

func TestVIZLevenshtein(t *testing.T) {
	if vizLev("kitten", "sitting") != 3 || vizLev("", "abc") != 3 || vizLev("abc", "abc") != 0 {
		t.Error("distanza di Levenshtein errata")
	}
	if s := vizSim("regional", "regional"); s != 1 {
		t.Errorf("somiglianza uguali = %v", s)
	}
}

// ---- spessore del tratto e binarizzazione adattiva ------------------------------------------------------------

// vizBars disegna n barre verticali nere larghe bw su fondo bianco, alte h.
func vizBars(n, bw, h int) (v []uint8, w int) {
	w = n * bw * 3
	v = make([]uint8, w*h)
	for i := range v {
		v[i] = 255
	}
	for b := 0; b < n; b++ {
		for y := 0; y < h; y++ {
			for x := 0; x < bw; x++ {
				v[y*w+b*bw*3+x] = 0
			}
		}
	}
	return v, w
}

func TestVIZStrokeRatio(t *testing.T) {
	v, w := vizBars(20, 4, 40)
	if r := vizStrokeRatio(v, w, 40, 128, 40); absf(r-0.1) > 1e-9 {
		t.Errorf("barre di 4 px su 40: rapporto %.4f, atteso 0.1", r)
	}
	// tratto doppio, stessa altezza: rapporto doppio
	v2, w2 := vizBars(20, 8, 40)
	if r := vizStrokeRatio(v2, w2, 40, 128, 40); absf(r-0.2) > 1e-9 {
		t.Errorf("barre di 8 px su 40: rapporto %.4f, atteso 0.2", r)
	}
	// invariante alla scala: tutto raddoppiato
	v3, w3 := vizBars(20, 8, 80)
	if r := vizStrokeRatio(v3, w3, 80, 128, 80); absf(r-0.1) > 1e-9 {
		t.Errorf("immagine raddoppiata: rapporto %.4f, atteso 0.1", r)
	}
	// i punti isolati (rumore, trattini) non spostano la mediana: si aggiunge una riga con molti brevi tratti
	for x := 0; x < w; x += 2 {
		v[x] = 0 // prima riga a tratti di 1 px
	}
	if r := vizStrokeRatio(v, w, 40, 128, 40); r < 0.09 || r > 0.11 {
		t.Errorf("con rumore: rapporto %.4f, atteso circa 0.1", r)
	}
	// niente inchiostro sotto soglia, altezza nulla, ritaglio vuoto
	if r := vizStrokeRatio(v, w, 40, 0, 40); r != 0 {
		t.Errorf("soglia 0: rapporto %.4f, atteso 0", r)
	}
	if r := vizStrokeRatio(v, w, 40, 128, 0); r != 0 {
		t.Errorf("altezza 0: rapporto %.4f, atteso 0", r)
	}
	if r := vizStrokeRatio(nil, 0, 0, 128, 10); r != 0 {
		t.Errorf("ritaglio vuoto: rapporto %.4f, atteso 0", r)
	}
	// la soglia conta: un tratto grigio (100) e' inchiostro a soglia 128 e non a soglia 60
	g := make([]uint8, len(v2))
	for i, x := range v2 {
		g[i] = x
		if x == 0 {
			g[i] = 100
		}
	}
	if vizStrokeRatio(g, w2, 40, 60, 40) != 0 || vizStrokeRatio(g, w2, 40, 128, 40) == 0 {
		t.Error("la soglia di binarizzazione non e' rispettata")
	}
}

func TestVIZPickThreshold(t *testing.T) {
	// rapporto lineare nella soglia: 0.125 a soglia 125
	if thr := vizPickThreshold(func(t int) float64 { return float64(t) / 1000 }); thr != 125 {
		t.Errorf("soglia %d, attesa 125", thr)
	}
	// tratto sempre troppo spesso: soglia minima; sempre sottile: soglia massima (nessun assottigliamento)
	if thr := vizPickThreshold(func(int) float64 { return 0.4 }); thr != vizThrMin {
		t.Errorf("tratto spesso: soglia %d, attesa %d", thr, vizThrMin)
	}
	if thr := vizPickThreshold(func(int) float64 { return 0.05 }); thr != vizThrMax {
		t.Errorf("tratto sottile: soglia %d, attesa %d", thr, vizThrMax)
	}
}

// vizThicken ispessisce i tratti scuri: ogni pixel prende il valore piu' scuro del suo intorno (2n+1)^2.
func vizThicken(g *image.Gray, n int) *image.Gray {
	b := g.Bounds()
	out := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			m := uint8(255)
			for dy := -n; dy <= n; dy++ {
				for dx := -n; dx <= n; dx++ {
					if image.Pt(x+dx, y+dy).In(b) {
						m = min(m, g.GrayAt(x+dx, y+dy).Y)
					}
				}
			}
			out.SetGray(x, y, color.Gray{m})
		}
	}
	return out
}

func TestVIZAdaptiveThresholdThickVsThin(t *testing.T) {
	thin := vizDrawMRZ(1000, 1400, 120, 1250, 16, 0)
	thick := vizThicken(thin, 1)
	var thr [2]int
	for i, g := range []*image.Gray{thin, thick} {
		geo, ok := vizLocateMRZ(g)
		if !ok {
			t.Fatalf("MRZ sintetica %d non trovata", i)
		}
		thr[i], _ = vizAdaptiveThreshold(g, geo)
	}
	if thr[1] >= thr[0] {
		t.Errorf("soglia con tratto spesso %d, non inferiore a quella con tratto sottile %d", thr[1], thr[0])
	}
	if v := vizExtraVariants(thr[0]); v != nil {
		t.Errorf("tratto sottile (soglia %d): attese nessuna variante aggiuntiva, ottenute %d", thr[0], len(v))
	}
	extra := vizExtraVariants(thr[1])
	if len(extra) == 0 {
		t.Fatal("tratto spesso: nessuna variante aggiuntiva")
	}
	for _, v := range extra {
		if v.unit <= 0 || v.up <= 0 || v.down <= 0 {
			t.Errorf("variante aggiuntiva malformata: %+v", v)
		}
		if v.thr != 0 && (v.thr < vizThrMin || v.thr > 255) {
			t.Errorf("soglia della variante fuori intervallo: %d", v.thr)
		}
	}
}

func TestVIZPrepareImageThickAddsExtraCrops(t *testing.T) {
	thick := vizThicken(vizDrawMRZ(1000, 1400, 120, 1250, 16, 0), 1)
	out := prepareVIZImage(thick)
	if b := out.Bounds(); b.Dx() != vizSheetW || b.Dy() != vizSheetH() {
		t.Fatalf("il foglio deve restare di dimensioni fisse: %dx%d", b.Dx(), b.Dy())
	}
	c := vizTakeStash(vizSheetW, vizSheetH())
	if c == nil {
		t.Fatal("scansione non conservata per i ritagli")
	}
	c.build() // i ritagli si costruiscono in extractVIZ, dopo la calibrazione
	if len(c.extra) == 0 {
		t.Fatal("ritagli aggiuntivi assenti con il tratto spesso")
	}
	for k := range c.extra {
		for i := range vizBands {
			if c.crops[vizExtraBase+k*len(vizBands)+i] == nil {
				t.Fatalf("ritaglio mancante: variante aggiuntiva %d, fascia %d", k, i)
			}
		}
	}
	// le varianti aggiuntive restano fuori dal foglio: nessuno slot oltre quelli del foglio
	for slot := range c.crops {
		if slot >= len(vizVariants)*len(vizBands)+2 && slot < vizExtraBase {
			t.Errorf("slot %d fuori da foglio e varianti aggiuntive", slot)
		}
	}
}

func TestVIZSheetExtraCropsVote(t *testing.T) {
	// la variante di riferimento del foglio sbaglia una cifra; tre ritagli aggiuntivi concordano sul valore giusto
	nb := len(vizBands)
	i := vizBandIndex("data_scadenza")
	extra := vizExtraVariants(50)
	words := map[int][]word{}
	for k := range extra[:3] {
		v := extra[k]
		up, _ := vizWindow(vizBands[i], v)
		y := float64(vizCropMargin) + up*v.unit
		words[vizExtraBase+k*nb+i] = []word{{"04-03-2031", box{40, y - 14, 300, 28}}}
	}
	sheet := vizSheetWord("data_scadenza", 0, "04-03-2037")
	out := &Result{Fields: map[string]string{}}
	vizExtractSheet(vizWords(vizSheetResult([]vizWordSpec{sheet})), out, vizCropWords{words: words, extra: extra})
	vizWant(t, out, map[string]string{"data_scadenza": "04-03-2031", "data_scadenza_iso": "2031-03-04"})
}

func TestVIZPickLongest(t *testing.T) {
	pick := func(order []string, counts map[string]int) string {
		f := map[string]float64{}
		for k, v := range counts {
			f[k] = float64(v)
		}
		return vizPickLongest(order, f)
	}
	// lettura incompleta molto votata e lettura completa con due voti: vince quella completa
	if got := pick([]string{"VERAGUAS", "VERAGUAS PEDRO", "VERAGUAS PXDRO", "VERAGUAS PEDO", "VERAGUAS PEDR"},
		map[string]int{"VERAGUAS": 5, "VERAGUAS PEDRO": 2, "VERAGUAS PXDRO": 2, "VERAGUAS PEDO": 1, "VERAGUAS PEDR": 1}); got != "VERAGUAS PEDRO" {
		t.Errorf("parita' tra letture complete: scelta %q", got)
	}
	// un'estensione con un solo voto non scavalca la lettura base
	if got := pick([]string{"VERAGUAS", "VERAGUAS 1"}, map[string]int{"VERAGUAS": 5, "VERAGUAS 1": 1}); got != "VERAGUAS" {
		t.Errorf("estensione con un voto: scelta %q", got)
	}
	// senza relazioni di prefisso resta la regola dei voti (e a parita' la priorita')
	if got := pick([]string{"PANAMA DR", "PANAMA OR"}, map[string]int{"PANAMA DR": 4, "PANAMA OR": 4}); got != "PANAMA DR" {
		t.Errorf("senza prefissi: scelta %q", got)
	}
	// parola intera: "PAN" non e' l'inizio di "PANAMA" (parole diverse)
	if got := pick([]string{"PAN", "PANAMA"}, map[string]int{"PAN": 3, "PANAMA": 2}); got != "PAN" {
		t.Errorf("prefisso di caratteri e non di parole: scelta %q", got)
	}
}
