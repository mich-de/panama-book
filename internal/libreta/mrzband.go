package libreta

// File di proprieta' dell'agente "MRZ": lettura delle due righe MRZ.

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/mrz"
	"scanlibretto/internal/ocr"
)

// Altezze (px) a cui si portano le righe prima dell'OCR. Il motore di Windows e' capriccioso: la stessa riga
// puo' essere letta bene a una scala e restare vuota a un'altra, quindi si leggono piu' scale nella stessa
// chiamata (le immagini aggiuntive costano ~0,1 s ciascuna, l'avvio di PowerShell ~1 s) e si tiene la migliore.
var targetRowHeights = []float64{32, 45, 60}

const (
	maxOCRDim = 2500 // il motore OCR di Windows accetta al massimo 2600 px per lato (OcrEngine.MaxImageDimension)
	maxZoom   = 5
	mrzCells  = 44 // caratteri per riga
)

// pieceSpec descrive un pezzo di riga: celle [k0,k1) e, se prefix > 0, le prime prefix celle della stessa riga
// incollate davanti (dopo uno spazio). Il motore di Windows non restituisce nulla per un pezzo fatto quasi solo di
// '<' (tipico della coda: '<<<<<<48'): il prefisso, letto sicuramente, gli da' un contesto di testo vero.
type pieceSpec struct{ k0, k1, prefix int }

// pieceSpecs: pezzi con tagli diversi, cosi' un pezzo che il motore non legge e' quasi sempre coperto da un altro.
var pieceSpecs = func() []pieceSpec {
	var out []pieceSpec
	for _, cuts := range [][]int{{11, 22, 33}, {5, 16, 27, 38}, {15, 30}, {8, 19, 30, 37}} {
		edges := append(append([]int{0}, cuts...), mrzCells)
		for i := 0; i+1 < len(edges); i++ {
			out = append(out, pieceSpec{edges[i], edges[i+1], 0})
		}
	}
	for _, k := range []int{33, 35, 37, 39, 41, 42} { // code con prefisso
		out = append(out, pieceSpec{k, mrzCells, 5})
	}
	return out
}()

// readMRZBand legge le due righe MRZ da una immagine gia' orientata (testo dritto).
// Restituisce i dati, le correzioni applicate, un punteggio (0 = MRZ non trovato; piu' alto = piu' affidabile,
// vedi scoreMRZ; 14 = check validi, emittente PAN e riga 1 di 44 caratteri esatti, vedi scoreSure) e un errore solo
// per guasti tecnici (OCR non disponibile, ecc.).
//
// Procedura: binarizza (sparisce il guilloche chiaro), trova le righe di testo per proiezione dell'inchiostro e
// prende la coppia piu' in basso con altezza, larghezza e margine sinistro simili. Se non c'e' nessuna coppia, o la
// riga dei nomi e' capovolta (mrzUpsideDown), ritorna subito senza avviare l'OCR. Se le righe sono inclinate di
// 0,4 gradi o piu' (EstimateRowsSkew) le legge prima raddrizzate con uno scorrimento verticale delle colonne
// (straightenBand), poi, se non basta, cosi' come sono (readMRZRows).
//
// readMRZRows: ritaglia le due righe a piu' scale e le legge con una sola chiamata OCR. Se entrambe le righe restano
// vuote (stampa laser col tratto troppo spesso per il motore) prova una seconda chiamata con le varianti
// assottigliate (thinVariants). Una riga rimasta vuota o non valida (il motore ogni tanto non restituisce nulla per
// una riga intera, soprattutto la seconda) si rilegge a pezzi tagliati sulle celle dei caratteri; se sono vuote
// entrambe si passa direttamente ai pezzi (su alcune scansioni pulite il motore non legge mai le righe intere, ma
// legge bene i pezzi). findMRZ sotto resta come ripiego sull'OCR della pagina intera.
func readMRZBand(ctx context.Context, img image.Image, debugDir string) (*mrz.Data, []string, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, 0, err
	}
	begin := time.Now()
	b := img.Bounds()
	if b.Dx() < 300 || b.Dy() < 100 {
		return nil, nil, 0, nil
	}
	// foglio inserito storto: le righe inclinate oltre 2-3 gradi non si separano con la proiezione orizzontale.
	// Se l'immagine e' gia' stata raddrizzata da chi chiama la stima e' ~0 e costa ~0,1 s.
	img, _ = imgproc.Deskew(img)
	b = img.Bounds()
	bin := imgproc.Binarize(img)
	imgproc.ClearLines(bin)
	pairs := imgproc.MRZPairs(imgproc.TrimRowEdgeBars(bin, imgproc.TextRows(bin, 0.35)), b.Dx())
	if len(pairs) == 0 {
		return nil, nil, 0, nil // niente coppia di righe: nessun OCR (costerebbe ~1 s)
	}
	r1, r2 := pairs[0][0], pairs[0][1]
	if mrzUpsideDown(bin, r1) {
		return nil, nil, 0, nil // testo capovolto: nessun OCR, Extract prova la rotazione di 180 gradi
	}

	// inclinazione residua delle sole righe MRZ (Deskew sulla pagina intera corregge solo oltre DeskewMinAngle): con
	// 1 grado circa (25-30 px di dislivello sulla riga, quasi un'altezza di carattere) il motore OCR restituisce testo
	// vuoto anche su ritagli perfetti. Si leggono la banda raddrizzata e quella originale (il motore e' capriccioso:
	// su un facsimile con la stessa inclinazione legge solo l'originale), la raddrizzata per prima; le fasi costose
	// (letture a pezzi su piu' varianti) si fanno solo dopo che le letture a riga intera di entrambe sono fallite.
	var readers []*bandRead
	if tilt := imgproc.EstimateRowsSkew(bin, r1, r2); math.Abs(tilt) >= minRowTilt {
		if simg, sbin, s1, s2, ok := straightenBand(img, r1, r2, tilt); ok {
			sdbg := ""
			if debugDir != "" {
				sdbg = filepath.Join(debugDir, "dritta")
			}
			readers = append(readers, newBandRead(ctx, simg, sbin, s1, s2, sdbg))
		}
	}
	readers = append(readers, newBandRead(ctx, img, bin, r1, r2, debugDir))
	for _, r := range readers {
		if err := r.whole(); err != nil {
			return nil, nil, 0, err
		}
		if r.done() {
			return r.result()
		}
	}
	// tempo massimo per le letture a pezzi: maxBandTime per banda dall'inizio, diviso tra le bande ancora da provare
	limit := begin.Add(time.Duration(len(readers)) * maxBandTime)
	for i, r := range readers {
		if err := r.thinPieces(time.Now().Add(time.Until(limit) / time.Duration(len(readers)-i))); err != nil {
			return nil, nil, 0, err
		}
		if r.done() {
			return r.result()
		}
	}
	best := readers[0]
	for _, r := range readers[1:] {
		if r.score > best.score {
			best = r
		}
	}
	return best.result()
}

// minRowTilt: inclinazione (gradi) delle righe MRZ da cui si prova a raddrizzare la banda prima dell'OCR.
const minRowTilt = 0.4

// straightenBand ritaglia dall'immagine a colori la banda delle due righe MRZ (con un margine di due altezze di riga),
// la raddrizza di tilt gradi con imgproc.ShearRows (tilt positivo = righe che scendono a destra, vedi
// imgproc.EstimateRowsSkew), la binarizza e ricerca la coppia di righe. ok=false se dopo il raddrizzamento la coppia
// non si ritrova.
func straightenBand(img image.Image, r1, r2 imgproc.Row, tilt float64) (image.Image, *image.Gray, imgproc.Row, imgproc.Row, bool) {
	b := img.Bounds()
	rowH := max(1, (r1.Height()+r2.Height())/2)
	pad := 2 * rowH
	x0, x1 := max(0, min(r1.X0, r2.X0)-pad), min(b.Dx(), max(r1.X1, r2.X1)+pad)
	y0, y1 := max(0, r1.Y0-pad), min(b.Dy(), r2.Y1+pad)
	band := image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	draw.Draw(band, band.Bounds(), img, image.Pt(b.Min.X+x0, b.Min.Y+y0), draw.Src)
	rot := imgproc.ShearRows(band, tilt) // niente rotazione: l'interpolazione sfoca e ingrossa i tratti
	bin := imgproc.Binarize(rot)
	imgproc.ClearLines(bin)
	pairs := imgproc.MRZPairs(imgproc.TrimRowEdgeBars(bin, imgproc.TextRows(bin, 0.35)), rot.Bounds().Dx())
	if len(pairs) == 0 {
		return nil, nil, imgproc.Row{}, imgproc.Row{}, false
	}
	return rot, bin, pairs[0][0], pairs[0][1], true
}

// bandRead e' la lettura di una coppia di righe MRZ gia' individuate (r1, r2 sulla bitmap bin, ricavata da img). Lo
// stato (testi letti, voti dei pezzi, miglior lettura) si conserva tra le fasi whole e thinPieces, che il chiamante
// puo' alternare su piu' bande (raddrizzata e originale).
type bandRead struct {
	ctx      context.Context
	img      image.Image
	bin      *image.Gray
	r1, r2   imgproc.Row
	debugDir string

	lines    [2][]string // testi letti per ogni riga (interi, a pezzi, votati)
	d        mrz.Data    // miglior lettura
	notes    []string
	score    int
	variants [][2]*image.Gray // varianti del gruppo in cui una riga e' stata letta per intero
	good     [2]int           // indice della prima variante in cui la riga e' stata letta per intero
	votes    pieceVotes       // voti cumulati di tutti i passi a pezzi
	tried    int              // gruppi di varianti passati all'OCR
	overlong bool             // l'OCR ha letto una riga molto piu' lunga di 44 caratteri: non e' una riga MRZ
}

// maxExtraChars: oltre mrzCells+maxExtraChars caratteri letti da una sola riga la coppia non e' una MRZ (testo normale,
// per esempio una copertina o una pagina di certificato: l'OCR restituisce righe di 70-80 caratteri).
const maxExtraChars = 8

func newBandRead(ctx context.Context, img image.Image, bin *image.Gray, r1, r2 imgproc.Row, debugDir string) *bandRead {
	b := &bandRead{ctx: ctx, img: img, bin: bin, r1: r1, r2: r2, debugDir: debugDir, good: [2]int{-1, -1}}
	if debugDir != "" { // il testo OCR dei vari passi si accoda in mrz_ocr.txt: si riparte da vuoto
		if os.MkdirAll(debugDir, 0o755) == nil {
			_ = os.WriteFile(filepath.Join(debugDir, "mrz_ocr.txt"), nil, 0o644)
			debugNote(debugDir, fmt.Sprintf("spessore tratto/altezza riga: %.3f\n", strokeRatio(bin, r1, r2)))
		}
	}
	return b
}

func (b *bandRead) done() bool { return readingDone(&b.d, b.score) }

func (b *bandRead) result() (*mrz.Data, []string, int, error) { return finish(b.d, b.notes, b.score) }

// whole esegue i passi 1 e 2: righe intere a piu' scale (un gruppo di varianti per chiamata OCR) e, se una sola riga e'
// stata letta per intero, l'altra a pezzi tagliati dalla stessa variante.
func (b *bandRead) whole() error {
	// passo 1
	// con tratto spesso (stima solo euristica, vedi thickStrokeRatio) i due gruppi vanno nella stessa chiamata: di
	// solito normale non basta, e una seconda chiamata costerebbe ~1 s di avvio di PowerShell (tanto piu' se la
	// pagina e' capovolta e va ripetuta a un'altra rotazione)
	gens := []func() [][2]*image.Gray{
		func() [][2]*image.Gray { return rowVariants(b.img, b.bin, b.r1, b.r2) },
		func() [][2]*image.Gray { return thinVariants(b.img, b.r1, b.r2) },
	}
	if strokeRatio(b.bin, b.r1, b.r2) >= thickStrokeRatio {
		gens = []func() [][2]*image.Gray{func() [][2]*image.Gray {
			return append(rowVariants(b.img, b.bin, b.r1, b.r2), thinVariants(b.img, b.r1, b.r2)...)
		}}
	}
	for gi, gen := range gens {
		if b.good[0] >= 0 || b.good[1] >= 0 {
			break // una riga e' stata letta: l'altro gruppo non serve, si passa ai pezzi
		}
		vs := gen()
		if len(vs) == 0 {
			continue
		}
		b.tried++
		var imgs []*image.Gray
		var names []string
		for v, rows := range vs {
			for i, g := range rows {
				imgs = append(imgs, g)
				names = append(names, fmt.Sprintf("g%d_v%d_riga%d", gi+1, v+1, i+1))
			}
		}
		txt, err := ocrImages(b.ctx, imgs, names, b.debugDir)
		if err != nil {
			return err
		}
		for v := range vs {
			for i := 0; i < 2; i++ {
				t := txt[2*v+i]
				c := mrzClean(t)
				// la riga 1 (senza check digit) conta come letta per intero solo se comincia con "ISPAN" e ha esattamente
				// 44 caratteri: con 41-47 l'OCR ha perso o aggiunto simboli e fitLength puo' aver messo i '<' nel posto
				// sbagliato, scambiando un nome mutilato per uno giusto (visto: "CORDERO<SALINAS" letto "TEJEII").
				// Se non lo e' la riga 1 viene riletta a pezzi.
				// La riga 2 conta come letta se ha 41-47 caratteri e almeno minRow2Digits cifre (una MRZ ne ha ~24: un testo
				// qualunque di quella lunghezza, per esempio su una copertina, no).
				if n := len(c); b.good[i] < 0 && ((i == 1 && n >= mrzCells-3 && n <= mrzCells+3 && countDigits(c) >= minRow2Digits) || (i == 0 && n == mrzCells && strings.HasPrefix(c, "ISPAN"))) {
					b.good[i] = v
					b.variants = vs
				}
				if len(c) > mrzCells+maxExtraChars {
					b.overlong = true
				}
				b.lines[i] = append(b.lines[i], t)
			}
		}
		b.update()
		if b.done() {
			return nil
		}
	}

	// passo 2: se una riga e' stata letta per intero e l'altra no, la MRZ e' quella giusta e si rilegge a pezzi la
	// riga mancante. Se entrambe sono vuote (il motore OCR non legge le righe intere di alcune scansioni pulite e
	// dritte, anche se i ritagli sono perfetti: misurato su due scansioni TWAIN dello stesso libretto, mentre i
	// pezzi di 5-15 caratteri si leggono bene) non c'e' una variante da cui ritagliare e si passa al passo 3.
	if v := max(b.good[0], b.good[1]); v >= 0 {
		var grids [2]*cellGrid
		for i := 0; i < 2; i++ {
			if b.good[i] < 0 {
				if g, ok := newCellGrid(b.variants[v][i]); ok {
					grids[i] = g
				}
			}
		}
		return b.readPieces(grids, "")
	}
	return nil
}

// minRow2Digits: cifre minime in una riga 2 letta per intero (nelle MRZ di Panama ce ne sono circa 24).
const minRow2Digits = 10

func countDigits(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			n++
		}
	}
	return n
}

// update aggiorna la miglior lettura con tutti i testi raccolti finora.
func (b *bandRead) update() {
	if d, n, s := bestReading(b.lines[0], b.lines[1]); s > b.score {
		b.d, b.notes, b.score = d, n, s
	}
}

// readPieces legge a pezzi le righe per cui c'e' una griglia (grids[i] = griglia della riga i) e aggiorna la lettura.
func (b *bandRead) readPieces(grids [2]*cellGrid, tag string) error {
	var pieceImgs []*image.Gray
	var pieceNames []string
	var slots []pieceSlot
	for i := 0; i < 2; i++ {
		if grids[i] == nil {
			continue
		}
		im, nm, sl := grids[i].pieces(i, tag)
		pieceImgs, pieceNames, slots = append(pieceImgs, im...), append(pieceNames, nm...), append(slots, sl...)
	}
	if len(pieceImgs) == 0 {
		return nil
	}
	ptxt, err := ocrImages(b.ctx, pieceImgs, pieceNames, b.debugDir)
	if err != nil {
		return err
	}
	var own pieceVotes // voti di questa sola variante
	own.add(slots, ptxt)
	b.votes.add(slots, ptxt)
	for i := 0; i < 2; i++ {
		if grids[i] == nil {
			continue
		}
		for _, pv := range []*pieceVotes{&own, &b.votes} {
			filled := pv[i] // copia: i voti dedotti dalla forma non entrano nei voti cumulati
			fillFiller(grids[i], &filled)
			if t, ok := voteLine(filled); ok {
				b.lines[i] = append(b.lines[i], t)
			}
		}
	}
	b.update()
	return nil
}

// thinPieces esegue il passo 3: ancora niente di valido, i pezzi si ritagliano da varianti assottigliate (la riga 2
// sempre, la 1 solo se non e' stata letta per intero), una chiamata OCR per variante, finche' i check non tornano o
// arriva la scadenza deadline (si controlla prima di ogni chiamata OCR, che dura ~2 s).
func (b *bandRead) thinPieces(deadline time.Time) error {
	if b.tried == 0 || (b.good[0] < 0 && b.good[1] < 0 && b.overlong) {
		return nil
	}
	g1, g2 := imgproc.GrayCrop(b.img, b.r1, 0, 0), imgproc.GrayCrop(b.img, b.r2, 0, 0)
	for k, tg := range pieceTargets {
		if b.done() {
			break
		}
		if time.Until(deadline) < 2*time.Second {
			break // un'altra chiamata (~2 s) sforerebbe il tempo massimo
		}
		if k >= 1 && b.good[0] < 0 && b.good[1] < 0 && b.votes.dateDigits() < minDateDigits {
			break // nemmeno i pezzi mostrano le date di una MRZ: non e' una MRZ (o e' illeggibile), non si insiste
		}
		var grids [2]*cellGrid
		for i, g := range [2]*image.Gray{g1, g2} {
			if i == 0 && b.good[0] >= 0 {
				continue
			}
			im, _ := imgproc.ThinToInk(g, tg.erode, tg.ink)
			if cg, ok := newCellGrid(imgproc.PadWhite(im, 30)); ok {
				grids[i] = cg
			}
		}
		if err := b.readPieces(grids, fmt.Sprintf("_e%d_%02.0f", tg.erode, tg.ink*100)); err != nil {
			return err
		}
	}
	return nil
}

// readingDone: lettura da accettare senza cercare oltre. I check digit da soli non bastano: la riga 1 (i nomi) non ne
// ha, e una riga 1 letta male (caratteri persi, '<' aggiunti per tornare a 44) non si vede se la riga 2 e' giusta. Il
// codice stato PAN subito dopo "IS" e' l'unico controllo che la riga 1 abbia, quindi lo si pretende (tutti i libretti
// di Panama lo hanno); se manca si prova ancora, a pezzi, e alla fine si tiene la lettura migliore.
func readingDone(d *mrz.Data, score int) bool {
	return d != nil && d.Checks.AllOK() && d.Issuer == "PAN" && score >= scoreSure
}

// scoreSure: punteggio di una lettura completa (scoreMRZ al massimo) la cui riga 1 e' arrivata a 44 caratteri esatti
// senza aggiustamenti (bestReading aggiunge 1 al punteggio in quel caso).
const scoreSure = 14

// pieceSlot dice a quale riga e a quali celle appartiene un pezzo ritagliato.
type pieceSlot struct {
	row  int
	spec pieceSpec
}

// pieces ritaglia tutti i pezzi di pieceSpecs dalla griglia della riga row (0 o 1); tag distingue i nomi di debug.
func (c *cellGrid) pieces(row int, tag string) ([]*image.Gray, []string, []pieceSlot) {
	var imgs []*image.Gray
	var names []string
	var slots []pieceSlot
	for k, sp := range pieceSpecs {
		imgs = append(imgs, c.crop(sp))
		names = append(names, fmt.Sprintf("pezzo%s_riga%d_%02d_%d-%d_p%d", tag, row+1, k, sp.k0, sp.k1, sp.prefix))
		slots = append(slots, pieceSlot{row, sp})
	}
	return imgs, names, slots
}

// minDateDigits: cifre (su 14 celle) che i pezzi della prima variante devono aver letto nei campi data della riga 2
// (nascita e scadenza, sempre numerici in una MRZ), altrimenti, con entrambe le righe intere vuote, si smette di
// provare: la coppia di righe non e' una MRZ (testo normale come una copertina, o immagine illeggibile).
const minDateDigits = 6

// dateDigits conta le celle dei campi data della riga 2 (posizioni 13-19 e 21-27) lette come cifra.
func (pv *pieceVotes) dateDigits() int {
	n := 0
	for k := 13; k < 28; k++ {
		if k == 20 {
			continue // sesso
		}
		if c, ok := pieceTopVote(pv[1][k]); ok && c >= '0' && c <= '9' {
			n++
		}
	}
	return n
}

// pieceVotes raccoglie i caratteri letti nei pezzi: per riga e cella, quante volte e' stato letto ogni carattere.
type pieceVotes [2][mrzCells]map[byte]int

// add registra il testo OCR di ogni pezzo. Un pezzo letto con un numero di caratteri sbagliato si scarta: non si
// sa a quali celle assegnare i caratteri.
func (pv *pieceVotes) add(slots []pieceSlot, txt []string) {
	for k, sl := range slots {
		c := mrzClean(txt[k])
		sp := sl.spec
		cells := sp.k1 - sp.k0
		switch {
		case len(c) == sp.prefix+cells:
			c = c[sp.prefix:]
		case sp.prefix > 0 && len(c) == cells && cells != sp.prefix:
			// il motore ha letto solo il pezzo, senza prefisso
		default:
			continue
		}
		for j := 0; j < len(c); j++ {
			if pv[sl.row][sp.k0+j] == nil {
				pv[sl.row][sp.k0+j] = map[byte]int{}
			}
			pv[sl.row][sp.k0+j][c[j]]++
		}
	}
}

func finish(d mrz.Data, notes []string, score int) (*mrz.Data, []string, int, error) {
	if score == 0 {
		return nil, nil, 0, nil
	}
	return &d, notes, score, nil
}

// thinInkTargets: frazioni di inchiostro (rispetto all'area della riga) che le varianti assottigliate cercano di
// ottenere, per erosione di 1 o 2 px. Il motore OCR di Windows non legge nulla se il tratto e' troppo spesso (stampa
// laser a 300 dpi: l'inchiostro riempie i contro-spazi e fonde i simboli '<') e dopo l'erosione legge solo con una
// finestra stretta di inchiostro residuo (misurata: ~0,25-0,30 con erosione di 1 px); esito caotico per piccole
// variazioni, quindi si provano diversi valori vicini (le immagini costano ~0,05 s ciascuna).
var thinInkTargets = []thinTarget{
	{0, 0.22}, {1, 0.26}, {0, 0.26}, {1, 0.20}, {1, 0.30}, {2, 0.17},
}

// thinTarget e' una variante assottigliata: erosione (px) e frazione di inchiostro da ottenere dopo l'erosione.
type thinTarget struct {
	erode int
	ink   float64
}

// pieceTargets: varianti da cui ritagliare i pezzi (nell'ordine di prova) quando la lettura a riga intera non basta.
// Misurato su tre scansioni laser: a pezzi la lettura e' molto piu' robusta (quasi tutte queste varianti danno i
// check validi), mentre a riga intera ogni scansione ha la sua finestra di inchiostro.
var pieceTargets = []thinTarget{{1, 0.24}, {0, 0.22}, {1, 0.20}, {0, 0.18}, {1, 0.28}}

// maxBandTime: tempo massimo (indicativo: si controlla prima di ogni chiamata OCR) per le letture a pezzi di una banda.
const maxBandTime = 8 * time.Second

// debugNote accoda una riga a mrz_ocr.txt.
func debugNote(debugDir, s string) {
	if f, err := os.OpenFile(filepath.Join(debugDir, "mrz_ocr.txt"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		_, _ = f.WriteString(s)
		f.Close()
	}
}

// thickStrokeRatio: spessore/altezza oltre il quale si legge in una sola chiamata OCR con tutte le varianti. Solo
// una scelta di velocita' (misurata: nessuna correlazione con la leggibilita'); la correttezza non ne dipende.
const thickStrokeRatio = 0.185

// strokeRatio stima lo spessore relativo del tratto: spessore delle aste (vedi imgproc.StrokeWidth) delle due
// righe, in media, diviso la loro altezza. Misurato, non distingue le stampe che il motore OCR legge da quelle
// che non legge: oltre che nel debug lo usa solo la scelta di velocita' (una sola chiamata OCR per le varianti
// normali e assottigliate quando e' >= thickStrokeRatio), mai per decidere se una lettura e' valida.
func strokeRatio(bin *image.Gray, r1, r2 imgproc.Row) float64 {
	rowH := float64(r1.Height()+r2.Height()) / 2
	if rowH <= 0 {
		return 0
	}
	w := (imgproc.StrokeWidth(imgproc.CropRow(bin, r1, 2, 2)) + imgproc.StrokeWidth(imgproc.CropRow(bin, r2, 2, 2))) / 2
	return w / rowH
}

// thinVariants prepara le due righe assottigliate (alla risoluzione originale): per ogni coppia (erosione, frazione
// di inchiostro) rilegge il ritaglio in grigio con la soglia che da' quella frazione dopo l'erosione.
func thinVariants(img image.Image, r1, r2 imgproc.Row) [][2]*image.Gray {
	g1, g2 := imgproc.GrayCrop(img, r1, 0, 0), imgproc.GrayCrop(img, r2, 0, 0)
	var out [][2]*image.Gray
	for _, tg := range thinInkTargets {
		t1, _ := imgproc.ThinToInk(g1, tg.erode, tg.ink)
		t2, _ := imgproc.ThinToInk(g2, tg.erode, tg.ink)
		out = append(out, [2]*image.Gray{imgproc.PadWhite(t1, 30), imgproc.PadWhite(t2, 30)})
	}
	return out
}

// rowVariants prepara le due righe a scale diverse. Le righe si ingrandiscono ritagliando la fascia dall'immagine
// a colori, scalandola e ribinarizzando (i bordi restano nitidi), poi si ricercano le righe sull'immagine scalata.
func rowVariants(img image.Image, bin *image.Gray, r1, r2 imgproc.Row) [][2]*image.Gray {
	b := img.Bounds()
	rowH := max(1, (r1.Height()+r2.Height())/2)
	pad := rowH
	x0, x1 := max(0, min(r1.X0, r2.X0)-pad), min(b.Dx(), max(r1.X1, r2.X1)+pad)
	y0, y1 := max(0, r1.Y0-pad), min(b.Dy(), r2.Y1+pad)
	var band *image.RGBA // creata solo se serve ingrandire

	var out [][2]*image.Gray
	seen := map[int]bool{}
	for _, t := range targetRowHeights {
		zoom := min(maxZoom, t/float64(rowH))
		if zoom > 0.85 && zoom < 1.2 {
			zoom = 1
		}
		zoom = max(zoom, 0.5)
		key := int(zoom * 10)
		if seen[key] {
			continue
		}
		seen[key] = true
		if zoom == 1 {
			out = append(out, [2]*image.Gray{imgproc.CropRow(bin, r1, 6, 3), imgproc.CropRow(bin, r2, 6, 3)})
			continue
		}
		if band == nil {
			band = image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
			draw.Draw(band, band.Bounds(), img, image.Pt(b.Min.X+x0, b.Min.Y+y0), draw.Src)
		}
		bbin := imgproc.Binarize(imgproc.Scale(band, zoom))
		pairs := imgproc.MRZPairs(imgproc.TrimRowEdgeBars(bbin, imgproc.TextRows(bbin, 0.35)), bbin.Bounds().Dx())
		if len(pairs) == 0 {
			continue
		}
		out = append(out, [2]*image.Gray{imgproc.CropRow(bbin, pairs[0][0], 8, 4), imgproc.CropRow(bbin, pairs[0][1], 8, 4)})
	}
	return out
}

// cellGrid e' la griglia di 44 celle di passo costante di una riga MRZ (inchiostro scuro su fondo bianco).
type cellGrid struct {
	g      *image.Gray
	cols   []bool // colonne con inchiostro
	x0, x1 int    // estremi del testo
	pitch  float64
}

func newCellGrid(g *image.Gray) (*cellGrid, bool) {
	w, h := g.Bounds().Dx(), g.Bounds().Dy()
	c := &cellGrid{g: g, cols: make([]bool, w)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if g.Pix[y*g.Stride+x] == 0 {
				c.cols[x] = true
			}
		}
	}
	c.x0, c.x1 = imgproc.MainSpan(c.cols, h)
	if c.x1-c.x0 < 4*mrzCells {
		return nil, false
	}
	c.pitch = float64(c.x1-c.x0) / (mrzCells - 0.3)
	return c, true
}

// cut da' la colonna di taglio prima della cella k: il vuoto tra caratteri piu' vicino al taglio teorico.
func (c *cellGrid) cut(k int) int {
	switch {
	case k <= 0:
		return max(0, c.x0-2)
	case k >= mrzCells:
		return min(len(c.cols), c.x1+2)
	}
	t := c.x0 + int(float64(k)*c.pitch-0.15*c.pitch)
	for d := 0; d <= int(0.4*c.pitch); d++ {
		for _, x := range []int{t - d, t + d} {
			if x > c.x0 && x < c.x1 && !c.cols[x] {
				return x
			}
		}
	}
	return t
}

// crop ritaglia il pezzo (con margine bianco) descritto da sp.
func (c *cellGrid) crop(sp pieceSpec) *image.Gray {
	const margin = 30
	h := c.g.Bounds().Dy()
	type span struct{ a, b int }
	gap := int(0.25 * c.pitch)
	var spans []span
	if sp.prefix > 0 {
		spans = append(spans, span{c.cut(0), c.cut(sp.prefix)})
	}
	spans = append(spans, span{c.cut(sp.k0), c.cut(sp.k1)})
	total := margin
	for i, s := range spans {
		total += s.b - s.a
		if i > 0 {
			total += gap // distanza normale tra due caratteri
		}
	}
	out := image.NewGray(image.Rect(0, 0, total+margin, h))
	for i := range out.Pix {
		out.Pix[i] = 255
	}
	x := margin
	for i, s := range spans {
		if i > 0 {
			x += gap
		}
		for y := 0; y < h; y++ {
			copy(out.Pix[y*out.Stride+x:y*out.Stride+x+s.b-s.a], c.g.Pix[y*c.g.Stride+s.a:y*c.g.Stride+s.b])
		}
		x += s.b - s.a
	}
	return out
}

// Forma di una cella (carattere) della griglia: mappa dell'inchiostro sul suo riquadro, scalata a fillShapeNX x fillShapeNY, e
// larghezza relativa al passo. Serve a riconoscere i simboli '<' per somiglianza, senza il motore OCR (che non
// restituisce nulla per le serie di '<' e per i pezzi fatti quasi solo di '<').
const (
	fillShapeNX = 6
	fillShapeNY = 10
)

type fillShape struct {
	m [fillShapeNX * fillShapeNY]float64
	w float64
}

// fillShapeOf calcola la forma della cella k; ok=false se la cella non ha inchiostro.
func (c *cellGrid) fillShapeOf(k int) (fillShape, bool) {
	var s fillShape
	a, b := c.cut(k), c.cut(k+1)
	h := c.g.Bounds().Dy()
	x0, x1, y0, y1 := b, a-1, h, -1
	for y := 0; y < h; y++ {
		for x := a; x < b; x++ {
			if c.g.Pix[y*c.g.Stride+x] == 0 {
				x0, x1, y0, y1 = min(x0, x), max(x1, x), min(y0, y), max(y1, y)
			}
		}
	}
	if x1 < x0 || y1 < y0 {
		return s, false
	}
	bw, bh := x1-x0+1, y1-y0+1
	var cnt [fillShapeNX * fillShapeNY]float64
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			cell := ((y-y0)*fillShapeNY/bh)*fillShapeNX + (x-x0)*fillShapeNX/bw
			cnt[cell]++
			if c.g.Pix[y*c.g.Stride+x] == 0 {
				s.m[cell]++
			}
		}
	}
	for i := range s.m {
		if cnt[i] > 0 {
			s.m[i] /= cnt[i]
		}
	}
	s.w = float64(bw) / c.pitch
	return s, true
}

// dist e' la distanza tra due forme (0 = identiche).
func (s fillShape) dist(o fillShape) float64 {
	var d float64
	for i := range s.m {
		d += math.Abs(s.m[i] - o.m[i])
	}
	return d/float64(len(s.m)) + 2*math.Abs(s.w-o.w)
}

// fillerRunSpread misura quanto sono uniformi le celle [k0,k1) della griglia: distanza media tra le forme di tutte le
// coppie (0 = tutte uguali, come in una serie di '<'). ok=false se meno di 6 celle hanno inchiostro.
func (c *cellGrid) fillerRunSpread(k0, k1 int) (float64, bool) {
	var shapes []fillShape
	for k := k0; k < k1; k++ {
		if s, ok := c.fillShapeOf(k); ok {
			shapes = append(shapes, s)
		}
	}
	if len(shapes) < 6 {
		return 0, false
	}
	var sum float64
	n := 0
	for i := range shapes {
		for j := i + 1; j < len(shapes); j++ {
			sum += shapes[i].dist(shapes[j])
			n++
		}
	}
	return sum / float64(n), true
}

// mrzUpsideDown dice se la riga dei nomi r1 sembra capovolta di 180 gradi. Nella riga 1 l'ultima parte e' quasi
// sempre riempimento '<' (celle tutte uguali) mentre l'inizio e' "ISPAN" + lettere: se invece le celle a sinistra sono
// uniformi e quelle a destra no, il testo e' capovolto e leggerlo con l'OCR e' tempo perso (il chiamante prova poi la
// rotazione di 180 gradi). Misurato su 22 scansioni e facsimili in entrambi i versi: dritte sinistra >= 0,52 e destra
// <= 0,20; capovolte sinistra <= 0,38 e destra >= 0,45. Nei casi dubbi (nome lunghissimo, riga illeggibile) risponde
// false, cioe' si legge comunque.
func mrzUpsideDown(bin *image.Gray, r1 imgproc.Row) bool {
	grid, ok := newCellGrid(imgproc.CropRow(bin, r1, 6, 3))
	if !ok {
		return false
	}
	l, lok := grid.fillerRunSpread(0, 8)
	r, rok := grid.fillerRunSpread(mrzCells-8, mrzCells)
	return lok && rok && l < 0.43 && r > 0.40 && r-l > 0.10
}

// pieceTopVote restituisce il carattere piu' votato della cella (a parita', il minore), ok=false se non ci sono voti.
func pieceTopVote(m map[byte]int) (byte, bool) {
	var out byte
	bestN := 0
	for c, n := range m {
		if n > bestN || (n == bestN && c < out) {
			out, bestN = c, n
		}
	}
	return out, bestN > 0
}

// fillFiller da' un voto '<' alle celle che nessun pezzo ha letto e che somigliano alle celle gia' lette come '<'.
// Il motore OCR di Windows non restituisce nulla per i pezzi fatti di soli '<' (le lunghe serie di riempimento della
// riga 1 e del campo regionale), che altrimenti restano buchi e fanno scartare la riga (voteLine). Il modello e' la
// forma media delle celle votate '<' (servono almeno 3); la soglia segue la loro dispersione. Misurato su una riga
// reale: '<' a distanza 0,08-0,21 dal modello, ogni lettera a 0,36 o piu'.
func fillFiller(c *cellGrid, votes *[mrzCells]map[byte]int) {
	var tpl []fillShape
	for k := range votes {
		if ch, ok := pieceTopVote(votes[k]); ok && ch == '<' {
			if s, ok := c.fillShapeOf(k); ok {
				tpl = append(tpl, s)
			}
		}
	}
	if len(tpl) < 3 {
		return
	}
	var avg fillShape
	for _, s := range tpl {
		for i := range s.m {
			avg.m[i] += s.m[i] / float64(len(tpl))
		}
		avg.w += s.w / float64(len(tpl))
	}
	ds := make([]float64, len(tpl))
	for i, s := range tpl {
		ds[i] = s.dist(avg)
	}
	sort.Float64s(ds)
	tau := math.Min(0.30, math.Max(0.18, 2.5*ds[len(ds)/2]))
	for k := range votes {
		if len(votes[k]) > 0 {
			continue
		}
		if s, ok := c.fillShapeOf(k); ok && s.dist(avg) <= tau {
			votes[k] = map[byte]int{'<': 1}
		}
	}
}

// voteLine assembla una riga a maggioranza dai caratteri letti nei pezzi. Le celle mai lette diventano '<'
// (nel riempimento e' il carattere piu' probabile); se ne mancano troppe la riga non e' utilizzabile.
func voteLine(votes [mrzCells]map[byte]int) (string, bool) {
	out := make([]byte, mrzCells)
	missing := 0
	for i, m := range votes {
		c, ok := pieceTopVote(m)
		if !ok {
			out[i] = '<'
			missing++
			continue
		}
		out[i] = c
	}
	return string(out), missing <= 6
}

// ocrImages salva le immagini (in debugDir, o in una cartella temporanea) e le legge con una sola chiamata OCR;
// restituisce per ognuna il testo (le righe riconosciute concatenate).
func ocrImages(ctx context.Context, imgs []*image.Gray, names []string, debugDir string) ([]string, error) {
	dir := debugDir
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	} else {
		tmp, err := os.MkdirTemp("", "mrzband")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmp)
		dir = tmp
	}
	paths := make([]string, len(imgs))
	for i, g := range imgs {
		paths[i] = filepath.Join(dir, "mrz_"+names[i]+".png")
		if g.Bounds().Dx() > maxOCRDim || g.Bounds().Dy() > maxOCRDim {
			// il motore rifiuta (e fa fallire l'intera chiamata) le immagini oltre MaxImageDimension: al loro posto
			// va un foglio bianco, che da' testo vuoto
			g = imgproc.PadWhite(image.NewGray(image.Rect(0, 0, 1, 1)), 10)
		}
		if err := imgproc.SavePNG(g, paths[i]); err != nil {
			return nil, err
		}
	}
	t0 := time.Now()
	res, err := ocr.RecognizeMany(ctx, paths)
	if err != nil && ctx.Err() == nil {
		// fallimenti sporadici di PowerShell (visto: exit 0xc0400065, probabile contesa con altri processi): un tentativo
		time.Sleep(300 * time.Millisecond)
		res, err = ocr.RecognizeMany(ctx, paths)
	}
	if err != nil {
		return nil, err
	}
	if debugDir != "" {
		debugNote(debugDir, fmt.Sprintf("-- OCR di %d immagini: %.2f s\n", len(paths), time.Since(t0).Seconds()))
	}
	texts := make([]string, len(res))
	var dump strings.Builder
	for k, r := range res {
		var sb strings.Builder
		for _, l := range r.Lines {
			sb.WriteString(l.Text)
		}
		texts[k] = sb.String()
		fmt.Fprintf(&dump, "%s (%d car): %s\n", names[k], len(mrzClean(texts[k])), texts[k])
	}
	if debugDir != "" {
		if f, err := os.OpenFile(filepath.Join(debugDir, "mrz_ocr.txt"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			_, _ = f.WriteString(dump.String())
			f.Close()
		}
	}
	return texts, nil
}

// bestReading prova tutte le combinazioni di riga 1 e riga 2 lette alle varie scale e tiene quella col punteggio
// piu' alto (si ferma alla prima con tutti i check digit validi). Score 0 se nessuna combinazione e' una MRZ.
func bestReading(lines1, lines2 []string) (mrz.Data, []string, int) {
	var l1s, l2s []string
	// la riga 1 (i nomi) non ha check digit che la validino: si preferiscono le letture che cominciano con "ISPAN" e
	// quelle di lunghezza esatta (nessun '<' aggiunto o tolto da fitLength), e queste ultime valgono un punto in piu'
	rank := map[string]int{}
	exact := map[string]bool{}
	for _, t := range lines1 {
		raw := mrzClean(t)
		if c := fitLength(raw); strings.Count(c, "<") >= 2 { // la riga dei nomi ha molti '<'
			l1s = appendUnique(l1s, c)
			r := 0
			if strings.HasPrefix(c, "ISPAN") {
				r += 2
			}
			if len(raw) == mrzCells {
				r++
				exact[c] = true
			}
			rank[c] = max(rank[c], r)
		}
	}
	sort.SliceStable(l1s, func(i, j int) bool { return rank[l1s[i]] > rank[l1s[j]] })
	for _, t := range lines2 {
		if c := fitLength(mrzClean(t)); c != "" {
			l2s = appendUnique(l2s, c)
		}
	}
	var best mrz.Data
	var bestNotes []string
	bestScore := 0
	for _, l1 := range l1s {
		for _, l2 := range l2s {
			d, notes, err := mrz.Repair(l1, l2)
			if err != nil {
				continue
			}
			sc := scoreMRZ(d)
			if exact[l1] {
				sc++
			}
			if sc > bestScore {
				best, bestNotes, bestScore = d, notes, sc
				if readingDone(&best, sc) {
					return best, bestNotes, bestScore
				}
			}
		}
	}
	return best, bestNotes, bestScore
}

func appendUnique(l []string, s string) []string {
	for _, x := range l {
		if x == s {
			return l
		}
	}
	return append(l, s)
}

// fitLength riporta a 44 caratteri una riga di 41-47 caratteri aggiungendo o togliendo '<' nella serie di '<'
// piu' lunga (quasi sempre e' il riempimento: li' l'OCR perde o aggiunge simboli, senza spostare i campi prima).
func fitLength(s string) string {
	const want = 44
	diff := want - len(s)
	if diff == 0 || diff < -3 || diff > 3 {
		return s
	}
	bestStart, bestLen := -1, 0
	for i := 0; i < len(s); {
		if s[i] != '<' {
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] == '<' {
			j++
		}
		if j-i > bestLen {
			bestStart, bestLen = i, j-i
		}
		i = j
	}
	if bestLen < 2 || (diff < 0 && bestLen+diff < 1) {
		return s
	}
	if diff > 0 {
		return s[:bestStart] + strings.Repeat("<", diff) + s[bestStart:]
	}
	return s[:bestStart] + s[bestStart-diff:]
}

// scoreMRZ da' un punteggio a una lettura: 1 se parsata, +2 per ogni check digit valido, +2 se l'emittente e' PAN.
func scoreMRZ(d mrz.Data) int {
	score := 1
	for _, ok := range []bool{d.Checks.DocNumber, d.Checks.Birth, d.Checks.Expiry, d.Checks.Optional, d.Checks.Composite} {
		if ok {
			score += 2
		}
	}
	if d.Issuer == "PAN" {
		score += 2
	}
	return score
}

func mrzClean(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '<':
			b.WriteRune(r)
		case strings.ContainsRune("«‹〈＜≪", r):
			b.WriteRune('<')
		}
	}
	return b.String()
}

// findMRZ cerca due righe consecutive con lunghezza vicina a 44 e le interpreta; restituisce un punteggio.
func findMRZ(res *ocr.Result, out *Result) int {
	type cand struct {
		clean string
		y     float64
	}
	var cs []cand
	for _, l := range res.Lines {
		c := mrzClean(l.Text)
		if len(c) >= 36 && len(c) <= 50 && (strings.Count(c, "<") >= 2) && len(l.Words) > 0 {
			cs = append(cs, cand{c, l.Words[0].Y})
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].y < cs[j].y })

	bestScore := 0
	for i := 0; i+1 < len(cs); i++ {
		d, notes, err := mrz.Repair(cs[i].clean, cs[i+1].clean)
		if err != nil {
			continue
		}
		score := scoreMRZ(d)
		if score > bestScore {
			bestScore = score
			dd := d
			out.MRZ = &dd
			out.MRZCorrections = notes
		}
	}
	return bestScore
}
