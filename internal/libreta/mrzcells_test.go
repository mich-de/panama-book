package libreta

// Prove del lettore MRZ a template (mrzcells*.go) senza scansioni ne' OCR: immagini sintetiche costruite con i glifi
// della libreria incorporata (solo bitmap di simboli, nessun dato personale).

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/mrz"
)

func TestTplRegistraIlLettore(t *testing.T) {
	if mrzCellsReader == nil {
		t.Fatal("mrzCellsReader non registrato: manca init() in mrzcells.go")
	}
}

// riquadro 40x40 con una barra verticale di spessore w (px) centrata in orizzontale a cx, lunga quanto il riquadro.
func tplBarra(cx, w int) []uint8 {
	fr := make([]uint8, tplFrame*tplFrame)
	for y := 0; y < tplFrame; y++ {
		for x := cx - w/2; x < cx-w/2+w; x++ {
			fr[y*tplFrame+x] = 255
		}
	}
	return fr
}

func TestTplReduceDimensioniEPosizione(t *testing.T) {
	v := tplReduce(tplBarra(20, 4))
	// la barra sta al centro: colonne ridotte 5-6 (di 12), righe 1..13
	var sum, left, right float32
	for j := 0; j < tplRedH; j++ {
		for i := 0; i < tplRedW; i++ {
			sum += v[j*tplRedW+i]
			if i < tplRedW/2 {
				left += v[j*tplRedW+i]
			} else {
				right += v[j*tplRedW+i]
			}
		}
	}
	if sum < 27 || sum > 33 { // area 4*30 px canonici nella zona usata = 120, ogni pixel ridotto ne copre 4: ~30
		t.Errorf("somma della copertura %.2f, attesa ~30", sum)
	}
	if math.Abs(float64(left-right)) > 0.3*float64(sum) {
		t.Errorf("barra centrata ma sbilanciata: sinistra %.2f destra %.2f", left, right)
	}
	for _, x := range v { // valori in 0..1
		if x < -1e-6 || x > 1+1e-6 {
			t.Fatalf("copertura fuori scala: %v", x)
		}
	}
	// una barra spostata di 6 px a destra cambia nettamente il vettore
	w := tplReduce(tplBarra(26, 4))
	var d float32
	for i := range v {
		d += (v[i] - w[i]) * (v[i] - w[i])
	}
	if d < 1 {
		t.Errorf("barre a 6 px di distanza troppo simili (distanza %.3f)", d)
	}
}

func TestTplShiftsEsattezza(t *testing.T) {
	var v tplVec
	v[7*tplRedW+5] = 1
	sh := tplShifts(&v)
	// ordine: dy -1..1, dx -1..1; la versione (dy,dx) legge il vettore di partenza in (riga+dy, colonna+dx): il pixel
	// (riga 7, colonna 5) compare in (7-dy, 5-dx)
	n := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if sh[n][(7-dy)*tplRedW+5-dx] != 1 {
				t.Errorf("spostamento (%d,%d): pixel non trovato", dy, dx)
			}
			n++
		}
	}
}

func TestTplStrokeWidthEThicken(t *testing.T) {
	// una barra che attraversa la zona usata ha solo bordi verticali: 2*area/perimetro e' la sua larghezza
	prev := 0.0
	for _, w := range []int{2, 4, 6} {
		got := tplStrokeWidth([][]uint8{tplBarra(20, w)})
		want := float64(w)
		if math.Abs(got-want) > 0.3 || got <= prev {
			t.Errorf("barra da %d px: stimato %.2f, atteso %.2f e crescente", w, got, want)
		}
		prev = got
	}
	// assottigliare una barra da 6 px di 1 px per lato la porta a 4 px; ispessire una da 2 la porta a 4
	w4 := tplStrokeWidth([][]uint8{tplBarra(20, 4)})
	thin := tplThicken(tplBarra(20, 6), -1)
	if got := tplStrokeWidth([][]uint8{thin}); math.Abs(got-w4) > 0.5 {
		t.Errorf("assottigliata: %.2f, attesa ~%.2f", got, w4)
	}
	thick := tplThicken(tplBarra(20, 2), 1)
	if got := tplStrokeWidth([][]uint8{thick}); math.Abs(got-w4) > 0.5 {
		t.Errorf("ispessita: %.2f, attesa ~%.2f", got, w4)
	}
	if tplThicknessRadius(tplStrokeTarget) != 0 || tplThicknessRadius(tplStrokeTarget+0.1) != 0 {
		t.Error("differenze trascurabili di spessore non vanno corrette")
	}
	if r := tplThicknessRadius(tplStrokeTarget + 2); r >= 0 {
		t.Errorf("tratto troppo spesso: raggio %.2f, atteso negativo", r)
	}
}

func TestTplLibreriaAndataERitorno(t *testing.T) {
	gl := []tplGlyph{{sym: 3, group: 0}, {sym: 36, group: 7}, {sym: 0, group: tplSynthetic}}
	for i := range gl {
		for j := range gl[i].v {
			gl[i].v[j] = float32((i*31+j*7)%256) / 255
		}
	}
	var buf bytes.Buffer
	if err := tplWriteLibrary(&buf, gl); err != nil {
		t.Fatal(err)
	}
	back, err := tplReadLibrary(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != len(gl) {
		t.Fatalf("glifi letti %d, attesi %d", len(back), len(gl))
	}
	for i := range gl {
		if back[i].sym != gl[i].sym || back[i].group != gl[i].group {
			t.Errorf("glifo %d: simbolo o libretto cambiati", i)
		}
		for j := range gl[i].v {
			if math.Abs(float64(back[i].v[j]-gl[i].v[j])) > 1.0/255 {
				t.Fatalf("glifo %d: valore %d cambiato oltre la quantizzazione", i, j)
			}
		}
	}
	if _, err := tplReadLibrary([]byte("non e' un file")); err == nil {
		t.Error("un file corrotto non deve essere accettato")
	}
}

func TestTplLibreriaIncorporata(t *testing.T) {
	lib := tplEmbedded()
	if len(lib.glyphs) < 300 {
		t.Fatalf("libreria incorporata troppo piccola: %d glifi (glyphs.bin rigenerato?)", len(lib.glyphs))
	}
	var mancanti []byte
	for s := 0; s < tplNSym; s++ {
		if len(lib.bySym[s]) == 0 {
			mancanti = append(mancanti, tplAlphabet[s])
		}
	}
	// ogni simbolo MRZ deve avere almeno un glifo (la 'W' e' sintetica: nessuna scansione del corpus ne ha una)
	if len(mancanti) > 0 {
		t.Errorf("simboli senza glifi: %q", mancanti)
	}
	for _, g := range lib.glyphs {
		for _, x := range g.v {
			if x < 0 || x > 1 {
				t.Fatal("copertura fuori scala nella libreria")
			}
		}
	}
	// i glifi sono bitmap, non testo: la libreria non porta nomi o numeri (controllo sul file)
	if bytes.Contains(tplGlyphsBin, []byte("PAN")) {
		t.Error("glyphs.bin contiene testo")
	}
}

func TestTplPrune(t *testing.T) {
	var gl []tplGlyph
	for i := 0; i < 50; i++ { // 50 glifi del simbolo 0: due gruppi distinti di quasi duplicati
		g := tplGlyph{sym: 0}
		for j := range g.v {
			if i < 25 {
				g.v[j] = 0.1
			} else {
				g.v[j] = 0.8
			}
		}
		g.v[0] += float32(i%25) * 0.001
		gl = append(gl, g)
	}
	gl = append(gl, tplGlyph{sym: 1}, tplGlyph{sym: 1}) // simbolo con pochi glifi: restano tutti
	out := tplPrune(gl, 4)
	var n0, n1 int
	var low, high bool
	for _, g := range out {
		switch g.sym {
		case 0:
			n0++
			low = low || g.v[5] < 0.5
			high = high || g.v[5] > 0.5
		case 1:
			n1++
		}
	}
	if n0 != 4 || n1 != 2 {
		t.Errorf("dopo la potatura: %d glifi del simbolo 0 (attesi 4) e %d del simbolo 1 (attesi 2)", n0, n1)
	}
	if !low || !high {
		t.Error("la potatura deve tenere glifi di entrambi i gruppi (campionamento del punto piu' lontano)")
	}
}

func TestTplLogProbsNormalizzate(t *testing.T) {
	var d [tplNSym]float32
	for s := range d {
		d[s] = float32(s) * 0.3
	}
	d[10] = tplInf // simbolo senza glifi
	lp := tplLogProbs(&d)
	var z float64
	for s, v := range lp {
		if s != 10 {
			z += math.Exp(v)
		}
	}
	if math.Abs(z-1) > 1e-9 {
		t.Errorf("le probabilita' sommano a %.6f", z)
	}
	if lp[10] > -50 {
		t.Error("un simbolo senza glifi non deve poter essere scelto")
	}
	if lp[0] < lp[1] {
		t.Error("la distanza minore deve avere la probabilita' maggiore")
	}
}

// ---------------------------------------------------------------------------------------------------------------
// Immagini sintetiche costruite con i glifi della libreria

// tplMedoids restituisce per ogni simbolo il glifo reale piu' vicino alla media del simbolo (indice in lib.glyphs), -1
// se il simbolo non ha glifi.
func tplMedoids(lib *tplLibrary) [tplNSym]int {
	var out [tplNSym]int
	for s := range out {
		out[s] = -1
		var mean tplVec
		idx := lib.bySym[s]
		for _, gi := range idx {
			for k := range mean {
				mean[k] += lib.glyphs[gi].v[k] / float32(len(idx))
			}
		}
		best := float32(math.MaxFloat32)
		for _, gi := range idx {
			var d float32
			for k := range mean {
				x := lib.glyphs[gi].v[k] - mean[k]
				d += x * x
			}
			if d < best {
				best, out[s] = d, int(gi)
			}
		}
	}
	return out
}

// tplRenderMRZ disegna le due righe su una pagina bianca: ogni carattere e' il glifo medio del simbolo, riportato alla
// scala dell'immagine (passo pitch px) con interpolazione bilineare. tilt in px di spostamento verticale per cella.
func tplRenderMRZ(lib *tplLibrary, lines [2]string, pitch, tilt float64) *image.Gray {
	med := tplMedoids(lib)
	w, h := int(pitch*(tplCells+6)), int(pitch*8)
	g := image.NewGray(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	x0 := 3 * pitch
	ys := [2]float64{float64(h) * 0.4, float64(h)*0.4 + 2.5*pitch} // le righe distano 2,5 passi (misurato sul corpus)
	s := pitch / tplPitch                                          // px dell'immagine per px canonico
	for r := 0; r < 2; r++ {
		for k := 0; k < tplCells; k++ {
			gi := med[tplSym(lines[r][k])]
			if gi < 0 {
				continue
			}
			v := &lib.glyphs[gi].v
			cx, cy := x0+(float64(k)+0.5)*pitch, ys[r]+tilt*float64(k)
			// zona del vettore ridotto: colonne canoniche 8..32, righe 5..35, centro del riquadro (20,20)
			for y := int(cy - 16*s); y <= int(cy+16*s); y++ {
				for x := int(cx - 13*s); x <= int(cx+13*s); x++ {
					if x < 0 || y < 0 || x >= w || y >= h {
						continue
					}
					u := 20 + (float64(x)+0.5-cx)/s // coordinate canoniche
					vv := 20 + (float64(y)+0.5-cy)/s
					fu, fv := (u-tplCropU0)/2-0.5, (vv-tplCropV0)/2-0.5 // coordinate nel vettore ridotto
					i0, j0 := int(math.Floor(fu)), int(math.Floor(fv))
					a, b := fu-float64(i0), fv-float64(j0)
					at := func(i, j int) float64 {
						if i < 0 || j < 0 || i >= tplRedW || j >= tplRedH {
							return 0
						}
						return float64(v[j*tplRedW+i])
					}
					cov := at(i0, j0)*(1-a)*(1-b) + at(i0+1, j0)*a*(1-b) + at(i0, j0+1)*(1-a)*b + at(i0+1, j0+1)*a*b
					px := &g.Pix[y*g.Stride+x]
					if val := uint8(255 - 255*math.Min(1, cov*1.1)); val < *px {
						*px = val
					}
				}
			}
		}
	}
	return g
}

func tplSyntheticPair() ([2]string, mrz.Data) {
	l1 := "ISPANROSSI<<MARIO<LUIGI" + string(bytes.Repeat([]byte("<"), tplCells-23))
	l2 := tplSyntheticLine2("PA1234567", "NIC", "850412", "M", "310315", "PANAMA<<<<<<<<")
	d, _ := mrz.Parse(l1, l2)
	return [2]string{l1, l2}, d
}

func TestReadMRZCellsSintetica(t *testing.T) {
	lib := tplEmbedded()
	lines, _ := tplSyntheticPair()
	for _, c := range []struct {
		pitch, tilt float64
	}{{30, 0}, {30, 0.35}, {22, 0}, {45, -0.3}} {
		img := tplRenderMRZ(lib, lines, c.pitch, c.tilt)
		d, notes, score := readMRZCells(img, "")
		if d == nil || score == 0 {
			t.Errorf("passo %.0f inclinazione %.2f: MRZ non letta", c.pitch, c.tilt)
			continue
		}
		if d.Line1 != lines[0] || d.Line2 != lines[1] || !d.Checks.AllOK() || score != 13 {
			t.Errorf("passo %.0f inclinazione %.2f: lette %q / %q, check %+v, punteggio %d, note %v", c.pitch, c.tilt, d.Line1, d.Line2, d.Checks, score, notes)
		}
	}
}

func TestReadMRZCellsCapovoltaOVuota(t *testing.T) {
	lib := tplEmbedded()
	lines, _ := tplSyntheticPair()
	img := tplRenderMRZ(lib, lines, 30, 0)
	if d, _, score := readMRZCells(imgproc.Rotate(img, 180), ""); d != nil || score != 0 {
		t.Errorf("riga capovolta: attesa nessuna lettura, ottenuto punteggio %d", score)
	}
	// il testo e' dritto ma la pagina e' ruotata di 90 gradi: nessuna coppia di righe
	if d, _, score := readMRZCells(imgproc.Rotate(img, 90), ""); d != nil || score != 0 {
		t.Errorf("pagina ruotata di 90 gradi: attesa nessuna lettura, ottenuto %d", score)
	}
	// pagina bianca, immagine piccola, pagina con una sola riga
	bianca := image.NewGray(image.Rect(0, 0, 1500, 900))
	for i := range bianca.Pix {
		bianca.Pix[i] = 255
	}
	if d, notes, score := readMRZCells(bianca, ""); d != nil || notes != nil || score != 0 {
		t.Errorf("pagina bianca: %v %v %d", d, notes, score)
	}
	if d, _, score := readMRZCells(image.NewGray(image.Rect(0, 0, 100, 50)), ""); d != nil || score != 0 {
		t.Error("immagine piccola: attesa nessuna lettura")
	}
	una := image.NewGray(bianca.Bounds())
	copy(una.Pix, bianca.Pix)
	for y := 400; y < 436; y++ {
		for x := 100; x < 1400; x++ {
			if (x/5)%2 == 0 {
				una.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}
	if d, _, score := readMRZCells(una, ""); d != nil || score != 0 {
		t.Errorf("una sola riga: attesa nessuna lettura, ottenuto %d", score)
	}
}

// Due righe di barre a passo regolare (non e' testo): la griglia si adatta ma le celle non somigliano a nessun glifo.
func TestReadMRZCellsBarreNonSonoMRZ(t *testing.T) {
	g := image.NewGray(image.Rect(0, 0, 1500, 500))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	for _, y0 := range []int{200, 270} {
		for k := 0; k < tplCells; k++ {
			for y := y0; y < y0+40; y++ {
				for x := 0; x < 8; x++ {
					g.SetGray(100+k*30+10+x, y, color.Gray{Y: 0})
				}
			}
		}
	}
	if d, _, score := readMRZCells(g, ""); d != nil || score != 0 {
		t.Errorf("barre regolari: attesa nessuna lettura, ottenuto punteggio %d", score)
	}
}

func TestTplFitGridPassoNoto(t *testing.T) {
	const pitch, x0 = 21.7, 57.3
	g := image.NewGray(image.Rect(0, 0, 1200, 300))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	for _, y0 := range []int{100, 160} {
		for k := 0; k < tplCells; k++ {
			cx := x0 + (float64(k)+0.5)*pitch
			for y := y0; y < y0+24; y++ {
				for x := int(cx) - 5; x <= int(cx)+5; x++ {
					g.SetGray(x, y, color.Gray{Y: 0})
				}
			}
		}
	}
	rows := imgproc.TextRows(g, 0.35)
	if len(rows) != 2 {
		t.Fatalf("righe trovate %d, attese 2", len(rows))
	}
	xl, p, q, ok := tplFitGrid(g, rows[0], rows[1])
	if !ok {
		t.Fatal("griglia non adattata")
	}
	if math.Abs(p-pitch) > 0.15 {
		t.Errorf("passo %.3f, atteso %.1f", p, pitch)
	}
	if math.Abs(xl-x0) > 2 {
		t.Errorf("bordo sinistro %.2f, atteso ~%.1f", xl, x0)
	}
	if q > 0.1 {
		t.Errorf("qualita' %.3f: i confini devono cadere nei vuoti", q)
	}
}

func TestTplRowLineInclinazioneEAltezza(t *testing.T) {
	const pitch, slope, h = 30.0, 0.4, 32
	g := image.NewGray(image.Rect(0, 0, 1500, 300))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	for k := 0; k < tplCells; k++ {
		y0 := 120 + int(math.Round(slope*float64(k)))
		for y := y0; y < y0+h; y++ {
			for x := 60 + k*30 + 8; x < 60+k*30+22; x++ {
				g.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}
	rows := imgproc.TextRows(g, 0.35)
	if len(rows) == 0 {
		t.Fatal("riga non trovata")
	}
	a, b, hh, ok := tplRowLine(g, rows[0], 60, pitch)
	if !ok {
		t.Fatal("riga non misurata")
	}
	if math.Abs(b-slope) > 0.05 {
		t.Errorf("pendenza %.3f, attesa %.2f", b, slope)
	}
	if math.Abs(hh-h) > 2 {
		t.Errorf("altezza %.1f, attesa %d", hh, h)
	}
	if want := 120 + (h-1)/2.0; math.Abs(a-want) > 2 {
		t.Errorf("centro alla cella 0: %.1f, atteso ~%.1f", a, want)
	}
}

func TestTplNoteSegnalanoCorrezioniEIncertezze(t *testing.T) {
	lines, _ := tplSyntheticPair()
	var rd tplReading
	rd.line = lines
	for r := 0; r < 2; r++ {
		rd.raw[r] = lines[r]
		for k := 0; k < tplCells; k++ {
			rd.lp[r][k][tplSym(lines[r][k])] = -0.01
		}
	}
	rd.raw[1] = lines[1][:4] + "O" + lines[1][5:] // il simbolo piu' vicino era una O, i vincoli hanno scelto la cifra
	rd.dmin[0][9] = tplFarDist + 0.5              // cella lontana da ogni glifo
	notes := tplNotes(&rd)
	var corr, inc bool
	for _, n := range notes {
		corr = corr || n == "riga2 pos 5: O→"+string(lines[1][4])+" (vincoli MRZ del lettore a template)"
		inc = inc || n == "riga1 pos 10: lettura incerta ("+string(lines[0][9])+")"
	}
	if !corr || !inc || len(notes) != 2 {
		t.Errorf("note inattese: %q", notes)
	}
}

// Un'immagine che non parte da (0,0) (sotto-immagine di una pagina piu' grande) si legge come quella intera.
func TestReadMRZCellsSottoImmagine(t *testing.T) {
	lib := tplEmbedded()
	lines, _ := tplSyntheticPair()
	img := tplRenderMRZ(lib, lines, 30, 0)
	big := image.NewGray(image.Rect(0, 0, img.Bounds().Dx()+400, img.Bounds().Dy()+300))
	for i := range big.Pix {
		big.Pix[i] = 255
	}
	for y := 0; y < img.Bounds().Dy(); y++ {
		copy(big.Pix[(y+120)*big.Stride+210:], img.Pix[y*img.Stride:y*img.Stride+img.Bounds().Dx()])
	}
	sub := big.SubImage(image.Rect(150, 100, 150+img.Bounds().Dx()+200, 100+img.Bounds().Dy()+150))
	d, _, score := readMRZCells(sub, "")
	if d == nil || score != 13 || d.Line1 != lines[0] || d.Line2 != lines[1] {
		t.Errorf("sotto-immagine: punteggio %d, dati %+v", score, d)
	}
}

// Con una cartella di debug il lettore salva i riquadri delle celle e il testo letto, solo li'.
func TestReadMRZCellsDebug(t *testing.T) {
	lib := tplEmbedded()
	lines, _ := tplSyntheticPair()
	dir := filepath.Join(t.TempDir(), "celle")
	if _, _, score := readMRZCells(tplRenderMRZ(lib, lines, 30, 0), dir); score == 0 {
		t.Fatal("MRZ non letta")
	}
	for _, f := range []string{"celle_riga1.png", "celle_riga2.png", "mrz_celle.txt"} {
		if st, err := os.Stat(filepath.Join(dir, f)); err != nil || st.Size() == 0 {
			t.Errorf("file di debug %s assente o vuoto", f)
		}
	}
}

// Qualunque immagine (rumore, tinta unita, bande) deve dare "non trovato" senza panic.
func TestReadMRZCellsRobustoAlRumore(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, sz := range [][2]int{{320, 120}, {1500, 400}, {900, 2500}} {
		g := image.NewGray(image.Rect(0, 0, sz[0], sz[1]))
		for i := range g.Pix {
			g.Pix[i] = uint8(rng.Intn(256))
		}
		if d, _, score := readMRZCells(g, ""); d != nil || score != 0 {
			t.Errorf("rumore %v: attesa nessuna lettura, ottenuto %d", sz, score)
		}
	}
	nera := image.NewGray(image.Rect(0, 0, 800, 400))
	if d, _, score := readMRZCells(nera, ""); d != nil || score != 0 {
		t.Errorf("immagine nera: ottenuto %d", score)
	}
	bande := image.NewGray(image.Rect(0, 0, 1200, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 1200; x++ {
			if (y/20)%2 == 0 {
				bande.Pix[y*bande.Stride+x] = 255
			}
		}
	}
	if d, _, score := readMRZCells(bande, ""); d != nil || score != 0 {
		t.Errorf("bande orizzontali: ottenuto %d", score)
	}
}

// Se la MRZ stampata ha un check digit sbagliato il lettore non deve "aggiustarla" inventando una cifra: la
// soluzione che rispetta i check costa una cella a evidenza forte, la probabilita' crolla e la lettura si scarta.
func TestReadMRZCellsNonInventaCheckDigit(t *testing.T) {
	lib := tplEmbedded()
	lines, _ := tplSyntheticPair()
	b := []byte(lines[1])
	b[9] = '0' + (b[9]-'0'+3)%10 // check del numero libretto sbagliato
	img := tplRenderMRZ(lib, [2]string{lines[0], string(b)}, 30, 0)
	rd, _ := tplReadImage(img, lib, "")
	t.Logf("costo dei vincoli riga1 %.2f riga2 %.2f, probabilita' %.3f/%.3f, mediana %.2f", rd.cost[0], rd.cost[1], rd.post[0], rd.post[1], rd.med)
	if d, _, score := readMRZCells(img, ""); d != nil || score != 0 {
		t.Errorf("check digit stampato sbagliato: attesa nessuna lettura, ottenuto punteggio %d (%v)", score, d)
	}
}

func TestTplNoteLetteraSoloSintetica(t *testing.T) {
	lines, _ := tplSyntheticPair()
	var rd tplReading
	rd.line = lines
	rd.raw = lines
	for r := 0; r < 2; r++ {
		for k := 0; k < tplCells; k++ {
			rd.lp[r][k][tplSym(lines[r][k])] = -0.01
		}
	}
	rd.synth[0][6] = true // una cella di riga 1 scelta tra simboli con soli glifi sintetici
	notes := tplNotes(&rd)
	if len(notes) != 1 || !strings.Contains(notes[0], "riga1 pos 7") || !strings.Contains(notes[0], "sintetico") {
		t.Errorf("note inattese: %q", notes)
	}
	// la libreria incorporata: la W e' l'unico simbolo solo sintetico
	lib := tplEmbedded()
	for s := 0; s < tplNSym; s++ {
		if want := tplAlphabet[s] == 'W'; lib.onlySynth[s] != want {
			t.Errorf("simbolo %c: onlySynth=%v, atteso %v", tplAlphabet[s], lib.onlySynth[s], want)
		}
	}
}
