package libreta

import (
	"context"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"scanlibretto/internal/imgproc"
)

// rigaSintetica disegna una "riga di testo" come una barra scura con tacche, abbastanza
// simile a una riga di MRZ per la proiezione dell'inchiostro.
func rigaSintetica(img *image.Gray, x0, x1, y0, y1 int) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if (x/3)%2 == 0 { // tratti verticali alternati
				img.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}
}

func paginaBianca(w, h int) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	return g
}

func TestRigheESelezioneCoppia(t *testing.T) {
	g := paginaBianca(1000, 600)
	rigaSintetica(g, 100, 900, 100, 130) // riga di testo piu' in alto (altro campo)
	rigaSintetica(g, 100, 900, 500, 530) // MRZ riga 1
	rigaSintetica(g, 102, 898, 550, 580) // MRZ riga 2
	rows := imgproc.TextRows(g, 0.35)
	if len(rows) != 3 {
		t.Fatalf("righe trovate %d, attese 3", len(rows))
	}
	pairs := imgproc.MRZPairs(rows, 1000)
	if len(pairs) == 0 {
		t.Fatal("coppia MRZ non trovata")
	}
	a, b := pairs[0][0], pairs[0][1]
	if a.Y0 != 500 || b.Y0 != 550 {
		t.Errorf("coppia scelta sbagliata: %v %v", a, b)
	}
}

func TestSegnoIsolatoNonAllargaLaRiga(t *testing.T) {
	g := paginaBianca(1000, 300)
	rigaSintetica(g, 300, 900, 100, 130)
	for y := 90; y < 140; y++ { // linea verticale lontana a sinistra (bordo del modulo)
		g.SetGray(20, y, color.Gray{Y: 0})
		g.SetGray(21, y, color.Gray{Y: 0})
	}
	rows := imgproc.TextRows(g, 0.35)
	if len(rows) != 1 {
		t.Fatalf("righe %d, attesa 1", len(rows))
	}
	if rows[0].X0 < 290 {
		t.Errorf("il segno isolato ha allargato la riga: X0=%d", rows[0].X0)
	}
}

func TestSenzaCoppiaNiente(t *testing.T) {
	g := paginaBianca(1000, 600)
	rigaSintetica(g, 100, 900, 100, 130)
	rigaSintetica(g, 100, 500, 300, 330) // larghezza diversa: non e' una coppia MRZ
	rigaSintetica(g, 100, 900, 450, 485) // altezza diversa
	if p := imgproc.MRZPairs(imgproc.TextRows(g, 0.35), 1000); len(p) != 0 {
		t.Errorf("trovate %d coppie, attese 0", len(p))
	}
}

func TestReadMRZBandSenzaCoppiaNonUsaOCR(t *testing.T) {
	// pagina bianca con una sola riga: deve tornare subito score 0 senza avviare PowerShell
	g := paginaBianca(1000, 600)
	rigaSintetica(g, 100, 900, 300, 330)
	d, notes, score, err := readMRZBand(context.Background(), g, "")
	if err != nil || d != nil || notes != nil || score != 0 {
		t.Errorf("atteso nil/0, ottenuto %v %v %d %v", d, notes, score, err)
	}
}

func TestFitLength(t *testing.T) {
	base := "ISPANROSSI<<MARIO" + strings.Repeat("<", 44-17)
	if len(base) != 44 {
		t.Fatal("base non lunga 44")
	}
	if got := fitLength(base); got != base {
		t.Errorf("una riga gia' di 44 non va toccata: %q", got)
	}
	// un '<' perso o in piu' nel riempimento viene ripristinato / tolto
	if got := fitLength(base[:43]); got != base {
		t.Errorf("43 -> %q", got)
	}
	if got := fitLength(base[:41]); got != base {
		t.Errorf("41 -> %q", got)
	}
	if got := fitLength(base + "<"); got != base {
		t.Errorf("45 -> %q", got)
	}
	// troppo lontana da 44: invariata
	if got := fitLength(base[:30]); got != base[:30] {
		t.Errorf("30 -> %q", got)
	}
}

func TestClearLinesTogliBordiScuri(t *testing.T) {
	g := paginaBianca(1000, 600)
	for y := 0; y < 600; y++ { // ombra verticale dello scanner, larga 12 px
		for x := 0; x < 12; x++ {
			g.SetGray(x, y, color.Gray{Y: 0})
		}
	}
	rigaSintetica(g, 100, 900, 500, 530)
	rigaSintetica(g, 100, 900, 550, 580)
	if n := len(imgproc.TextRows(g, 0.35)); n != 1 {
		t.Fatalf("senza pulizia ci si aspetta una sola riga fusa, trovate %d", n)
	}
	imgproc.ClearLines(g)
	if p := imgproc.MRZPairs(imgproc.TextRows(g, 0.35), 1000); len(p) != 1 {
		t.Errorf("dopo la pulizia attesa 1 coppia, trovate %d", len(p))
	}
}

// Un blocco scuro alto (la foto del titolare) sopra le righe MRZ rende "scure" le colonne che attraversano anche le
// righe: ClearLines deve cancellare solo le corse verticali lunghe, senza tagliare l'inizio delle righe di testo
// (regressione scana2: perdevano "ISPAN" e "PA012").
func TestClearLinesNonTagliaLeRigheSottoLaFoto(t *testing.T) {
	g := paginaBianca(1000, 600)
	for y := 50; y < 400; y++ { // foto: colonne con il 58% di inchiostro
		for x := 200; x < 320; x++ {
			g.SetGray(x, y, color.Gray{Y: 0})
		}
	}
	rigaSintetica(g, 100, 900, 500, 530)
	rigaSintetica(g, 100, 900, 550, 580)
	imgproc.ClearLines(g)
	p := imgproc.MRZPairs(imgproc.TextRows(g, 0.35), 1000)
	if len(p) != 1 {
		t.Fatalf("attesa 1 coppia, trovate %d", len(p))
	}
	if x0 := p[0][0].X0; x0 > 105 {
		t.Errorf("la riga e' stata tagliata a sinistra: X0=%d, atteso ~100", x0)
	}
}

// Due righe tipo MRZ inclinate di ~1,2 gradi: straightenBand le raddrizza e ritrova la coppia con righe piu' basse
// (inclinate occupano ~25 px in piu': e' cio' che fa restituire testo vuoto al motore OCR).
func TestStraightenBandRaddrizzaLeRighe(t *testing.T) {
	const w, h = 1500, 400
	g := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	tg := math.Tan(1.2 * math.Pi / 180)
	for _, y0 := range []int{150, 230} {
		for x := 100; x < 1400; x++ {
			if (x/14)%2 != 0 || x%14 > 5 {
				continue
			}
			dy := int(math.Round(float64(x-750) * tg))
			for y := y0; y < y0+36; y++ {
				g.SetRGBA(x, y+dy, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	bin := imgproc.Binarize(g)
	pairs := imgproc.MRZPairs(imgproc.TextRows(bin, 0.35), w)
	if len(pairs) != 1 {
		t.Fatalf("attesa 1 coppia sull'originale, trovate %d", len(pairs))
	}
	r1, r2 := pairs[0][0], pairs[0][1]
	tilt := imgproc.EstimateRowsSkew(bin, r1, r2)
	if math.Abs(tilt-1.2) > 0.15 {
		t.Fatalf("inclinazione stimata %.2f, attesa ~1,2", tilt)
	}
	_, _, s1, s2, ok := straightenBand(g, r1, r2, tilt)
	if !ok {
		t.Fatal("coppia non ritrovata dopo il raddrizzamento")
	}
	if s1.Height() > 42 || s2.Height() > 42 || r1.Height() < 55 {
		t.Errorf("altezze righe: prima %d,%d dopo %d,%d (attese ~36 dopo)", r1.Height(), r2.Height(), s1.Height(), s2.Height())
	}
}

// riga sintetica di 44 celle di 14 px: 'B' = barra piena, 'H' = due barre con traversa, '<' = chevron.
func rigaGlifi(testo string) *image.Gray {
	g := paginaBianca(44*14+100, 40)
	nero := color.Gray{Y: 0}
	for k := 0; k < mrzCells; k++ {
		x := 50 + k*14
		for y := 5; y < 35; y++ {
			switch testo[k] {
			case 'B':
				for dx := 2; dx < 8; dx++ {
					g.SetGray(x+dx, y, nero)
				}
			case 'H':
				for _, dx := range []int{2, 3, 8, 9} {
					g.SetGray(x+dx, y, nero)
				}
				if y >= 18 && y < 22 {
					for dx := 2; dx < 10; dx++ {
						g.SetGray(x+dx, y, nero)
					}
				}
			case 'T':
				for dx := 0; dx < 10; dx++ {
					if y < 10 {
						g.SetGray(x+dx, y, nero)
					}
				}
				if y >= 10 {
					g.SetGray(x+4, y, nero)
					g.SetGray(x+5, y, nero)
				}
			case 'L':
				g.SetGray(x+2, y, nero)
				g.SetGray(x+3, y, nero)
				if y >= 30 {
					for dx := 2; dx < 10; dx++ {
						g.SetGray(x+dx, y, nero)
					}
				}
			case '<':
				d := y - 20
				if d < 0 {
					d = -d
				}
				for dx := 0; dx < 3; dx++ {
					g.SetGray(x+2+d*8/15+dx, y, nero)
				}
			}
		}
	}
	return g
}

// fillFiller: le celle non lette che somigliano ai '<' gia' votati diventano '<'; quelle di forma diversa restano vuote.
func TestFillFillerRiconosceISimboliMinore(t *testing.T) {
	testo := "HBHBHBHB" + strings.Repeat("<", 8) + "BHBHBH" + strings.Repeat("<", 22)
	grid, ok := newCellGrid(rigaGlifi(testo))
	if !ok {
		t.Fatal("griglia non costruita")
	}
	var votes [mrzCells]map[byte]int
	for k := 0; k < mrzCells; k++ { // il motore ha letto le lettere e solo 4 dei 30 '<'
		switch {
		case testo[k] != '<' && k < 8, k == 8, k == 9, k == 10, k == 11:
			votes[k] = map[byte]int{testo[k]: 2}
		}
	}
	// le lettere centrali (14-21) non sono state lette
	fillFiller(grid, &votes)
	for k := 0; k < mrzCells; k++ {
		got, ok := pieceTopVote(votes[k])
		switch {
		case testo[k] == '<' && (!ok || got != '<'):
			t.Errorf("cella %d: '<' non riconosciuto (%q)", k, got)
		case testo[k] != '<' && k >= 16 && k < 22 && ok:
			t.Errorf("cella %d: lettera %c scambiata per %q", k, testo[k], got)
		}
	}
	// meno di 3 modelli: non si deduce nulla
	var poche [mrzCells]map[byte]int
	poche[8], poche[9] = map[byte]int{'<': 1}, map[byte]int{'<': 1}
	fillFiller(grid, &poche)
	if _, ok := pieceTopVote(poche[20]); ok {
		t.Error("con 2 soli modelli non si deve riempire")
	}
}

// mrzUpsideDown: riga dei nomi con il riempimento '<' a sinistra (capovolta) o a destra (dritta).
func TestMRZUpsideDown(t *testing.T) {
	lettere := "HBTLTBHLBTHL" + "TLHBLTBHLTHB" // 24 lettere diverse tra loro
	dritta := lettere[:5] + "BHTL" + strings.Repeat("<", 4) + lettere[5:11] + strings.Repeat("<", mrzCells-5-4-4-6)
	if len(dritta) != mrzCells {
		t.Fatalf("riga di prova lunga %d", len(dritta))
	}
	rev := func(s string) string {
		b := []byte(s)
		for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
			b[i], b[j] = b[j], b[i]
		}
		return string(b)
	}
	for _, c := range []struct {
		nome  string
		testo string
		want  bool
	}{{"dritta", dritta, false}, {"capovolta", rev(dritta), true}, {"nome lunghissimo", lettere + lettere[:20], false}} {
		g := rigaGlifi(c.testo)
		r := imgproc.TextRows(g, 0.35)
		if len(r) != 1 {
			t.Fatalf("%s: attesa 1 riga, trovate %d", c.nome, len(r))
		}
		if got := mrzUpsideDown(g, r[0]); got != c.want {
			t.Errorf("%s: mrzUpsideDown = %v, atteso %v", c.nome, got, c.want)
		}
	}
}

// riga a 44 "celle": un tratto scuro largo 6 px ogni 14 px.
func rigaACelle() *image.Gray {
	g := paginaBianca(44*14+100, 40)
	for k := 0; k < mrzCells; k++ {
		for y := 5; y < 35; y++ {
			for x := 0; x < 6; x++ {
				g.SetGray(50+k*14+x, y, color.Gray{Y: 0})
			}
		}
	}
	return g
}

func TestCellGridTagliNelleCelle(t *testing.T) {
	grid, ok := newCellGrid(rigaACelle())
	if !ok {
		t.Fatal("griglia non costruita")
	}
	// ogni pezzo deve contenere esattamente il numero di tratti (= celle) richiesto
	conta := func(g *image.Gray) int {
		n, in := 0, false
		for x := 0; x < g.Bounds().Dx(); x++ {
			ink := g.Pix[20*g.Stride+x] == 0
			if ink && !in {
				n++
			}
			in = ink
		}
		return n
	}
	for _, sp := range []pieceSpec{{0, 11, 0}, {11, 22, 0}, {33, 44, 0}, {38, 44, 0}, {42, 44, 5}} {
		got := conta(grid.crop(sp))
		if want := sp.prefix + sp.k1 - sp.k0; got != want {
			t.Errorf("pezzo %+v: %d celle, attese %d", sp, got, want)
		}
	}
}

func TestVoteLine(t *testing.T) {
	var votes [mrzCells]map[byte]int
	for i := range votes {
		votes[i] = map[byte]int{'A': 2, 'B': 1}
	}
	votes[3] = nil // cella mai letta: '<'
	s, ok := voteLine(votes)
	if !ok || len(s) != mrzCells || s[0] != 'A' || s[3] != '<' {
		t.Errorf("voteLine: %q %v", s, ok)
	}
	for i := 0; i < 10; i++ {
		votes[i] = nil
	}
	if _, ok := voteLine(votes); ok {
		t.Error("con troppe celle mancanti la riga non deve essere accettata")
	}
}

// rigaDiAste disegna n aste verticali di spessore w e altezza h, passo 4*w, a partire da (x0,y0).
func rigaDiAste(g *image.Gray, x0, y0, n, w, h int) {
	for k := 0; k < n; k++ {
		for y := y0; y < y0+h; y++ {
			for x := 0; x < w; x++ {
				g.SetGray(x0+k*4*w+x, y, color.Gray{Y: 0})
			}
		}
	}
}

func TestStrokeWidthSottileESpesso(t *testing.T) {
	for _, w := range []int{2, 7} {
		g := paginaBianca(600, 80)
		rigaDiAste(g, 20, 20, 40, w, 36)
		if got := imgproc.StrokeWidth(g); got != float64(w) {
			t.Errorf("aste da %d px: stimato %.0f", w, got)
		}
	}
	if got := imgproc.StrokeWidth(paginaBianca(50, 50)); got != 0 {
		t.Errorf("senza inchiostro atteso 0, ottenuto %.0f", got)
	}
}

func TestErodeAssottigliaIlTratto(t *testing.T) {
	g := paginaBianca(600, 80)
	rigaDiAste(g, 20, 20, 40, 7, 36) // aste spesse 7 px, alte 36
	for n, want := range map[int]float64{0: 7, 1: 5, 2: 3} {
		e := imgproc.Erode(g, n)
		if got := imgproc.StrokeWidth(e); got != want {
			t.Errorf("erosione %d: spessore %.0f, atteso %.0f", n, got, want)
		}
	}
	// l'originale non cambia e l'altezza cala di 2n px (il bordo alto e basso si erode)
	if imgproc.StrokeWidth(g) != 7 {
		t.Error("Erode ha modificato l'originale")
	}
	rows := imgproc.TextRows(imgproc.Erode(g, 2), 0.35)
	if len(rows) != 1 || rows[0].Height() != 36-4 {
		t.Errorf("riga erosa: %+v", rows)
	}
	// un'asta sottile (2 px) con erosione 1 sparisce solo se spessa 2: la mediana torna 0
	f := paginaBianca(600, 80)
	rigaDiAste(f, 20, 20, 40, 2, 36)
	if got := imgproc.StrokeWidth(imgproc.Erode(f, 1)); got != 0 {
		t.Errorf("aste da 2 px erose di 1: atteso 0, ottenuto %.0f", got)
	}
}

func TestThinToInkRaggiungeLaFrazione(t *testing.T) {
	// ritaglio in grigio: aste con bordo sfumato (centro 0, bordo 120), come una stampa antialias
	g := image.NewGray(image.Rect(0, 0, 400, 40))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	for k := 0; k < 30; k++ {
		for y := 4; y < 36; y++ {
			for x := 0; x < 8; x++ {
				v := uint8(0)
				if x == 0 || x == 7 {
					v = 120
				}
				g.SetGray(10+k*12+x, y, color.Gray{Y: v})
			}
		}
	}
	// col centro nero a soglia 0 l'inchiostro erosa e' ~0,225; con il bordo (120) incluso sale a ~0,34
	out, thr := imgproc.ThinToInk(g, 1, 0.20)
	if thr != 0 || imgproc.InkFraction(out) < 0.20 {
		t.Errorf("target 0,20: soglia %d, frazione %.3f", thr, imgproc.InkFraction(out))
	}
	out, thr = imgproc.ThinToInk(g, 1, 0.30)
	if thr < 120 || imgproc.InkFraction(out) < 0.30 {
		t.Errorf("target 0,30: soglia %d, frazione %.3f", thr, imgproc.InkFraction(out))
	}
}

func TestMRZPairsScartaRigheTroppoAlte(t *testing.T) {
	g := paginaBianca(1000, 900)
	rigaSintetica(g, 100, 900, 50, 450)  // 400 px di altezza: non e' una riga MRZ
	rigaSintetica(g, 100, 900, 460, 860) // idem
	if p := imgproc.MRZPairs(imgproc.TextRows(g, 0.35), 1000); len(p) != 0 {
		t.Errorf("trovate %d coppie, attese 0", len(p))
	}
}
