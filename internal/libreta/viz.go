package libreta

// File di proprieta' dell'agente "VIZ": lettura dei campi stampati della pagina dati (zona visiva).
//
// Strategia in due modi:
//
//   - modo "foglio" (preferito): prepareVIZImage individua le due righe MRZ nella scansione (e ne stima
//     l'inclinazione) e, poiche' il layout della pagina dati e' fisso (le posizioni dei campi sono costanti in
//     rapporto alla larghezza W della MRZ), ritaglia ogni campo (etichetta + valore) in piu' varianti di
//     scala e finestra verticale. Le varianti sono impilate in un unico "foglio" a dimensioni costanti (OCR
//     dell'orchestratore) e conservate anche come immagini singole: extractVIZ le fa leggere una per una
//     (un solo avvio di PowerShell), perche' l'OCR di un ritaglio isolato e' piu' affidabile che in mezzo alla
//     pagina (sfondo guilloche, colonne vicine, ologrammi). Il valore finale di ogni campo e' quello su cui la
//     maggioranza delle letture concorda, dopo il riconoscimento per formato (data, numero, livello, testo...).
//     extractVIZ riconosce il foglio dalle dimensioni dell'immagine OCR. Il nero delle scansioni reali a 300 dpi e'
//     piu' spesso di quello dei facsimili: se lo spessore del tratto delle righe MRZ (vizAdaptiveThreshold) e'
//     alto, ai ritagli si aggiungono varianti binarizzate con soglia adattiva (vizExtraVariants), che entrano nel
//     voto insieme alle altre.
//   - modo "pagina" (ripiego, se la MRZ non si trova): l'OCR e' della pagina intera. I campi si ancorano per
//     PATTERN dei valori (date, numeri a 9 cifre, SUPPORT/OPERATIONAL/MANAGEMENT, "P"+7 cifre...) e per
//     etichette confrontate in modo approssimato (Levenshtein), perche' l'OCR storpia molte etichette.
//
// finalizeVIZ normalizza le date (DD-MM-YYYY e ISO) e confronta VIZ e MRZ senza riportare valori personali.

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/ocr"
)

// ---- geometria del foglio ---------------------------------------------------------------------------------

const (
	vizSheetW  = 1500 // larghezza del foglio in pixel
	vizBandH   = 130  // altezza di ogni fascia
	vizUnit    = 1300 // pixel del foglio per la larghezza W della riga MRZ (variante di riferimento)
	vizBandPad = 6    // margine superiore del ritaglio dentro la fascia

	vizCropMargin = 16 // margine bianco attorno ai ritagli inviati singolarmente all'OCR
)

// vizCropSet conserva i ritagli del foglio per l'OCR separato (un'immagine per ritaglio: l'OCR legge meglio un
// ritaglio isolato che lo stesso ritaglio in mezzo al foglio). prepareVIZImage e extractVIZ hanno firma fissa
// e non si passano l'immagine: il passaggio avviene qui, riconosciuto dalle dimensioni del foglio.
type vizCropSet struct {
	w, h  int
	g     *image.Gray // scansione in grigi (per costruire i ritagli dopo la calibrazione)
	geo   vizGeom
	thr   int                 // soglia adattiva (vizAdaptiveThreshold)
	crops map[int]*image.Gray // indice della fascia -> ritaglio
	extra []vizVariant        // varianti aggiuntive (solo ritagli, non nel foglio): slot = vizExtraBase + k*len(vizBands) + i
	jit   []vizJitter         // ritagli di testo spostati di pochi pixel (vedi vizJitterJobs): slot = vizJitterBase + k*len(vizBands) + i
	words []vizWordJob        // parole dei campi di testo, ognuna a se' (viz_words.go): slot = vizWordBase + ...
}

// vizExtraBase e' il primo slot dei ritagli delle varianti aggiuntive (oltre le fasce del foglio).
const vizExtraBase = 1000

var (
	vizStashMu sync.Mutex
	vizStash   *vizCropSet
)

func vizSetStash(c *vizCropSet) {
	vizStashMu.Lock()
	vizStash = c
	vizStashMu.Unlock()
}

// vizTakeStash restituisce (e svuota) i ritagli dell'ultimo foglio, se la sua dimensione e' quella dell'OCR.
func vizTakeStash(w, h int) *vizCropSet {
	vizStashMu.Lock()
	defer vizStashMu.Unlock()
	c := vizStash
	vizStash = nil
	if c == nil || c.w != w || c.h != h {
		return nil
	}
	return c
}

// vizOCRMany e' la lettura OCR di piu' immagini con un solo avvio (ocr.RecognizeMany); una variabile solo perche' il
// laboratorio (viz_lab_test.go) la sostituisce con una versione con cache su disco.
var vizOCRMany = ocr.RecognizeMany

// vizCropWords sono le parole lette nei ritagli, per slot, e le varianti aggiuntive a cui appartengono.
type vizCropWords struct {
	words map[int][]word
	extra []vizVariant
	jit   []vizJitter
	jobs  []vizWordJob
}

// vizOCRCrops esegue l'OCR dei ritagli (una sola chiamata a PowerShell per tutti) e restituisce le parole di
// ciascuno, per indice di fascia. In caso di errore restituisce una mappa vuota: restano le letture del foglio.
func vizOCRCrops(c *vizCropSet) vizCropWords {
	out := vizCropWords{words: map[int][]word{}, extra: c.extra, jit: c.jit, jobs: c.words}
	dir, err := os.MkdirTemp("", "vizcrop")
	if err != nil {
		return out
	}
	defer os.RemoveAll(dir)
	var idx []int
	var paths []string
	for slot, g := range c.crops {
		p := filepath.Join(dir, fmt.Sprintf("c%03d.png", slot))
		if imgproc.SavePNG(g, p) != nil {
			continue
		}
		idx = append(idx, slot)
		paths = append(paths, p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	rs, err := vizOCRMany(ctx, paths)
	if err != nil {
		return out
	}
	for i, r := range rs {
		out.words[idx[i]] = vizWords(r)
	}
	return out
}

// vizVariant: ogni campo e' ritagliato piu' volte con scala e finestra verticale diverse. Il riconoscimento
// OCR sbaglia in modo diverso sulle varianti (es. un trattino fuso con la cifra successiva): il valore finale
// e' quello su cui la maggioranza concorda. unit = pixel del foglio per W; up/down moltiplicano l'estensione
// del ritaglio sopra/sotto il valore (la variante stretta esclude quasi del tutto la riga precedente).
type vizVariant struct {
	unit, up, down float64
	thin           bool // assottiglia i tratti (la cifra "44" ravvicinata in grassetto viene letta come "U")
	thr            int  // se > 0 il ritaglio e' binarizzato a questa soglia (dopo lo stiramento del contrasto)
}

var vizVariants = []vizVariant{
	{1300, 1.0, 1.0, false, 0},
	{1050, 0.7, 0.9, true, 0},
	{1650, 0.85, 1.0, false, 0},
}

// vizBand descrive il ritaglio di un campo: x in unita' di W (larghezza MRZ) dal margine sinistro della MRZ,
// yc e' il centro verticale del VALORE rispetto al bordo superiore della prima riga MRZ (in unita' di W,
// negativo = sopra); up/down l'estensione del ritaglio sopra e sotto yc (0 = valori predefiniti).
type vizBand struct {
	key, kind string
	xa, xb    float64
	yc        float64
	up, down  float64
}

const (
	vizUpDef   = 0.036
	vizDownDef = 0.028
)

// Posizioni misurate sul facsimile 2020, sui due facsimili 2026 e sulla scansione reale (scarto tra campioni
// circa 0,01 W). Il valore sta sotto l'etichetta: i ritagli includono anche questa.
var vizBands = []vizBand{
	// tipo e codice paese stanno sulla stessa riga: letti insieme (la riga "SI   PAN" e' piu' leggibile dei due
	// gruppi isolati di 2-3 lettere)
	{"tipo", "tipopaese", 0.262, 0.56, -0.534, 0, 0},
	// riga del nome stampato ("Nombres/Given Names Apellidos/Surname"): non e' uno dei 14 campi, serve a confrontarlo con il
	// nome dell'MRZ (la riga 1 non ha check digit); si espone con vizPrintedName e non entra in Result.Fields
	{"nome", "text", 0.262, 0.86, -0.484, 0, 0},
	{"nacionalidad", "text", 0.262, 0.76, -0.440, 0, 0},
	{"sesso_viz", "sex", 0.262, 0.37, -0.390, 0, 0},
	{"luogo_nascita", "text", 0.37, 0.76, -0.390, 0, 0},
	{"data_emissione", "date", 0.262, 0.52, -0.345, 0, 0},
	{"data_scadenza", "date", 0.262, 0.52, -0.300, 0, 0},
	{"livello", "level", 0.262, 0.52, -0.258, 0, 0},
	{"cargo", "text", 0.262, 0.76, -0.208, 0, 0},
	{"id_marino", "num9", 0.262, 0.52, -0.100, 0.050, 0.030},
	{"numero_libretto", "libretto", 0.885, 1.12, -0.523, 0, 0},
	{"identificazione", "ident", 0.885, 1.12, -0.427, 0, 0},
	{"refrendo", "num9", 0.885, 1.12, -0.379, 0, 0},
	{"data_nascita", "date", 0.885, 1.12, -0.334, 0, 0},
	{"regionale", "text", 0.885, 1.12, -0.288, 0, 0},
	// solo le cifre del numero libretto (allineate a destra): la "P"/"PA" iniziale e' esclusa perche' l'OCR
	// legge male il gruppo prefisso+cifre in grassetto (es. "44" fuso in "U") ma legge bene le sole cifre
	{"libdig", "libdig", 0.935, 1.12, -0.523, 0, 0},
}

// numero di fasce del foglio: i campi, piu' le due righe MRZ.
func vizSheetH() int { return (len(vizVariants)*len(vizBands) + 2) * vizBandH }

// vizGeom e' la posizione della MRZ nella scansione, in pixel dell'immagine originale.
type vizGeom struct {
	x0, w float64   // margine sinistro e larghezza (44 caratteri)
	y0    float64   // bordo superiore della prima riga
	rows  [2][2]int // Y0,Y1 delle due righe
	skew  float64   // pendenza stimata (dy/dx) delle righe MRZ
	// scostamento verticale (in W) misurato per la colonna sinistra e per la destra dalla lettura del foglio (viz_calib.go)
	calL, calR float64
}

// ---- immagini ---------------------------------------------------------------------------------------------

// prepareVIZImage prepara l'immagine (gia' orientata) per l'OCR della pagina intera.
// Se trova la MRZ costruisce il "foglio" dei campi (vedi sopra); altrimenti ingrandisce e converte in grigi.
func prepareVIZImage(img image.Image) image.Image {
	g := vizToGray(img)
	if geo, ok := vizLocateMRZ(g); ok {
		if s := vizBuildSheet(g, geo); s != nil {
			return s
		}
	}
	return vizPreparePage(g, autoScale(img.Bounds().Dx()))
}

func vizToGray(img image.Image) *image.Gray {
	b := img.Bounds()
	if g, ok := img.(*image.Gray); ok && b.Min == (image.Point{}) {
		return g
	}
	out := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			out.Pix[y*out.Stride+x] = uint8((0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)) / 257)
		}
	}
	return out
}

// vizInk binarizza con soglia locale (inchiostro = true) un'immagine in grigi, eventualmente ridotta di k
// prendendo il pixel piu' scuro di ogni blocco k x k (conserva i tratti sottili).
func vizInk(g *image.Gray, k int) (ink []bool, w, h int) {
	gw, gh := g.Bounds().Dx(), g.Bounds().Dy()
	w, h = gw/k, gh/k
	if w < 8 || h < 8 {
		return nil, 0, 0
	}
	v := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m := uint8(255)
			for dy := 0; dy < k; dy++ {
				row := g.Pix[(y*k+dy)*g.Stride+x*k:]
				for dx := 0; dx < k; dx++ {
					if row[dx] < m {
						m = row[dx]
					}
				}
			}
			v[y*w+x] = float64(m)
		}
	}
	W := w + 1
	ii := make([]float64, W*(h+1))
	for y := 0; y < h; y++ {
		run := 0.0
		for x := 0; x < w; x++ {
			run += v[y*w+x]
			ii[(y+1)*W+x+1] = ii[y*W+x+1] + run
		}
	}
	rad := 12
	ink = make([]bool, w*h)
	for y := 0; y < h; y++ {
		ya, yb := max(0, y-rad), min(h, y+rad+1)
		for x := 0; x < w; x++ {
			xa, xb := max(0, x-rad), min(w, x+rad+1)
			mean := (ii[yb*W+xb] - ii[ya*W+xb] - ii[yb*W+xa] + ii[ya*W+xa]) / float64((xb-xa)*(yb-ya))
			if v[y*w+x] < mean-30 && v[y*w+x] < 140 {
				ink[y*w+x] = true
			}
		}
	}
	return ink, w, h
}

type vizRow struct{ y0, y1, x0, x1, runs int } // y1 e x1 esclusi; runs = gruppi di colonne con inchiostro (caratteri)

// vizTextRows trova le righe di inchiostro e la loro estensione orizzontale principale.
func vizTextRows(ink []bool, w, h int) []vizRow {
	cnt := make([]int, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if ink[y*w+x] {
				cnt[y]++
			}
		}
	}
	var rows []vizRow
	start, gap := -1, 0
	flush := func(end int) {
		if start < 0 {
			return
		}
		cols := make([]bool, w)
		for y := start; y < end; y++ {
			for x := 0; x < w; x++ {
				if ink[y*w+x] {
					cols[x] = true
				}
			}
		}
		x0, x1 := vizMainSpan(cols, end-start)
		runs, prev := 0, false
		for x := x0; x < x1; x++ {
			if cols[x] && !prev {
				runs++
			}
			prev = cols[x]
		}
		if end-start >= 6 && x1-x0 >= w/5 {
			rows = append(rows, vizRow{start, end, x0, x1, runs})
		}
		start = -1
	}
	minCnt := max(3, w/60)
	for y := 0; y < h; y++ {
		if cnt[y] >= minCnt {
			if start < 0 {
				start = y
			}
			gap = 0
		} else if start >= 0 {
			gap++
			if gap > 2 {
				flush(y - gap + 1)
				gap = 0
			}
		}
	}
	flush(h)
	return rows
}

// vizMainSpan restituisce il gruppo piu' largo di colonne con inchiostro separate da meno di maxGap.
func vizMainSpan(cols []bool, maxGap int) (int, int) {
	bx0, bx1 := 0, 0
	x0, last := -1, -1
	closeGroup := func() {
		if x0 >= 0 && last+1-x0 > bx1-bx0 {
			bx0, bx1 = x0, last+1
		}
	}
	for x, on := range cols {
		if !on {
			continue
		}
		if x0 >= 0 && x-last > maxGap {
			closeGroup()
			x0 = -1
		}
		if x0 < 0 {
			x0 = x
		}
		last = x
	}
	closeGroup()
	return bx0, bx1
}

// vizLocateMRZ cerca, partendo dal basso, due righe consecutive di altezza, margine sinistro e larghezza simili.
func vizLocateMRZ(g *image.Gray) (vizGeom, bool) {
	k := max(1, g.Bounds().Dx()/1200)
	ink, w, h := vizInk(g, k)
	if ink == nil {
		return vizGeom{}, false
	}
	// Scelta principale: le righe candidate di imgproc (cancella i bordi lunghi) con il loro periodo (vedi viz_geom.go).
	// Ripiego: la ricerca locale qui sotto.
	if a, b, ok := vizPickPair(vizCandidateRows(g, ink, w, k)); ok {
		return vizGeomFromPair(a, b, ink, w, k), true
	}
	rows := vizTextRows(ink, w, h)
	for i := len(rows) - 2; i >= 0; i-- {
		a, b := rows[i], rows[i+1]
		ha, hb := a.y1-a.y0, b.y1-b.y0
		hMax, hMin := max(ha, hb), min(ha, hb)
		wa, wb := a.x1-a.x0, b.x1-b.x0
		wMax, wMin := max(wa, wb), min(wa, wb)
		switch {
		case float64(hMin) < 0.65*float64(hMax),
			absInt(a.x0-b.x0) > w/25,
			float64(wMin) < 0.85*float64(wMax),
			float64(wMax) < 0.4*float64(w),
			b.y0-a.y1 > 3*hMax,
			hMax > h/10,
			a.runs < 25, b.runs < 25: // una riga MRZ ha ~44 caratteri separati; un bordo o un'ombra e' continuo
			continue
		}
		// Intersezione, non unione, dei due estremi: un bordo della tessera, un'ombra o la sfumatura della plastificazione
		// sporcano spesso UNA riga sola (nella scansione a4-1 la riga 1 risultava il 7% piu' larga e tutti i ritagli
		// finivano fuori posto di 50 px). Le righe MRZ hanno gli estremi allineati: la differenza dovuta alla forma
		// dei glifi agli estremi (I contro P, < contro cifra) e' di pochi pixel.
		x0, x1 := max(a.x0, b.x0), min(a.x1, b.x1)
		skew := vizSkew(ink, w, [2]vizRow{a, b})
		geo := vizGeom{skew: skew,
			x0: float64(x0 * k), w: float64((x1 - x0) * k), y0: float64(a.y0 * k),
			rows: [2][2]int{{a.y0 * k, a.y1 * k}, {b.y0 * k, b.y1 * k}},
		}
		return geo, true
	}
	return vizGeom{}, false
}

// vizBinRows binarizza g come imgproc, cancella le linee lunghe e ne restituisce le righe di testo.
func vizBinRows(g *image.Gray) []imgproc.Row {
	bin := imgproc.Binarize(g)
	imgproc.ClearLines(bin)
	return imgproc.TextRows(bin, 0.35)
}

// vizSkew stima la pendenza (dy/dx) delle righe MRZ: per ciascuna delle due righe calcola il baricentro verticale
// dell'inchiostro in 8 tratti e ne ricava la retta ai minimi quadrati; la stima e' la media delle due pendenze.
// Una scansione ruotata di 0,5 gradi sposta di circa 0,009 W in verticale i campi della colonna destra rispetto
// a quelli della sinistra.
func vizSkew(ink []bool, w int, rows [2]vizRow) float64 {
	h := len(ink) / w
	var slopes []float64
	for _, r := range rows {
		y0, y1 := max(0, r.y0-2), min(h, r.y1+2)
		const chunks = 8
		cw := (r.x1 - r.x0) / chunks
		if cw < 4 {
			continue
		}
		var xs, ys []float64
		for c := 0; c < chunks; c++ {
			xa, xb := r.x0+c*cw, r.x0+(c+1)*cw
			sum, n := 0.0, 0
			for y := y0; y < y1; y++ {
				for x := xa; x < xb; x++ {
					if ink[y*w+x] {
						sum += float64(y)
						n++
					}
				}
			}
			if n >= 10 {
				xs = append(xs, float64(xa+xb)/2)
				ys = append(ys, sum/float64(n))
			}
		}
		if len(xs) < 4 {
			continue
		}
		var mx, my float64
		for i := range xs {
			mx += xs[i]
			my += ys[i]
		}
		mx /= float64(len(xs))
		my /= float64(len(xs))
		var sxy, sxx float64
		for i := range xs {
			sxy += (xs[i] - mx) * (ys[i] - my)
			sxx += (xs[i] - mx) * (xs[i] - mx)
		}
		if sxx > 0 {
			slopes = append(slopes, sxy/sxx)
		}
	}
	if len(slopes) == 0 {
		return 0
	}
	sk := 0.0
	for _, v := range slopes {
		sk += v
	}
	sk /= float64(len(slopes))
	if sk > 0.05 || sk < -0.05 { // oltre 3 gradi non e' inclinazione: scarto la stima
		return 0
	}
	return sk
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// vizWindow restituisce l'estensione (in unita' di W) del ritaglio sopra e sotto il valore per la variante v.
func vizWindow(b vizBand, v vizVariant) (up, down float64) {
	up, down = b.up, b.down
	if up == 0 {
		up = vizUpDef
	}
	if down == 0 {
		down = vizDownDef
	}
	return up * v.up, down * v.down
}

// vizCropVals ritaglia r da g, lo porta alla scala s e lo restituisce in toni di grigio con il contrasto stirato
// (vizStretchCrop); maxW e maxH limitano le dimensioni del risultato (0 = nessun limite).
func vizCropVals(g *image.Gray, r image.Rectangle, s float64, maxW, maxH int) (vals []uint8, w, h int) {
	r = r.Intersect(g.Bounds())
	if r.Empty() {
		return nil, 0, 0
	}
	sc := imgproc.Scale(g.SubImage(r), s)
	sb := sc.Bounds()
	w, h = sb.Dx(), sb.Dy()
	if maxW > 0 {
		w = min(w, maxW)
	}
	if maxH > 0 {
		h = min(h, maxH)
	}
	vals = make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, _, _, _ := sc.At(sb.Min.X+x, sb.Min.Y+y).RGBA()
			vals[y*w+x] = uint8(r >> 8)
		}
	}
	vizStretchCrop(vals)
	return vals, w, h
}

// Spessore del tratto. Il testo stampato dalle stampanti a 300 dpi ha un nero piu' spesso di quello dei facsimili
// e il motore OCR di Windows non lo legge (tratti che si toccano, contatori delle cifre chiusi). Lo spessore si
// stima sulle due righe MRZ, di altezza e font noti: media dei tratti neri orizzontali tra il 25 e il 75 percentile
// (robusta a punti e trattini) divisa per l'altezza delle righe. La soglia di binarizzazione e' scelta in modo che
// il rapporto torni a quello tipico di un facsimile (vizTargetStroke).
const (
	vizTargetStroke = 0.125 // rapporto spessore/altezza dei glifi a cui si riporta il tratto
	vizThrMin       = 30
	vizThrMax       = 170
)

// vizStrokeRatio da' lo spessore tipico del tratto (pixel con valore < thr) rispetto all'altezza glyphH dei
// glifi, su un ritaglio in grigi w x h. Restituisce 0 se non c'e' inchiostro.
func vizStrokeRatio(v []uint8, w, h int, thr uint8, glyphH float64) float64 {
	var runs []int
	for y := 0; y < h; y++ {
		n := 0
		for x := 0; x < w; x++ {
			if v[y*w+x] < thr {
				n++
			} else if n > 0 {
				runs = append(runs, n)
				n = 0
			}
		}
		if n > 0 {
			runs = append(runs, n)
		}
	}
	if len(runs) < 8 || glyphH <= 0 {
		return 0
	}
	sort.Ints(runs)
	lo, hi := len(runs)/4, len(runs)*3/4
	sum := 0
	for _, r := range runs[lo : hi+1] {
		sum += r
	}
	return float64(sum) / float64(hi-lo+1) / glyphH
}

// vizPickThreshold sceglie la soglia di binarizzazione piu' alta per cui il rapporto spessore/altezza (calcolato
// da ratio, crescente con la soglia) non supera vizTargetStroke. Restituisce vizThrMin se anche la soglia minima
// supera l'obiettivo, vizThrMax se nessuna soglia lo supera (tratto sottile: non serve assottigliare).
func vizPickThreshold(ratio func(thr int) float64) int {
	best := vizThrMin
	for t := vizThrMin; t <= vizThrMax; t += 5 {
		if ratio(t) > vizTargetStroke*1.0001 {
			return best
		}
		best = t
	}
	return best
}

// vizAdaptiveThreshold misura lo spessore del tratto sulle righe MRZ e restituisce la soglia per i ritagli
// binarizzati e il rapporto spessore/altezza misurato con la soglia di riferimento 100.
func vizAdaptiveThreshold(g *image.Gray, geo vizGeom) (thr int, ref float64) {
	s := vizUnit / geo.w
	type rowCrop struct {
		v    []uint8
		w, h int
		gh   float64
	}
	var rows []rowCrop
	for j := 0; j < 2; j++ {
		r := image.Rect(int(geo.x0), geo.rows[j][0]-2, int(geo.x0+geo.w), geo.rows[j][1]+2)
		v, w, h := vizCropVals(g, r, s, 0, 0)
		if v != nil {
			rows = append(rows, rowCrop{v, w, h, float64(geo.rows[j][1]-geo.rows[j][0]) * s})
		}
	}
	if len(rows) == 0 {
		return vizThrMax, 0
	}
	ratio := func(t int) float64 {
		sum, n := 0.0, 0
		for _, r := range rows {
			if x := vizStrokeRatio(r.v, r.w, r.h, uint8(t), r.gh); x > 0 {
				sum += x
				n++
			}
		}
		if n == 0 {
			return 0
		}
		return sum / float64(n)
	}
	return vizPickThreshold(ratio), ratio(100)
}

// vizExtraVariants sono le varianti binarizzate (solo ritagli) che si aggiungono quando la soglia adattiva indica
// un tratto spesso: a pari soglia l'OCR legge diversamente a scale diverse, e il voto sceglie il valore piu' frequente.
func vizExtraVariants(thr int) []vizVariant {
	if thr >= vizThrMax { // tratto sottile a qualunque soglia: i ritagli in grigi bastano
		return nil
	}
	return []vizVariant{
		{1300, 1.0, 1.0, false, thr},
		{1950, 0.85, 1.0, false, thr},
		{1300, 0.7, 0.9, false, thr + 25},
		{1950, 0.85, 1.0, false, thr + 25},
		// finestra stretta (quasi solo il valore: senza l'etichetta il motore OCR legge meglio il testo in grassetto)
		{1300, 0.53, 0.53, false, thr},
		{1300, 0.53, 0.53, false, thr + 45},
		{1650, 0.53, 0.53, false, thr + 25},
		{1650, 0.53, 0.53, false, 0},
		{1950, 0.53, 0.53, false, thr + 45},
		{1950, 0.53, 0.53, false, 0},
		{1950, 0.53, 0.53, false, thr + 25},
	}
}

// vizBuildSheet costruisce il "foglio": ogni campo ritagliato nelle varianti di base (impilate in un'immagine grigia di dimensioni
// fisse che Extract fa leggere dall'OCR) piu' le due righe MRZ. Conserva scansione e geometria per extractVIZ, che dopo aver letto
// il foglio misura lo scostamento verticale della pagina (viz_calib.go) e costruisce tutti gli altri ritagli (vizCropSet.build).
func vizBuildSheet(g *image.Gray, geo vizGeom) image.Image {
	if vizGeomHook != nil {
		vizGeomHook(&geo)
	}
	if geo.w < 150 {
		return nil
	}
	sheet := image.NewGray(image.Rect(0, 0, vizSheetW, vizSheetH()))
	for i := range sheet.Pix {
		sheet.Pix[i] = 255
	}
	// put ritaglia r nel foglio (con i limiti della fascia)
	put := func(slot int, r image.Rectangle, v vizVariant) {
		vals, w, h := vizPrepCrop(g, geo, r, v, vizSheetW-24, vizBandH-vizBandPad)
		if vals == nil {
			return
		}
		oy := slot*vizBandH + vizBandPad
		for y := 0; y < h; y++ {
			copy(sheet.Pix[(oy+y)*sheet.Stride+24:], vals[y*w:(y+1)*w])
		}
	}
	for vi, v := range vizVariants {
		for i, b := range vizBands {
			put(vi*len(vizBands)+i, vizBandRect(geo, b, v, 0, 0), v)
		}
	}
	pad := int(0.01 * geo.w)
	for j := 0; j < 2; j++ {
		put(len(vizVariants)*len(vizBands)+j,
			image.Rect(int(geo.x0)-pad, geo.rows[j][0]-pad/2, int(geo.x0+geo.w)+pad, geo.rows[j][1]+pad/2),
			vizVariant{unit: vizUnit})
	}
	thr, _ := vizAdaptiveThreshold(g, geo)
	vizSetStash(&vizCropSet{w: vizSheetW, h: vizSheetH(), g: g, geo: geo, thr: thr})
	return sheet
}

// build costruisce i ritagli separati per l'OCR (uno per variante e campo, piu' le varianti aggiuntive binarizzate, quelle
// spostate dei testi liberi e le parole dei testi), con la geometria calibrata di c.geo.
func (c *vizCropSet) build() {
	g, geo, thr := c.g, c.geo, c.thr
	c.crops = map[int]*image.Gray{}
	field := func(slot int, b vizBand, v vizVariant, ox, oy float64) {
		vals, w, h := vizPrepCrop(g, geo, vizBandRect(geo, b, v, ox, oy), v, 0, 0)
		if vals != nil {
			c.crops[slot] = vizPadGray(vals, w, h)
		}
	}
	for vi, v := range vizVariants {
		for i, b := range vizBands {
			field(vi*len(vizBands)+i, b, v, 0, 0)
		}
	}
	c.extra = vizExtraVariants(thr)
	for k, v := range c.extra {
		for i, b := range vizBands {
			field(vizExtraBase+k*len(vizBands)+i, b, v, 0, 0)
		}
	}
	// testi liberi: stesse varianti spostate di pochi pixel, solo sulle scansioni a tratto spesso (le reali)
	c.jit = nil
	if c.extra != nil {
		c.jit = vizJitterJobs(thr)
		for k, j := range c.jit {
			for i, b := range vizBands {
				if vizJitterBands[b.key] {
					field(vizJitterBase+k*len(vizBands)+i, b, j.v, j.ox, j.oy)
				}
			}
		}
	}
	// campi di testo letti parola per parola (viz_words.go)
	c.words = vizBuildWordJobs(g, geo, c.crops)
}

// vizSoftRamp applica la rampa di binarizzazione morbida (vedi vizSoftWidth) attorno a thr.
func vizSoftRamp(vals []uint8, thr int) {
	for i, x := range vals {
		f := (float64(x) - float64(thr) + vizSoftWidth/2) / vizSoftWidth
		vals[i] = uint8(max(0, min(1, f)) * 255)
	}
}

// vizThin assottiglia i tratti scuri: ogni pixel prende il valore piu' chiaro del suo intorno 3x3.
func vizThin(v []uint8, w, h int) []uint8 {
	out := make([]uint8, len(v))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m := uint8(0)
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					xx, yy := x+dx, y+dy
					if xx >= 0 && yy >= 0 && xx < w && yy < h && v[yy*w+xx] > m {
						m = v[yy*w+xx]
					}
				}
			}
			out[y*w+x] = m
		}
	}
	return out
}

// vizStretchCrop schiarisce i toni chiari e medi (guilloche, filigrana, fondo colorato) e scurisce il testo:
// mappa l'intervallo [inchiostro, fondo] del ritaglio su [0,255]; inchiostro = 3% piu' scuro, fondo = mediana.
func vizStretchCrop(v []uint8) {
	var hist [256]int
	for _, x := range v {
		hist[x]++
	}
	ink, bg, acc := 0, 255, 0
	gotInk := false
	for x := 0; x < 256; x++ {
		acc += hist[x]
		if !gotInk && acc >= len(v)*3/100 {
			ink, gotInk = x, true
		}
		if acc >= len(v)*60/100 {
			bg = x
			break
		}
	}
	if bg-ink < 30 {
		return
	}
	// i toni chiari oltre il fondo diventano bianchi
	hi := float64(bg)
	for i, x := range v {
		f := (float64(x) - float64(ink)) / (hi - float64(ink))
		v[i] = uint8(max(0, min(1, f)) * 255)
	}
}

// ---- normalizzazione e distanza tra stringhe ---------------------------------------------------------------

func norm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(foldAccent(r))
		}
	}
	return b.String()
}

func foldAccent(r rune) rune {
	switch r {
	case 'á', 'à', 'â', 'ä':
		return 'a'
	case 'é', 'è', 'ê', 'ë':
		return 'e'
	case 'í', 'ì', 'î', 'ï':
		return 'i'
	case 'ó', 'ò', 'ô', 'ö':
		return 'o'
	case 'ú', 'ù', 'û', 'ü':
		return 'u'
	case 'ñ':
		return 'n'
	}
	return r
}

// vizLev e' la distanza di Levenshtein tra due stringhe.
func vizLev(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			c := 1
			if ra[i-1] == rb[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// vizSim e' la somiglianza normalizzata 0..1 (1 = uguali) tra due stringhe gia' normalizzate.
func vizSim(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	n := max(len([]rune(a)), len([]rune(b)))
	return 1 - float64(vizLev(a, b))/float64(n)
}

// ---- etichette --------------------------------------------------------------------------------------------

// vizLabel: etichetta stampata (parte spagnola e inglese) del campo.
type vizLabel struct{ key, es, en string }

var vizLabels = []vizLabel{
	{"tipo", "tipo", "type"},
	{"paese", "codigo del pais", "country code"},
	{"nomi", "nombres", "given names"},
	{"cognome", "apellidos", "surname"},
	{"nacionalidad", "nacionalidad", "nationality"},
	{"sesso_viz", "sexo", "sex"},
	{"luogo_nascita", "lugar de nacimiento", "place of birth"},
	{"data_emissione", "fecha de emision", "date of issue"},
	{"data_scadenza", "fecha de expiracion", "date of expiry"},
	{"livello", "nivel", "level"},
	{"cargo", "cargo", "capacity"},
	{"identificazione", "identificacion", "id number"},
	{"refrendo", "n de refrendo", "endorsement"},
	{"data_nascita", "fecha de nacimiento", "date of birth"},
	{"regionale", "regional", "regional"},
	{"id_marino", "id marino", "seafarers id"},
	{"libretto", "libreta de embarque", "seamans book"},
	{"firma", "firma del titular", "holders signature"},
}

// vizLabelSim da' la somiglianza massima del testo con l'etichetta di key (intera, parte ES o parte EN).
func vizLabelSim(text, key string) float64 {
	n := norm(text)
	best := 0.0
	for _, l := range vizLabels {
		if l.key != key {
			continue
		}
		es, en := norm(l.es), norm(l.en)
		for _, c := range []string{es + en, es, en} {
			best = max(best, vizSim(n, c))
		}
	}
	return best
}

// vizBestLabel restituisce la chiave dell'etichetta piu' simile al testo e la somiglianza.
func vizBestLabel(text string) (string, float64) {
	bestKey, best := "", 0.0
	for _, l := range vizLabels {
		if s := vizLabelSim(text, l.key); s > best {
			bestKey, best = l.key, s
		}
	}
	return bestKey, best
}

// ---- parole, righe e riquadri -----------------------------------------------------------------------------

type box struct{ x, y, w, h float64 }

func (b box) right() float64  { return b.x + b.w }
func (b box) bottom() float64 { return b.y + b.h }
func (b box) cx() float64     { return b.x + b.w/2 }
func (b box) cy() float64     { return b.y + b.h/2 }

type word struct {
	text string
	box
}

func union(a, b box) box {
	x0, y0 := minf(a.x, b.x), minf(a.y, b.y)
	x1, y1 := maxf(a.right(), b.right()), maxf(a.bottom(), b.bottom())
	return box{x0, y0, x1 - x0, y1 - y0}
}

type vizLine struct {
	text   string
	words  []word
	box            // riquadro dell'unione delle parole; h = altezza mediana delle parole
	ay, ah float64 // centro e altezza della prima parola (riferimento per il raggruppamento)
}

// vizGroupLines raggruppa le parole in righe per allineamento verticale (centro entro meta' dell'altezza della
// parola piu' piccola): righe vicine ma distinte (nome sopra, etichetta sotto) non si fondono. Le righe escono
// dall'alto in basso.
func vizGroupLines(ws []word) []vizLine {
	sorted := append([]word(nil), ws...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].cy() < sorted[j].cy() })
	var lines []vizLine
	for _, w := range sorted {
		placed := false
		for i := range lines {
			l := &lines[i]
			if absf(w.cy()-l.ay) < maxf(3, 0.5*minf(w.h, l.ah)) {
				l.words = append(l.words, w)
				l.box = union(l.box, w.box)
				placed = true
				break
			}
		}
		if !placed {
			lines = append(lines, vizLine{words: []word{w}, box: w.box, ay: w.cy(), ah: w.h})
		}
	}
	// colonne vicine sulla stessa riga: una distanza orizzontale grande tra due parole le separa in righe distinte
	var split []vizLine
	for _, l := range lines {
		sort.Slice(l.words, func(a, b int) bool { return l.words[a].x < l.words[b].x })
		cur := vizLine{words: []word{l.words[0]}, box: l.words[0].box, ay: l.ay, ah: l.ah}
		for _, w := range l.words[1:] {
			prev := cur.words[len(cur.words)-1]
			if w.x-prev.right() > 4*maxf(w.h, prev.h) {
				split = append(split, cur)
				cur = vizLine{words: []word{w}, box: w.box, ay: l.ay, ah: l.ah}
				continue
			}
			cur.words = append(cur.words, w)
			cur.box = union(cur.box, w.box)
		}
		split = append(split, cur)
	}
	lines = split
	for i := range lines {
		l := &lines[i]
		parts := make([]string, len(l.words))
		hs := make([]float64, len(l.words))
		for j, w := range l.words {
			parts[j] = w.text
			hs[j] = w.h
		}
		l.text = strings.Join(parts, " ")
		// altezza tipica: mediana delle altezze delle parole (esclude trattini e punti)
		sort.Float64s(hs)
		l.box.h = hs[len(hs)/2]
		l.box.y = l.ay - l.box.h/2
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].cy() < lines[j].cy() })
	return lines
}

func vizWords(res *ocr.Result) []word {
	var ws []word
	for _, l := range res.Lines {
		for _, w := range l.Words {
			if strings.TrimSpace(w.Text) == "" {
				continue
			}
			ws = append(ws, word{w.Text, box{w.X, w.Y, w.W, w.H}})
		}
	}
	return ws
}

// ---- riconoscimento dei valori ---------------------------------------------------------------------------

// vizDigitAlts: lettere che l'OCR scambia con cifre (la prima alternativa e' la piu' probabile).
var vizDigitAlts = map[rune]string{
	'O': "0", 'o': "0", 'Q': "0", 'D': "0",
	'I': "1", 'l': "1", 'i': "1", '|': "1", '!': "1",
	'Z': "2", 'z': "2",
	'A': "4",
	'S': "59", 's': "59", '$': "5",
	'G': "6", 'b': "6",
	'T': "7",
	'B': "8",
	'g': "9", 'q': "9",
}

// vizDigitVariants espande una stringa in tutte le varianti con sole cifre (al massimo 16), oppure nil se
// contiene caratteri che non possono essere cifre.
func vizDigitVariants(s string) []string {
	variants := []string{""}
	for _, r := range s {
		var opts string
		switch {
		case r >= '0' && r <= '9':
			opts = string(r)
		case vizDigitAlts[r] != "":
			opts = vizDigitAlts[r]
		default:
			return nil
		}
		var next []string
		for _, v := range variants {
			for _, o := range opts {
				next = append(next, v+string(o))
			}
		}
		variants = next
		if len(variants) > 16 {
			variants = variants[:16]
		}
	}
	return variants
}

type vizDate struct{ d, m, y int }

func (d vizDate) printed() string {
	return pad2(d.d) + "-" + pad2(d.m) + "-" + strconv.Itoa(d.y)
}
func (d vizDate) iso() string { return strconv.Itoa(d.y) + "-" + pad2(d.m) + "-" + pad2(d.d) }

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func vizValidDate(d, m, y int) bool {
	if y < 1900 || y > 2100 || m < 1 || m > 12 || d < 1 {
		return false
	}
	return d <= time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

var reDateSep = regexp.MustCompile(`^(\S{2})[-./ ]+(\S{2})[-./ ]+(\S{4})$`)

// vizDates interpreta un testo come data stampata DD-MM-YYYY; restituisce tutte le letture valide
// (l'OCR confonde alcune lettere con cifre, e la prima e' la piu' probabile).
func vizDates(s string) []vizDate {
	s = strings.TrimSpace(strings.Trim(s, "•·*|"))
	s = strings.TrimSpace(s)
	var parts [3]string
	if m := reDateSep.FindStringSubmatch(s); m != nil {
		parts = [3]string{m[1], m[2], m[3]}
	} else {
		var dig strings.Builder
		for _, r := range s {
			if r == '-' || r == '.' || r == '/' || r == ' ' {
				continue
			}
			dig.WriteRune(r)
		}
		d := dig.String()
		if len([]rune(d)) != 8 {
			return nil
		}
		rs := []rune(d)
		parts = [3]string{string(rs[0:2]), string(rs[2:4]), string(rs[4:8])}
	}
	var out []vizDate
	seen := map[vizDate]bool{}
	for _, dd := range vizDigitVariants(parts[0]) {
		for _, mm := range vizDigitVariants(parts[1]) {
			for _, yy := range vizDigitVariants(parts[2]) {
				d, _ := strconv.Atoi(dd)
				m, _ := strconv.Atoi(mm)
				y, _ := strconv.Atoi(yy)
				if vizValidDate(d, m, y) {
					v := vizDate{d, m, y}
					if !seen[v] {
						seen[v] = true
						out = append(out, v)
					}
				}
			}
		}
	}
	return out
}

// intervallo di anni plausibile per la data di ciascun campo.
func vizYearRange(key string) (int, int) {
	now := time.Now().Year()
	switch key {
	case "data_nascita":
		return 1920, now - 14
	case "data_emissione":
		return 2000, now + 1
	case "data_scadenza":
		return 2000, now + 15
	}
	return 1900, 2100
}

// vizPickDate sceglie, tra le letture, la prima con anno plausibile per il campo.
func vizPickDate(cands []vizDate, key string) (vizDate, bool) {
	lo, hi := vizYearRange(key)
	for _, c := range cands {
		if c.y >= lo && c.y <= hi {
			return c, true
		}
	}
	return vizDate{}, false
}

// vizFindDate cerca in una fila di parole (ordinate per x) una data, anche spezzata su piu' parole.
func vizFindDate(ws []word, key string) (vizDate, bool) {
	sorted := append([]word(nil), ws...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].x < sorted[j].x })
	for n := 1; n <= 3; n++ {
		for i := 0; i+n <= len(sorted); i++ {
			parts := make([]string, n)
			for j := 0; j < n; j++ {
				parts[j] = sorted[i+j].text
			}
			if d, ok := vizPickDate(vizDates(strings.Join(parts, " ")), key); ok {
				return d, true
			}
		}
	}
	return vizDate{}, false
}

var (
	reLibretto = regexp.MustCompile(`^(PA|P)(\S{7})$`)
	reIdent    = regexp.MustCompile(`^([A-Z]{1,2})(\S{7,8})$`)
)

// vizLibretto riconosce "P"+7 cifre o "PA"+7 cifre (anche con cifre storpiate in lettere).
func vizLibretto(s string) (string, bool) {
	s = vizExpandFusedFours(strings.ToUpper(vizAlnum(s)))
	if m := reLibretto.FindStringSubmatch(s); m != nil {
		if v := vizDigitVariants(m[2]); v != nil {
			digits := 0
			for _, r := range m[2] {
				if r >= '0' && r <= '9' {
					digits++
				}
			}
			if digits >= 4 {
				return m[1] + v[0], true
			}
		}
	}
	return "", false
}

// vizExpandFusedFours: il motore OCR legge due "4" ravvicinati in grassetto come un'unica "U" (il numero libretto
// "PA0124417" diventa "PA012U17"): se il numero e' corto di un carattere e contiene una U, la U vale "44".
func vizExpandFusedFours(s string) string {
	for _, pre := range []string{"PA", "P"} {
		if t, ok := strings.CutPrefix(s, pre); ok && len(t) == 6 && strings.Count(t, "U") == 1 {
			return pre + strings.Replace(t, "U", "44", 1)
		}
	}
	return s
}

func vizAlnum(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// vizNum9 riconosce un numero di 9 cifre (ID marittimo, refrendo).
func vizNum9(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 9 || len(s) > 11 {
		return "", false
	}
	s = vizAlnum(s)
	if len(s) != 9 {
		return "", false
	}
	digits := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	if digits < 6 {
		return "", false
	}
	if v := vizDigitVariants(s); v != nil {
		return v[0], true
	}
	return "", false
}

// vizIdent riconosce l'identificazione: lettere + 7/8 cifre; in subordine una sequenza con almeno 6 cifre.
func vizIdent(s string) (string, bool) {
	raw := strings.ToUpper(strings.TrimSpace(s))
	a := vizAlnum(raw)
	if m := reIdent.FindStringSubmatch(a); m != nil {
		digits := 0
		for _, r := range m[2] {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits >= 5 {
			if v := vizDigitVariants(m[2]); v != nil {
				return m[1] + v[0], true
			}
		}
	}
	n := 0
	for _, r := range a {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	if n >= 6 && len(a) <= 14 {
		return strings.Trim(raw, " .,:;*•|"), true
	}
	return "", false
}

// vizIdentLoose riconosce l'identificazione come codice alfanumerico di 6-12 caratteri con almeno una cifra: il
// documento d'identita' del marittimo ha il formato del suo Paese (cedula panamense, lettera e sei cifre, codici di nove
// caratteri misti...). Solo nella fascia dell'identificazione, dopo vizIdent.
func vizIdentLoose(s string) (string, bool) {
	a := strings.ToUpper(vizAlnum(s))
	if len(a) < 6 || len(a) > 12 {
		return "", false
	}
	for _, r := range a {
		if r >= '0' && r <= '9' {
			return a, true
		}
	}
	return "", false
}

var vizLevels = []string{"SUPPORT", "OPERATIONAL", "MANAGEMENT"}

// vizLevel riconosce SUPPORT / OPERATIONAL / MANAGEMENT con tolleranza ai caratteri storpiati.
func vizLevel(s string) (string, bool) {
	n := norm(s)
	if len(n) < 5 {
		return "", false
	}
	best, bestS := "", 0.0
	for _, l := range vizLevels {
		if sc := vizSim(n, strings.ToLower(l)); sc > bestS {
			best, bestS = l, sc
		}
	}
	if bestS >= 0.65 {
		return best, true
	}
	return "", false
}

// vizCleanText ripulisce un valore di testo: maiuscolo, niente simboli spuri, spazi singoli.
func vizCleanText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == ' ', r == '-', r == '\'', r == '/', r == '.', r == '(', r == ')':
			b.WriteRune(r)
		case r == '*' || r == '•' || r == '·' || r == ',':
			// scartati
		default:
			b.WriteRune(' ')
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	return strings.Trim(out, " -'./")
}

func vizLetters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

// vizIsLabelish e' vero se la riga somiglia a un'etichetta stampata (bilingue o storpiata).
func vizIsLabelish(text string) bool {
	_, s := vizBestLabel(text)
	if s >= 0.7 {
		return true
	}
	return strings.Contains(text, "/") && s >= 0.5
}

// ---- estrazione --------------------------------------------------------------------------------------------

// extractVIZ legge i campi stampati. Se l'immagine OCR e' il foglio costruito da prepareVIZImage interpreta
// ogni fascia; altrimenti lavora sulla pagina intera ancorandosi ai pattern dei valori e alle etichette.
func extractVIZ(res *ocr.Result, out *Result) {
	if out.Fields == nil {
		out.Fields = map[string]string{}
	}
	words := vizWords(res)
	if res.Width == vizSheetW && res.Height == vizSheetH() {
		var crops vizCropWords
		if c := vizTakeStash(res.Width, res.Height); c != nil {
			c.geo = vizApplyCalibration(c.geo, vizSheetSlots(words))
			c.build()
			crops = vizOCRCrops(c)
		}
		vizExtractSheet(words, out, crops)
		return
	}
	vizExtractPageSet(words, vizTakePage(res.Width, res.Height), out)
}

// vizSetDate registra la data in forma stampata e ISO.
func vizSetDate(out *Result, key string, d vizDate) {
	out.Fields[key] = d.printed()
	out.Fields[key+"_iso"] = d.iso()
}

// vizRefrendoLabel e' la chiave delle letture che registra se l'etichetta del refrendo (presente solo su alcuni
// modelli: "Nº Titulo/Certificate" o "Endorsement") e' comparsa in almeno una variante della fascia.
const vizRefrendoLabel = "refrendo_label"

func vizHasRefrendoLabel(ws []word) bool {
	for _, w := range ws {
		n := norm(w.text)
		for _, k := range []string{"certif", "endors", "refrend", "titulo"} {
			if strings.Contains(n, k) {
				return true
			}
		}
	}
	return false
}

// vizTrace (solo diagnostica dei test) riceve la lettura di ogni campo in ogni variante.
var vizTrace func(key, variant string, fields map[string]string)

// vizGeomHook (solo diagnostica dei test) puo' spostare la geometria per misurare la sensibilita' del voto.
var vizGeomHook func(*vizGeom)

// vizTraceWidth (solo diagnostica dei test) riceve la larghezza misurata dagli estremi e quella stimata dal passo.
var vizTraceWidth func(measured, fromPitch float64)

// vizSoftWidth: i ritagli "binarizzati" non hanno un taglio netto alla soglia ma una rampa lineare larga vizSoftWidth
// livelli di grigio attorno ad essa (nero sotto thr-w/2, bianco sopra thr+w/2): i tratti si assottigliano senza
// spezzarsi e il bordo resta sfumato. Misurato sulle tre scansioni reali (due stampe): l'accuratezza media dei
// caratteri letti nelle 11 varianti sale da 0,56 (taglio netto) a 0,74 con la rampa di 180, 0,71 con 255, 0,66 con 90.
const vizSoftWidth = 180

// vizTraceWeights (solo diagnostica dei test) riceve i pesi delle varianti calibrati sull'MRZ.
var vizTraceWeights func(map[int]float64)

func vizExtractSheet(words []word, out *Result, crops vizCropWords) {
	nb := len(vizBands)
	slots := vizSheetSlots(words)
	rd := vizReadings{}
	for i, b := range vizBands {
		tag, id, unit := "", 0, 1.0
		read := func(ws []word, expY float64) {
			tmp := &Result{Fields: map[string]string{}}
			vizReadBand(b, ws, expY, tmp)
			if h, err := strconv.ParseFloat(tmp.Fields[vizHeightKey], 64); err == nil {
				tmp.Fields["_hw"] = strconv.FormatFloat(h/unit, 'f', 4, 64) // altezza in unita' di W
			}
			rd[b.key] = append(rd[b.key], vizCand{id, tmp.Fields})
			if b.key == "refrendo" && vizHasRefrendoLabel(ws) {
				rd[vizRefrendoLabel] = []vizCand{{id, nil}}
			}
			if vizTrace != nil {
				vizTrace(b.key, tag, tmp.Fields)
			}
		}
		for vi, v := range vizVariants {
			up, _ := vizWindow(b, v)
			// i ritagli OCR-ati da soli hanno la precedenza (letture piu' pulite): vengono prima nell'ordine
			unit = v.unit
			tag, id = fmt.Sprintf("v%d-crop", vi), vizVariantID(vi, true)
			if ws, ok := crops.words[vi*nb+i]; ok {
				read(ws, float64(vizCropMargin)+up*v.unit)
			}
			tag, id = fmt.Sprintf("v%d-foglio", vi), vizVariantID(vi, false)
			read(slots[vi*nb+i], float64((vi*nb+i)*vizBandH+vizBandPad)+up*v.unit)
		}
		// varianti aggiuntive (binarizzate): solo ritagli, in coda per priorita'
		for k, v := range crops.extra {
			up, _ := vizWindow(b, v)
			unit = v.unit
			tag, id = fmt.Sprintf("x%d", k), vizExtraID(k)
			if ws, ok := crops.words[vizExtraBase+k*nb+i]; ok {
				read(ws, float64(vizCropMargin)+up*v.unit)
			}
		}
	}
	for k, j := range crops.jit {
		for i, b := range vizBands {
			if ws, ok := crops.words[vizJitterBase+k*nb+i]; ok && vizJitterBands[b.key] {
				up, _ := vizWindow(b, j.v)
				tmp := &Result{Fields: map[string]string{}}
				vizReadBand(b, ws, float64(vizCropMargin)+(up-j.oy)*j.v.unit, tmp)
				if h, err := strconv.ParseFloat(tmp.Fields[vizHeightKey], 64); err == nil {
					tmp.Fields["_hw"] = strconv.FormatFloat(h/j.v.unit, 'f', 4, 64)
				}
				rd[b.key] = append(rd[b.key], vizCand{vizJitterID(len(crops.extra), k), tmp.Fields})
				if vizTrace != nil {
					vizTrace(b.key, fmt.Sprintf("j%d", k), tmp.Fields)
				}
			}
		}
	}
	vizReadWordJobs(rd, crops)
	vizVoteAll(rd, nil, out)
	vizTakeName(out, out.Fields)
	vizStripInternal(out.Fields)
	vizSetReadings(out, rd) // finalizeVIZ rivota con i pesi calibrati sull'MRZ
}

// vizPickLongest sceglie, tra letture di testo con i rispettivi pesi (order e' ordinato per peso decrescente),
// quella con il punteggio maggiore: peso proprio piu' peso delle letture che ne sono l'inizio o la fine per parole
// intere (l'OCR perde spesso l'ultima parola di un valore, es. "VERAGUAS" per "VERAGUAS PEDRO", o la prima, es.
// "SEAMAN" per "ORDINARY SEAMAN"). Una lettura con piu' parole di un'altra conta solo se ha almeno due voti propri. A parita' vince la piu' pesata.
func vizPickLongest(order []string, counts map[string]float64) string {
	toks := func(s string) []string { return strings.Fields(s) }
	// isPart: p e' un inizio o una fine di v, per parole intere
	isPart := func(p, v string) bool {
		a, b := toks(p), toks(v)
		if len(a) >= len(b) {
			return false
		}
		pre, suf := true, true
		for i := range a {
			if a[i] != b[i] {
				pre = false
			}
			if a[i] != b[len(b)-len(a)+i] {
				suf = false
			}
		}
		return pre || suf
	}
	isExt := func(v string) bool {
		for _, p := range order {
			if isPart(p, v) {
				return true
			}
		}
		return false
	}
	// vicinanza alle altre letture (pesata): a parita' di punteggio vince la lettura "centrale", che
	// condivide piu' caratteri con le altre (le letture sbagliate differiscono in punti diversi)
	closeness := func(v string) float64 {
		sum := 0.0
		for _, o := range order {
			if o != v {
				sum += counts[o] * vizSim(norm(v), norm(o))
			}
		}
		return sum
	}
	best, bestScore, bestClose := order[0], counts[order[0]], -1.0
	for _, v := range order {
		score, hasPart := counts[v], false
		for _, p := range order {
			if isPart(p, v) {
				score += counts[p]
				hasPart = true
			}
		}
		if !hasPart || counts[v] < 2-1e-9 {
			continue // le letture senza un'altra come parte restano alla regola dei voti
		}
		switch {
		case score > bestScore+1e-9:
			best, bestScore, bestClose = v, score, -1
		case score > bestScore-1e-9 && v != best && isExt(best):
			if bestClose < 0 {
				bestClose = closeness(best)
			}
			if c := closeness(v); c > bestClose+1e-9 {
				best, bestClose = v, c
			}
		}
	}
	return best
}

// vizSnapTo riporta una lettura al valore noto se e' abbastanza simile (cifre scambiate con lettere comprese).
func vizSnapTo(v, known string, minSim float64, digitsToLetters bool) string {
	n := strings.ToUpper(vizAlnum(v))
	if digitsToLetters {
		n = strings.NewReplacer("0", "O", "1", "I", "3", "S", "5", "S", "8", "B").Replace(n)
	}
	if vizSim(strings.ToLower(n), strings.ToLower(known)) >= minSim {
		return known
	}
	return v
}

// vizSnapNationality riporta un aggettivo di nazionalita' storpiato dall'OCR (es. "TALIAN") a quello noto piu'
// vicino, se la somiglianza e' alta; altrimenti lascia il valore com'e'.
func vizSnapNationality(v string) string {
	n := norm(v)
	best, bestS := "", 0.0
	for dem := range vizDemonyms {
		if s := vizSim(n, norm(dem)); s > bestS || (s == bestS && dem < best) {
			best, bestS = dem, s
		}
	}
	if bestS >= 0.8 {
		return best
	}
	return v
}

// vizReadBand interpreta le parole di una fascia secondo il tipo di campo.
func vizReadBand(b vizBand, ws []word, expY float64, out *Result) {
	if len(ws) == 0 {
		return
	}
	switch b.kind {
	case "date":
		if d, ok := vizFindDate(ws, b.key); ok {
			vizSetDate(out, b.key, d)
		}
	case "libretto":
		if v, ok := vizScanTokens(ws, vizLibretto); ok {
			out.Fields[b.key] = v
		}
		out.Fields["libpre"] = vizLibPrefix(ws)
	case "libdig":
		if v, ok := vizScanTokens(ws, vizLast7Digits); ok {
			out.Fields[b.key] = v
		}
	case "num9":
		if v, ok := vizScanTokens(ws, vizNum9); ok {
			out.Fields[b.key] = v
		}
	case "ident":
		if v, ok := vizScanTokens(ws, func(s string) (string, bool) {
			if strings.Contains(s, "/") { // etichetta bilingue
				return "", false
			}
			if v, ok := vizIdent(s); ok {
				return v, true
			}
			return vizIdentLoose(s)
		}); ok {
			out.Fields[b.key] = v
		}
	case "level":
		if v, ok := vizScanTokens(ws, vizLevel); ok {
			out.Fields[b.key] = v
		}
	case "sex":
		for _, w := range ws {
			t := strings.ToUpper(strings.Trim(w.text, " .,:*"))
			if t == "M" || t == "F" || t == "X" {
				out.Fields[b.key] = t
			}
		}
	case "tipopaese":
		l := vizValueLine(ws, expY, true, false)
		if l == nil {
			return
		}
		// le due colonne sulla stessa riga sono righe distinte per vizGroupLines: si riuniscono per allineamento
		row := append([]word(nil), ws...)
		sort.Slice(row, func(i, j int) bool { return row[i].x < row[j].x })
		var toks []string
		for _, w := range row {
			if absf(w.cy()-l.cy()) > 0.6*l.h {
				continue
			}
			if t := strings.ToUpper(vizAlnum(w.text)); t != "" {
				toks = append(toks, t)
			}
		}
		switch {
		case len(toks) == 1 && len(toks[0]) == 3:
			out.Fields["paese"] = toks[0]
		case len(toks) == 1 && len(toks[0]) <= 3:
			out.Fields["tipo"] = toks[0]
		case len(toks) >= 2:
			if len(toks[0]) <= 3 {
				out.Fields["tipo"] = toks[0]
			}
			if last := toks[len(toks)-1]; len(last) == 3 {
				out.Fields["paese"] = last
			}
		}
	case "short":
		if v := vizPickLine(ws, expY, true); v != "" {
			v = vizAlnum(v)
			if b.key == "paese" {
				if len(v) != 3 {
					return
				}
			} else if len(v) < 1 || len(v) > 3 {
				return
			}
			out.Fields[b.key] = strings.ToUpper(v)
		}
	default:
		if l := vizValueLine(ws, expY, false, true); l != nil {
			v := vizCleanText(l.text)
			if b.key == "cargo" {
				v, _ = vizDecodeRankTail(v) // la sigla di grado ("II/2") esce dall'OCR a pezzi
			}
			if vizLetters(v) >= 2 {
				out.Fields[b.key] = v
				out.Fields[vizHeightKey] = strconv.FormatFloat(l.h, 'f', 1, 64) // altezza della riga, per vizSheetHeight
			}
		}
	}
}

// vizHeightKey e' la chiave di lavoro (solo nelle letture per variante) con l'altezza in pixel della riga letta.
const vizHeightKey = "_h"

// vizLast7Digits legge le ultime 7 cifre di un token (le lettere iniziali eventuali sono residui del prefisso).
func vizLast7Digits(s string) (string, bool) {
	a := vizAlnum(s)
	if len(a) < 7 {
		return "", false
	}
	tail := a[len(a)-7:]
	digits := 0
	for _, r := range tail {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	if digits < 5 {
		return "", false
	}
	if v := vizDigitVariants(tail); v != nil {
		return v[0], true
	}
	return "", false
}

// vizLibPrefix deduce il prefisso del numero libretto ("PA" o "P") dalla lettura del campo intero, anche se le
// cifre sono storpiate: "PA" se il secondo carattere e' una A, "P" se e' una cifra (o somiglia a una cifra).
func vizLibPrefix(ws []word) string {
	for _, l := range vizGroupLines(ws) {
		for _, w := range l.words {
			t := strings.ToUpper(vizAlnum(w.text))
			if len(t) < 6 || len(t) > 10 || t[0] != 'P' {
				continue
			}
			if t[1] == 'A' {
				return "PA"
			}
			if _, ok := vizDigitAlts[rune(t[1])]; ok || (t[1] >= '0' && t[1] <= '9') {
				return "P"
			}
		}
	}
	return ""
}

// vizScanTokens prova il riconoscitore su ogni parola e sulle sequenze di 2-3 parole adiacenti della fascia.
func vizScanTokens(ws []word, rec func(string) (string, bool)) (string, bool) {
	lines := vizGroupLines(ws)
	for n := 1; n <= 3; n++ {
		for _, l := range lines {
			for i := 0; i+n <= len(l.words); i++ {
				parts := make([]string, n)
				for j := 0; j < n; j++ {
					parts[j] = l.words[i+j].text
				}
				if v, ok := rec(strings.Join(parts, "")); ok {
					return v, true
				}
			}
		}
	}
	return "", false
}

// vizFirstSegment tiene solo il primo tratto della riga: una distanza maggiore di 2 altezze tra due parole
// indica un altro elemento (filigrana, colonna vicina), non una parola del valore.
func vizFirstSegment(l vizLine) string {
	parts := []string{l.words[0].text}
	for i := 1; i < len(l.words); i++ {
		if l.words[i].x-l.words[i-1].right() > 2*l.h {
			break
		}
		parts = append(parts, l.words[i].text)
	}
	return strings.Join(parts, " ")
}

// vizPickLine sceglie in una fascia la riga del valore (solo il primo tratto, vedi vizFirstSegment) e la pulisce.
func vizPickLine(ws []word, expY float64, short bool) string {
	l := vizValueLine(ws, expY, short, true)
	if l == nil {
		return ""
	}
	return vizCleanText(l.text)
}

// vizLabelWords sono le parole delle etichette bilingui (normalizzate): una riga fatta solo di queste e' il frammento
// di un'etichetta spezzata dall'ologramma (es. "DE" di "Lugar de nacimiento"), non un valore.
var vizLabelWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`de del la lugar nacimiento place of birth fecha emision expiracion date issue expiry nivel level
		cargo capacity nacionalidad nationality nombres given names apellidos surname sexo sex codigo pais country code
		tipo type regional identificacion id number libreta embarque seaman s book marino seafarers titular holder signature firma`) {
		vizLabelWords[w] = true
	}
}

// vizLabelFragment e' vero se il testo e' composto solo da parole di etichetta.
func vizLabelFragment(t string) bool {
	f := strings.Fields(strings.ToLower(norm2(t)))
	if len(f) == 0 {
		return false
	}
	for _, w := range f {
		if !vizLabelWords[w] {
			return false
		}
	}
	return true
}

// norm2 come norm ma conserva gli spazi tra le parole.
func norm2(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(foldAccent(r))
		default:
			b.WriteRune(' ')
		}
	}
	return b.String()
}

// vizValueLine sceglie in una fascia la riga del valore. Scarta le etichette (confronto approssimato); se c'e'
// un'etichetta, il valore sta sotto di essa. Tiene le righe con altezza del testo grande (il valore e' in
// grassetto, piu' grande dell'etichetta e di eventuali residui della riga precedente) e prende la piu' vicina
// alla posizione attesa. Con cut si tiene solo il primo tratto della riga.
func vizValueLine(ws []word, expY float64, short, cut bool) *vizLine {
	labelY := -1.0
	var cands []vizLine
	all := vizGroupLines(ws)
	for _, l := range all {
		if vizIsLabelish(l.text) {
			labelY = maxf(labelY, l.cy())
		}
	}
	for _, l := range all {
		if cut {
			l.text = vizFirstSegment(l)
		}
		t := vizCleanText(l.text)
		if t == "" || (!short && vizLetters(t) < 2) || vizIsLabelish(l.text) || vizLabelFragment(t) {
			continue
		}
		if labelY >= 0 && l.cy() <= labelY {
			continue
		}
		cands = append(cands, l)
	}
	if len(cands) == 0 {
		return nil
	}
	maxH := 0.0
	for _, l := range cands {
		maxH = maxf(maxH, l.h)
	}
	var best *vizLine
	for i := range cands {
		l := &cands[i]
		if l.h < 0.7*maxH {
			continue
		}
		if best == nil || absf(l.cy()-expY) < absf(best.cy()-expY) {
			best = l
		}
	}
	return best
}

// ---- modo pagina intera ------------------------------------------------------------------------------------

type vizDateTok struct {
	cands []vizDate
	box   box
}

func vizExtractPage(words []word, out *Result) {
	lines := vizGroupLines(words)

	// 1. valori a pattern: libretto, identificazione, numeri a 9 cifre, livello, date
	var libBox *box
	var nums []vizNumTok
	var levelBox *box
	var dates []vizDateTok
	for _, l := range lines {
		for n := 1; n <= 3; n++ {
			for i := 0; i+n <= len(l.words); i++ {
				seg := l.words[i : i+n]
				var parts []string
				bb := seg[0].box
				for _, w := range seg {
					parts = append(parts, w.text)
					bb = union(bb, w.box)
				}
				joined := strings.Join(parts, "")
				if _, ok := out.Fields["numero_libretto"]; !ok {
					if v, ok := vizLibretto(joined); ok && n == 1 {
						out.Fields["numero_libretto"] = v
						b := bb
						libBox = &b
					}
				}
				if v, ok := vizNum9(joined); ok {
					dup := false
					for _, x := range nums {
						if x.v == v {
							dup = true
						}
					}
					if !dup {
						nums = append(nums, vizNumTok{v, bb})
					}
				}
				if n == 1 && levelBox == nil {
					if v, ok := vizLevel(joined); ok {
						out.Fields["livello"] = v
						b := bb
						levelBox = &b
					}
				}
				if ds := vizDates(strings.Join(parts, " ")); len(ds) > 0 {
					dates = append(dates, vizDateTok{ds, bb})
				}
			}
		}
	}
	vizDedupDates(&dates)

	// 2. date: le tre date sono in ordine cronologico nascita < emissione < scadenza
	vizAssignDates(dates, lines, libBox, out)

	// 3. numeri a 9 cifre: refrendo nella colonna destra, id marino a sinistra
	vizAssignNum9(nums, libBox, lines, out)

	// 4. identificazione: token alfanumerico nella colonna destra
	for _, l := range lines {
		for _, w := range l.words {
			if _, ok := out.Fields["identificazione"]; ok {
				break
			}
			if libBox != nil && w.cx() < libBox.x-libBox.w {
				continue
			}
			if v, ok := vizIdent(w.text); ok && vizAlnumDigits(w.text) >= 6 && !strings.Contains(w.text, "/") {
				if _, isNum := vizNum9(w.text); isNum {
					continue
				}
				if vl, isLib := vizLibretto(w.text); isLib && vl == out.Fields["numero_libretto"] {
					continue
				}
				if d := vizDates(w.text); len(d) > 0 {
					continue
				}
				out.Fields["identificazione"] = v
			}
		}
	}

	// 5. campi di testo: etichetta approssimata + riga sotto
	for _, key := range []string{"nacionalidad", "luogo_nascita", "cargo", "regionale", "tipo", "paese"} {
		lb, ok := vizFindLabel(lines, key)
		if !ok {
			continue
		}
		if v := vizValueBelow(lines, lb); v != "" {
			if key == "tipo" || key == "paese" {
				v = vizAlnum(v)
				if (key == "paese" && len(v) != 3) || len(v) > 3 || v == "" {
					continue
				}
			}
			out.Fields[key] = v
		}
	}
	// ripieghi geometrici: cargo sotto il livello, regionale sotto la data di nascita
	if _, ok := out.Fields["cargo"]; !ok && levelBox != nil {
		if v := vizValueBelow(lines, *levelBox); vizLetters(v) >= 3 {
			out.Fields["cargo"] = v
		}
	}
	if _, ok := out.Fields["regionale"]; !ok {
		if nb, ok := vizDateBox(dates, out, "data_nascita"); ok {
			if v := vizValueBelow(lines, nb); vizLetters(v) >= 3 {
				out.Fields["regionale"] = v
			}
		}
	}
	// ultimo ripiego per il numero libretto: ricerca nel testo
	if _, ok := out.Fields["numero_libretto"]; !ok {
		if v := firstMatch(allText(res2(lines)), `\bP[A-Z]?\d{7}\b`); v != "" {
			out.Fields["numero_libretto"] = v
		}
	}
}

func vizAlnumDigits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

// res2 ricostruisce un ocr.Result minimale dalle righe (per riusare allText).
func res2(lines []vizLine) *ocr.Result {
	r := &ocr.Result{}
	for _, l := range lines {
		r.Lines = append(r.Lines, ocr.Line{Text: l.text})
	}
	return r
}

func allText(res *ocr.Result) string {
	var b strings.Builder
	for _, l := range res.Lines {
		b.WriteString(l.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

// vizDedupDates elimina i riquadri di data duplicati (stesso testo letto da sequenze diverse di parole):
// tiene, tra riquadri molto sovrapposti, quello con piu' parole... qui il piu' piccolo che contiene la data.
func vizDedupDates(ds *[]vizDateTok) {
	var out []vizDateTok
	for _, d := range *ds {
		dup := false
		for i, o := range out {
			if absf(d.box.cy()-o.box.cy()) < 0.5*maxf(d.box.h, o.box.h) &&
				d.box.x < o.box.right() && o.box.x < d.box.right() {
				dup = true
				if d.box.w < o.box.w { // piu' stretto = meno rumore attorno
					out[i] = d
				}
				break
			}
		}
		if !dup {
			out = append(out, d)
		}
	}
	*ds = out
}

// vizAssignDates assegna le date ai campi. Con tre date vale l'ordine cronologico; altrimenti si usano le
// etichette sopra le date (confronto approssimato) e la colonna (a destra = nascita).
func vizAssignDates(ds []vizDateTok, lines []vizLine, libBox *box, out *Result) {
	if len(ds) == 0 {
		return
	}
	type pick struct {
		d   vizDate
		box box
	}
	if len(ds) == 3 {
		// scelta delle letture: tutte le combinazioni con nascita < emissione < scadenza e anni plausibili
		var best []pick
		for _, a := range ds[0].cands {
			for _, b := range ds[1].cands {
				for _, c := range ds[2].cands {
					ps := []pick{{a, ds[0].box}, {b, ds[1].box}, {c, ds[2].box}}
					sort.Slice(ps, func(i, j int) bool { return ps[i].d.iso() < ps[j].d.iso() })
					if _, ok := vizPickDate([]vizDate{ps[0].d}, "data_nascita"); !ok {
						continue
					}
					if _, ok := vizPickDate([]vizDate{ps[1].d}, "data_emissione"); !ok {
						continue
					}
					if _, ok := vizPickDate([]vizDate{ps[2].d}, "data_scadenza"); !ok {
						continue
					}
					if best == nil {
						best = ps
					}
				}
			}
		}
		if best != nil {
			vizSetDate(out, "data_nascita", best[0].d)
			vizSetDate(out, "data_emissione", best[1].d)
			vizSetDate(out, "data_scadenza", best[2].d)
			return
		}
	}
	// etichetta piu' vicina sopra ogni data
	type scored struct {
		key string
		s   float64
		i   int
	}
	var all []scored
	for i, d := range ds {
		for _, key := range []string{"data_nascita", "data_emissione", "data_scadenza"} {
			s := 0.0
			for _, l := range lines {
				if l.bottom() <= d.box.y+d.box.h*0.3 && l.bottom() >= d.box.y-d.box.h*3 &&
					l.x < d.box.right() && l.right() > d.box.x-d.box.h*2 {
					s = maxf(s, vizLabelSim(l.text, key))
				}
			}
			all = append(all, scored{key, s, i})
		}
	}
	sort.Slice(all, func(a, b int) bool { return all[a].s > all[b].s })
	usedKey, usedTok := map[string]bool{}, map[int]bool{}
	for _, c := range all {
		if c.s < 0.45 || usedKey[c.key] || usedTok[c.i] {
			continue
		}
		if d, ok := vizPickDate(ds[c.i].cands, c.key); ok {
			vizSetDate(out, c.key, d)
			usedKey[c.key], usedTok[c.i] = true, true
		}
	}
	// date senza etichetta leggibile: a destra del libretto = nascita
	for i, d := range ds {
		if usedTok[i] || libBox == nil {
			continue
		}
		if d.box.cx() > libBox.x && !usedKey["data_nascita"] {
			if v, ok := vizPickDate(d.cands, "data_nascita"); ok {
				vizSetDate(out, "data_nascita", v)
				usedKey["data_nascita"], usedTok[i] = true, true
			}
		}
	}
}

// vizDateBox restituisce il riquadro della data gia' assegnata a key.
func vizDateBox(ds []vizDateTok, out *Result, key string) (box, bool) {
	iso := out.Fields[key+"_iso"]
	for _, d := range ds {
		for _, c := range d.cands {
			if c.iso() == iso {
				return d.box, true
			}
		}
	}
	return box{}, false
}

type vizNumTok struct {
	v string
	b box
}

// vizAssignNum9 distingue i due numeri a 9 cifre: il refrendo sta nella colonna destra (sotto
// l'identificazione), l'ID marittimo in basso a sinistra. Usa l'etichetta sopra il numero se leggibile,
// altrimenti la colonna rispetto al numero libretto (colonna destra).
func vizAssignNum9(nums []vizNumTok, libBox *box, lines []vizLine, out *Result) {
	for _, n := range nums {
		// etichetta piu' vicina sopra
		sRef, sId := 0.0, 0.0
		for _, l := range lines {
			if l.bottom() <= n.b.y+n.b.h*0.3 && l.bottom() >= n.b.y-n.b.h*3 &&
				l.x < n.b.right() && l.right() > n.b.x-n.b.h*2 {
				sRef = maxf(sRef, vizLabelSim(l.text, "refrendo"))
				sId = maxf(sId, vizLabelSim(l.text, "id_marino"))
			}
		}
		key := ""
		switch {
		case sRef >= 0.45 && sRef > sId+0.1:
			key = "refrendo"
		case sId >= 0.45 && sId > sRef+0.1:
			key = "id_marino"
		case libBox != nil && n.b.cx() >= libBox.x-libBox.w*0.5:
			key = "refrendo"
		case libBox != nil:
			key = "id_marino"
		}
		if key == "" {
			continue
		}
		if _, ok := out.Fields[key]; !ok {
			out.Fields[key] = n.v
		}
	}
	// un solo numero e nessun indizio (modello 2020: niente refrendo): e' l'ID marittimo
	if len(nums) == 1 && out.Fields["refrendo"] == "" && out.Fields["id_marino"] == "" && libBox == nil {
		out.Fields["id_marino"] = nums[0].v
	}
}

// vizFindLabel cerca la riga (o la sua prima parte) che somiglia di piu' all'etichetta del campo.
func vizFindLabel(lines []vizLine, key string) (box, bool) {
	best := 0.45
	var bb box
	found := false
	for _, l := range lines {
		// la riga intera, e le finestre di 1-4 parole (le etichette stanno spesso sulla stessa riga di altre)
		for n := 1; n <= min(4, len(l.words)); n++ {
			for i := 0; i+n <= len(l.words); i++ {
				var parts []string
				b := l.words[i].box
				for _, w := range l.words[i : i+n] {
					parts = append(parts, w.text)
					b = union(b, w.box)
				}
				if s := vizLabelSim(strings.Join(parts, " "), key); s > best {
					best, bb, found = s, b, true
				}
			}
		}
	}
	return bb, found
}

// vizValueBelow prende la riga di testo piu' vicina sotto il riquadro, nella stessa colonna.
func vizValueBelow(lines []vizLine, lb box) string {
	var best *vizLine
	for i := range lines {
		l := &lines[i]
		if l.cy() <= lb.cy()+lb.h*0.4 || l.y > lb.bottom()+lb.h*3.2 {
			continue
		}
		if l.cx() < lb.x-lb.h*1.5 && l.right() < lb.x {
			continue
		}
		// stessa colonna: la riga inizia entro la larghezza dell'etichetta (con margine)
		if l.x > lb.right()+maxf(lb.w, lb.h*8)*0.6 || l.right() < lb.x-lb.h {
			continue
		}
		if vizIsLabelish(l.text) {
			continue
		}
		if vizCleanText(l.text) == "" {
			continue
		}
		if best == nil || l.y < best.y {
			best = l
		}
	}
	if best == nil {
		return ""
	}
	// solo le parole della riga che stanno nella colonna
	var parts []string
	for _, w := range best.words {
		if w.x <= lb.right()+maxf(lb.w, lb.h*8) {
			parts = append(parts, w.text)
		}
	}
	return vizCleanText(strings.Join(parts, " "))
}

// ---- rifinitura e confronto con l'MRZ ----------------------------------------------------------------------

// finalizeVIZ rifinisce i campi dopo extractVIZ e crossCheck: normalizza date e numeri e confronta zona visiva
// e MRZ, aggiungendo avvisi in r.Warnings (senza riportare valori personali).
func finalizeVIZ(r *Result) { vizFinalize(r) }
