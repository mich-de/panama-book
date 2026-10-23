package libreta

// File di proprieta' dell'agente "MRZ template": lettura delle due righe MRZ per confronto di ogni cella con una
// libreria di glifi (bitmap), senza il motore OCR di Windows. Vedi docs/agenti/12-mrz-template.md.
//
// Questo file contiene la geometria (ritrovamento delle due righe, adattamento della griglia di 44 celle a passo
// costante, misura di inclinazione, altezza e spessore del tratto, ritaglio di ogni cella in un riquadro canonico con lo
// stesso passo e la stessa scala per ogni scansione) e la lettura: readMRZCells, registrata in mrzCellsReader.
// La libreria di glifi e' in mrzcells_glyph.go, la decodifica con i vincoli della MRZ in mrzcells_decode.go.

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/mrz"
)

const (
	tplCells = 44 // caratteri per riga MRZ (TD3)
	tplFrame = 40 // lato (px) del riquadro canonico di una cella
	tplPitch = 20 // larghezza di una cella (passo dei caratteri) nel riquadro canonico: il riquadro contiene 2 passi
)

// tplBand: le due righe MRZ di un'immagine, con la griglia dei caratteri e il riquadro canonico di ogni cella.
type tplBand struct {
	rows    [2]imgproc.Row
	xl      float64    // bordo sinistro della cella 0 (px dell'immagine)
	pitch   float64    // larghezza di una cella (px)
	yc      [2]float64 // centro verticale del testo di ciascuna riga, alla cella 0 (px dell'immagine)
	slope   [2]float64 // inclinazione della riga: spostamento verticale del testo per cella (px dell'immagine)
	height  [2]float64 // altezza dei caratteri (px dell'immagine, mediana sulle celle)
	stroke  float64    // spessore del tratto misurato nei riquadri canonici (px), prima della normalizzazione
	quality float64    // 0 = i confini tra le celle cadono tutti nei vuoti tra i caratteri; ~1 = griglia a caso
	// frames[riga][cella]: tplFrame x tplFrame byte, 0 = fondo, 255 = inchiostro pieno. Il riquadro e' centrato sulla
	// cella e sul centro verticale della riga, in scala tale che il passo valga tplPitch px.
	frames [2][tplCells][]uint8
}

// Aggancio a Extract (extract.go): dopo readMRZBand, se questo non trova l'MRZ o non tornano tutti i check digit,
// Extract chiama anche questo lettore sulla stessa immagine orientata e tiene il punteggio piu' alto.
func init() { mrzCellsReader = readMRZCells }

// tplFindRows ritrova la coppia di righe MRZ piu' in basso come fa readMRZBand: binarizza, toglie le linee scure, cerca
// le righe per proiezione. Restituisce l'immagine (eventualmente raddrizzata), la bitmap binarizzata e le coppie
// trovate (dal basso verso l'alto).
func tplFindRows(img image.Image) (image.Image, *image.Gray, [][2]imgproc.Row) {
	img, _ = imgproc.Deskew(img)
	b := img.Bounds()
	bin := imgproc.Binarize(img)
	imgproc.ClearLines(bin)
	return img, bin, imgproc.MRZPairs(imgproc.TrimRowEdgeBars(bin, imgproc.TextRows(bin, 0.35)), b.Dx())
}

// tplFitGrid cerca bordo sinistro della cella 0 e passo che mettono i 43 confini tra celle nei vuoti tra i
// caratteri (somma delle colonne di inchiostro delle due righe, sfumata con una gaussiana). Il bordo sinistro e'
// vincolato a stare accanto al primo inchiostro e quello destro dopo l'ultimo, cosi' la griglia non si puo'
// spostare di una cella. quality e' l'inchiostro medio sui confini diviso quello medio per colonna del testo.
func tplFitGrid(bin *image.Gray, r1, r2 imgproc.Row) (xl, pitch, quality float64, ok bool) {
	bw := bin.Bounds().Dx()
	xa, xb := min(r1.X0, r2.X0), max(r1.X1, r2.X1)
	w := xb - xa
	if w < 8*tplCells || w > 120*tplCells { // passo da 8 a 120 px (da ~80 a ~1200 dpi)
		return 0, 0, 0, false
	}
	p0 := float64(w) / (tplCells - 0.5)
	pad := int(2*p0) + 4
	lo, hi := xa-pad, xb+pad // profilo [lo,hi); fuori dall'immagine non c'e' inchiostro
	prof := make([]float64, hi-lo)
	var total float64
	for _, r := range [2]imgproc.Row{r1, r2} {
		for y := r.Y0; y < r.Y1; y++ {
			for x := max(r.X0, 0); x < min(r.X1, bw); x++ {
				if bin.Pix[y*bin.Stride+x] == 0 {
					prof[x-lo]++
					total++
				}
			}
		}
	}
	if total == 0 {
		return 0, 0, 0, false
	}
	// profilo sfumato (sigma 0,07 passi): la somma ai confini misura quanto inchiostro li attraversa
	sigma := math.Max(0.7, 0.07*p0)
	rad := int(3*sigma) + 1
	kern := make([]float64, 2*rad+1)
	var ks float64
	for i := range kern {
		d := float64(i - rad)
		kern[i] = math.Exp(-d * d / (2 * sigma * sigma))
		ks += kern[i]
	}
	sm := make([]float64, len(prof))
	for i := range sm {
		var s float64
		for j, kv := range kern {
			if x := i + j - rad; x >= 0 && x < len(prof) {
				s += kv * prof[x]
			}
		}
		sm[i] = s / ks
	}
	at := func(x float64) float64 { // sm interpolato linearmente, x in coordinate dell'immagine
		f := x - float64(lo)
		i := int(math.Floor(f))
		if i < 0 || i+1 >= len(sm) {
			return 0
		}
		t := f - float64(i)
		return sm[i]*(1-t) + sm[i+1]*t
	}
	best := math.Inf(1)
	for p := 0.955 * p0; p <= 1.045*p0; p += 0.0004 * p0 {
		for x := float64(xa) - 0.55*p; x <= float64(xa)+0.15*p; x += 0.25 {
			if e := x + tplCells*p; e < float64(xb)-0.1*p || e > float64(xb)+0.6*p {
				continue // il bordo destro sta dopo l'ultimo inchiostro, entro mezza cella
			}
			var c float64
			for k := 1; k < tplCells && c < best; k++ {
				c += at(x + float64(k)*p)
			}
			if c < best {
				best, xl, pitch = c, x, p
			}
		}
	}
	if math.IsInf(best, 1) {
		return 0, 0, 0, false
	}
	meanInk := total / float64(w) // inchiostro medio per colonna del testo (somma delle due righe)
	return xl, pitch, best / float64(tplCells-1) / meanInk, true
}

// tplSAT e' una tabella di somme cumulate (integral image) per medie di area con bordi frazionari.
type tplSAT struct {
	w, h int
	s    []float64 // (h+1) x (w+1): somma dei pixel con x'<x e y'<y
}

func newTplSAT(v []float32, w, h int) *tplSAT {
	t := &tplSAT{w: w, h: h, s: make([]float64, (w+1)*(h+1))}
	for y := 0; y < h; y++ {
		var run float64
		for x := 0; x < w; x++ {
			run += float64(v[y*w+x])
			t.s[(y+1)*(w+1)+x+1] = t.s[y*(w+1)+x+1] + run
		}
	}
	return t
}

// at e' l'integrale su [0,x) x [0,y), interpolato in modo bilineare; fuori dall'immagine vale il valore di bordo.
func (t *tplSAT) at(x, y float64) float64 {
	x = math.Min(math.Max(x, 0), float64(t.w))
	y = math.Min(math.Max(y, 0), float64(t.h))
	x0, y0 := int(x), int(y)
	fx, fy := x-float64(x0), y-float64(y0)
	x1, y1 := min(x0+1, t.w), min(y0+1, t.h)
	s := func(xx, yy int) float64 { return t.s[yy*(t.w+1)+xx] }
	return s(x0, y0)*(1-fx)*(1-fy) + s(x1, y0)*fx*(1-fy) + s(x0, y1)*(1-fx)*fy + s(x1, y1)*fx*fy
}

// mean e' la media dei pixel nel rettangolo [x0,x1) x [y0,y1) (frazionario); la parte fuori immagine conta come 0.
func (t *tplSAT) mean(x0, y0, x1, y1 float64) float64 {
	a := (x1 - x0) * (y1 - y0)
	if a <= 0 {
		return 0
	}
	return (t.at(x1, y1) - t.at(x0, y1) - t.at(x1, y0) + t.at(x0, y0)) / a
}

// tplMakeBand adatta la griglia alle righe r1, r2 (sulla bitmap bin, ricavata da img) e ritaglia i riquadri
// canonici di tutte le celle dall'immagine in toni di grigio. ok=false se la griglia non si adatta.
func tplMakeBand(img image.Image, bin *image.Gray, r1, r2 imgproc.Row) (*tplBand, bool) {
	xl, pitch, quality, ok := tplFitGrid(bin, r1, r2)
	if !ok {
		return nil, false
	}
	band := &tplBand{rows: [2]imgproc.Row{r1, r2}, xl: xl, pitch: pitch, quality: quality}
	rowH := float64(r1.Height()+r2.Height()) / 2 // altezza grezza delle righe: serve solo per ritagliare la zona di lavoro
	// zona di lavoro: le due righe, 1,5 passi ai lati e un'altezza sopra e sotto
	reg := imgproc.Row{
		X0: int(math.Floor(xl - 1.5*pitch)), X1: int(math.Ceil(xl + (tplCells+1.5)*pitch)),
		Y0: int(math.Floor(float64(r1.Y0) - rowH)), Y1: int(math.Ceil(float64(r2.Y1) + rowH)),
	}
	g := imgproc.GrayCrop(img, reg, 0, 0)
	ox, oy := max(0, reg.X0), max(0, reg.Y0) // origine di g nelle coordinate dell'immagine
	gw, gh := g.Bounds().Dx(), g.Bounds().Dy()
	if gw < 8 || gh < 8 {
		return nil, false
	}
	bg, ink := tplLevels(g, ox, oy, r1, r2)
	if bg-ink < 20 {
		return nil, false // contrasto troppo basso: niente testo
	}
	cov := make([]float32, gw*gh)
	for y := 0; y < gh; y++ {
		for x := 0; x < gw; x++ {
			c := (bg - float64(g.Pix[y*g.Stride+x])) / (bg - ink)
			cov[y*gw+x] = float32(math.Min(1, math.Max(0, c)))
		}
	}
	sat := newTplSAT(cov, gw, gh)
	sx := float64(tplPitch) / pitch // px del riquadro per px dell'immagine, in orizzontale
	for r, row := range band.rows {
		a, b, h, ok := tplRowLine(bin, row, xl, pitch)
		if !ok {
			return nil, false
		}
		band.yc[r], band.slope[r], band.height[r] = a, b, h
		// scala verticale: l'altezza dei caratteri nel riquadro e' tplGlyphQ[r] passi, qualunque sia la scansione
		// (alcune scansioni TWAIN hanno la scala verticale diversa dall'orizzontale)
		sy := tplGlyphQ[r] * float64(tplPitch) / h
		for k := 0; k < tplCells; k++ {
			cx := xl + (float64(k)+0.5)*pitch - float64(ox)
			cy := a + b*float64(k) - float64(oy)
			fr := make([]uint8, tplFrame*tplFrame)
			for v := 0; v < tplFrame; v++ {
				y0 := cy + (float64(v)-tplFrame/2)/sy
				y1 := cy + (float64(v)+1-tplFrame/2)/sy
				for u := 0; u < tplFrame; u++ {
					x0 := cx + (float64(u)-tplFrame/2)/sx
					x1 := cx + (float64(u)+1-tplFrame/2)/sx
					fr[v*tplFrame+u] = uint8(255*sat.mean(x0, y0, x1, y1) + 0.5)
				}
			}
			band.frames[r][k] = fr
		}
	}
	tplNormalizeThickness(band)
	return band, true
}

// tplGlyphQ: altezza dei caratteri delle due righe, in passi, nel riquadro canonico (mediana misurata sulle scansioni
// dritte: la riga 2 risulta circa il 10% piu' alta della 1 perche' le cifre e il '<' sporgono).
var tplGlyphQ = [2]float64{1.05, 1.11}

// tplRowLine misura la riga di testo: altezza dei caratteri (mediana delle altezze dell'inchiostro nelle colonne
// centrali di ogni cella) e posizione verticale del centro del testo come retta a + b*k sulle celle k (stima robusta:
// pendenza = mediana delle pendenze tra celle lontane). Il centro e' la meta' tra il primo e l'ultimo scanline con
// inchiostro, uguale per tutti i caratteri perche' lettere e cifre hanno la stessa altezza.
func tplRowLine(bin *image.Gray, r imgproc.Row, xl, pitch float64) (a, b, h float64, ok bool) {
	bw, bh := bin.Bounds().Dx(), bin.Bounds().Dy()
	y0, y1 := max(0, r.Y0-2), min(bh, r.Y1+2)
	if y1-y0 < 6 {
		return 0, 0, 0, false
	}
	// banda principale: scanline con almeno il 30% del massimo, la corsa piu' lunga (buchi fino a 2 px)
	xa, xb := max(0, int(xl)), min(bw, int(xl+tplCells*pitch))
	prof := make([]float64, y1-y0)
	var mx float64
	for y := y0; y < y1; y++ {
		for x := xa; x < xb; x++ {
			if bin.Pix[y*bin.Stride+x] == 0 {
				prof[y-y0]++
			}
		}
		mx = math.Max(mx, prof[y-y0])
	}
	if mx == 0 {
		return 0, 0, 0, false
	}
	bestA, bestB := -1, -1
	for i := 0; i < len(prof); {
		if prof[i] < 0.3*mx {
			i++
			continue
		}
		j, last := i, i
		for j < len(prof) && (prof[j] >= 0.3*mx || j-last <= 2) {
			if prof[j] >= 0.3*mx {
				last = j
			}
			j++
		}
		if last-i > bestB-bestA {
			bestA, bestB = i, last
		}
		i = last + 1
	}
	if bestA < 0 {
		return 0, 0, 0, false
	}
	t0, b0 := bestA+y0, bestB+y0
	h0 := float64(b0 - t0 + 1)
	yc0 := float64(t0+b0) / 2
	// Estremi per cella e adattamento della retta. Con la riga inclinata il profilo complessivo e' sfumato e la banda
	// principale e' piu' stretta del testo: la prima passata guarda una finestra larga, le successive una finestra
	// stretta (+-0,8 altezze: la riga vicina sta a piu' di 1,4 altezze) centrata sulla retta gia' stimata.
	a, b, h = yc0, 0, h0
	win := 1.0
	for pass := 0; pass < 3; pass++ {
		type cell struct {
			k    int
			c, h float64
		}
		var cs []cell
		for k := 0; k < tplCells; k++ {
			xs, xe := int(xl+(float64(k)+0.25)*pitch), int(xl+(float64(k)+0.75)*pitch)
			cy := a + b*float64(k)
			lo, hi := max(0, int(cy-win*h)), min(bh, int(cy+win*h)+1)
			top, bot := -1, -1
			for y := lo; y < hi; y++ {
				for x := max(0, xs); x < min(bw, xe); x++ {
					if bin.Pix[y*bin.Stride+x] == 0 {
						if top < 0 {
							top = y
						}
						bot = y
						break
					}
				}
			}
			if top >= 0 && float64(bot-top+1) >= h0/2 {
				cs = append(cs, cell{k, float64(top+bot) / 2, float64(bot - top + 1)})
			}
		}
		if len(cs) < tplCells/2 {
			return 0, 0, 0, false
		}
		// pendenza: mediana delle pendenze tra coppie di celle distanti almeno 8
		var sl []float64
		for i := range cs {
			for j := i + 1; j < len(cs); j++ {
				if cs[j].k-cs[i].k >= 8 {
					sl = append(sl, (cs[j].c-cs[i].c)/float64(cs[j].k-cs[i].k))
				}
			}
		}
		b = tplMedian(sl)
		ic := make([]float64, len(cs))
		hs := make([]float64, len(cs))
		for i, c := range cs {
			ic[i] = c.c - b*float64(c.k)
			hs[i] = c.h
		}
		a, h = tplMedian(ic), tplMedian(hs)
		win = 0.8
	}
	return a, b, h, true
}

// tplMedian restituisce la mediana (il valore centrale dopo l'ordinamento; non modifica x). 0 se x e' vuoto.
func tplMedian(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	s := append([]float64(nil), x...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// tplStrokeTarget: spessore del tratto (px del riquadro canonico) a cui si portano tutte le scansioni prima del
// confronto con i glifi: la mediana misurata sul corpus. Le stampe laser hanno il tratto piu' spesso dei facsimili
// digitali e il confronto pixel per pixel e' sensibile a quanto inchiostro c'e' (misurato: +2 px di tratto sulla
// scansione fanno scartare quasi tutte le letture, -2 px le peggiorano).
const tplStrokeTarget = 3.3

// tplStrokeWidth stima lo spessore del tratto nei riquadri come 2*area/perimetro: l'area e' la somma della
// copertura, il perimetro la variazione totale della copertura (formula dell'area di contorno: vale 1 per ogni
// unita' di lunghezza di bordo qualunque sia la sfumatura, quindi la stima e' sotto il pixel).
func tplStrokeWidth(frames [][]uint8) float64 {
	var area, perim float64
	for _, fr := range frames {
		if fr == nil {
			continue
		}
		for y := tplCropV0; y < tplCropV1; y++ {
			for x := tplCropU0; x < tplCropU1; x++ {
				c := float64(fr[y*tplFrame+x]) / 255
				area += c
				dx := float64(fr[y*tplFrame+x+1])/255 - c
				dy := float64(fr[(y+1)*tplFrame+x])/255 - c
				perim += math.Hypot(dx, dy)
			}
		}
	}
	if perim == 0 {
		return 0
	}
	return 2 * area / perim
}

// tplThicken ispessisce (r > 0) o assottiglia (r < 0) di |r| px per lato il tratto di un riquadro, usando la distanza
// con segno dal bordo (livello di copertura 0,5): copertura nuova = distanza + r + 0,5, limitata a 0..1.
func tplThicken(fr []uint8, r float64) []uint8 {
	const rad = 4
	out := make([]uint8, len(fr))
	for y := 0; y < tplFrame; y++ {
		for x := 0; x < tplFrame; x++ {
			inside := fr[y*tplFrame+x] >= 128
			// distanza dal pixel di tipo opposto piu' vicino (limitata a rad)
			d := float64(rad)
			for dy := -rad; dy <= rad; dy++ {
				yy := y + dy
				if yy < 0 || yy >= tplFrame {
					continue
				}
				for dx := -rad; dx <= rad; dx++ {
					xx := x + dx
					if xx < 0 || xx >= tplFrame || (fr[yy*tplFrame+xx] >= 128) == inside {
						continue
					}
					if e := math.Hypot(float64(dx), float64(dy)); e < d {
						d = e
					}
				}
			}
			sd := d - 0.5 // distanza dal bordo tra i pixel: > 0 dentro l'inchiostro
			if !inside {
				sd = -sd
			}
			out[y*tplFrame+x] = uint8(255*math.Min(1, math.Max(0, sd+r+0.5)) + 0.5)
		}
	}
	return out
}

// tplThicknessRadius dice di quanto va ispessito (> 0) o assottigliato (< 0) il tratto per portarlo a
// tplStrokeTarget; 0 se la differenza e' trascurabile.
func tplThicknessRadius(w float64) float64 {
	if w <= 0 {
		return 0
	}
	r := math.Max(-1.5, math.Min(1.5, (tplStrokeTarget-w)/2))
	if math.Abs(r) < 0.2 {
		return 0
	}
	return r
}

// tplNormalizeThickness porta lo spessore del tratto di tutti i riquadri della banda a tplStrokeTarget.
func tplNormalizeThickness(band *tplBand) {
	var all [][]uint8
	for r := range band.frames {
		all = append(all, band.frames[r][:]...)
	}
	band.stroke = tplStrokeWidth(all)
	r := tplThicknessRadius(band.stroke)
	if r == 0 {
		return
	}
	for ri := range band.frames {
		for k := range band.frames[ri] {
			band.frames[ri][k] = tplThicken(band.frames[ri][k], r)
		}
	}
}

// tplLevels stima il livello del fondo (mediana) e dell'inchiostro (1% piu' scuro) dai pixel dentro le due righe.
// g e' il ritaglio con origine (ox,oy) nelle coordinate dell'immagine.
func tplLevels(g *image.Gray, ox, oy int, r1, r2 imgproc.Row) (bg, ink float64) {
	var hist [256]int
	n := 0
	gw, gh := g.Bounds().Dx(), g.Bounds().Dy()
	for _, r := range [2]imgproc.Row{r1, r2} {
		for y := max(0, r.Y0-oy); y < min(gh, r.Y1-oy); y++ {
			for x := max(0, r.X0-ox); x < min(gw, r.X1-ox); x++ {
				hist[g.Pix[y*g.Stride+x]]++
				n++
			}
		}
	}
	if n == 0 {
		return 255, 0
	}
	acc := 0
	inkSet := false
	for v := 0; v < 256; v++ {
		acc += hist[v]
		if !inkSet && acc*100 >= n {
			ink, inkSet = float64(v), true
		}
		if acc*2 >= n {
			bg = float64(v)
			break
		}
	}
	return bg, ink
}

// ---------------------------------------------------------------------------------------------------------------
// Lettura

// Soglie di plausibilita': la decodifica trova sempre due righe con la struttura giusta e i check digit validi, anche
// da rumore o da testo capovolto: va giudicata la verosimiglianza della lettura, non solo i check. Misurato sul corpus
// (leave-one-booklet-out): griglia 0,02-0,06 sulle MRZ vere, dalla distanza mediana 0,2-0,6 (1,3 sul facsimile 2020 a
// tratto sottile) contro 2,4-5,2 delle stesse righe capovolte di 180 gradi; probabilita' della riga 2 = 1,00 sulle
// letture giuste, 0,05-0,92 su quelle capovolte.
const (
	tplMaxGridQuality = 0.25 // qualita' della griglia (tplFitGrid): le righe MRZ vere stanno sotto 0,06, le altre sopra 0,9
	tplMaxMedianDist  = 1.8  // distanza mediana delle celle dal glifo piu' vicino
	tplMinPost2       = 0.5  // probabilita' a posteriori minima della riga 2 (quella con i check digit)
	tplMinPost1       = 0.02 // idem per la riga 1 (senza check: resta bassa se i nomi hanno lettere incerte)
	// Costo dei vincoli (nat): quanto evidenza perde la stringa scelta rispetto al simbolo piu' vicino di ogni cella.
	// La probabilita' sopra e' relativa alle sole stringhe valide: se la MRZ stampata ha un check digit sbagliato la
	// soluzione valida piu' vicina costa una cella a evidenza forte (44 nat nella prova sintetica) ma resta l'unica
	// plausibile, quindi la probabilita' relativa e' ~1 e solo il costo assoluto la smaschera. Misurato: 0,0 su tutte
	// le righe 2 lette bene (le celle erano gia' giuste prima dei vincoli); la riga 1 arriva a 9 quando la grammatica
	// sposta una lettera.
	tplMaxCost2 = 8.0
	tplMaxCost1 = 20.0
)

// tplReading e' l'esito della lettura di una banda di due righe.
type tplReading struct {
	d     mrz.Data
	notes []string
	score int // 0 = non plausibile
	line  [2]string
	raw   [2]string // simbolo piu' vicino cella per cella, senza vincoli
	post  [2]float64
	cost  [2]float64 // costo (in nat) dei vincoli: somma dei log-punteggi del simbolo piu' vicino meno quelli scelti
	med   float64    // distanza mediana delle celle dal glifo piu' vicino
	dmin  [2][tplCells]float32
	lp    [2]tplLogp
	synth [2][tplCells]bool // il simbolo scelto ha solo glifi sintetici nella libreria (lettera mai vista su una scansione)
}

// tplRead classifica le 88 celle della banda con la libreria lib e decodifica le due righe con i vincoli MRZ.
func tplRead(band *tplBand, lib *tplLibrary) tplReading {
	var rd tplReading
	var all []float64
	for r := 0; r < 2; r++ {
		for k := 0; k < tplCells; k++ {
			q := tplReduce(band.frames[r][k])
			d := lib.classDist(&q)
			rd.lp[r][k] = tplLogProbs(&d)
			dm := tplInf
			for _, v := range d {
				dm = min(dm, v)
			}
			rd.dmin[r][k] = dm
			all = append(all, float64(dm))
		}
		rd.raw[r] = tplArgmax(&rd.lp[r])
	}
	rd.med = tplMedian(all)
	d1, d2 := tplDecodeLine1(&rd.lp[0]), tplDecodeLine2(&rd.lp[1])
	if !d1.ok || !d2.ok {
		return rd
	}
	rd.line = [2]string{d1.text, d2.text}
	rd.post = [2]float64{d1.post, d2.post}
	for r := 0; r < 2; r++ {
		for k := 0; k < tplCells; k++ {
			rd.synth[r][k] = lib.onlySynth[tplSym(rd.line[r][k])]
		}
	}
	for r := 0; r < 2; r++ {
		for k := 0; k < tplCells; k++ {
			best := rd.lp[r][k][tplSym(rd.raw[r][k])]
			rd.cost[r] += best - rd.lp[r][k][tplSym(rd.line[r][k])]
		}
	}
	data, err := mrz.Parse(d1.text, d2.text)
	if err != nil {
		return rd
	}
	rd.d = data
	rd.notes = tplNotes(&rd)
	if band.quality <= tplMaxGridQuality && rd.med <= tplMaxMedianDist && d2.post >= tplMinPost2 && d1.post >= tplMinPost1 &&
		rd.cost[1] <= tplMaxCost2 && rd.cost[0] <= tplMaxCost1 {
		rd.score = scoreMRZ(data)
	}
	return rd
}

// tplUnsure dice se la lettura della cella k della riga r e' incerta: la probabilita' del simbolo scelto e' sotto il
// 70% oppure la cella e' lontana da ogni glifo della libreria (forma mai vista: misurato leave-one-booklet-out, le
// celle sbagliate stanno tutte oltre 1,9 di distanza, il 98% di quelle giuste sotto).
func tplUnsure(rd *tplReading, r, k int) bool {
	return math.Exp(rd.lp[r][k][tplSym(rd.line[r][k])]) < 0.7 || rd.dmin[r][k] > tplFarDist
}

// tplFarDist: distanza dal glifo piu' vicino oltre la quale la cella e' considerata una forma mai vista.
const tplFarDist = 1.9

// tplNotes elenca le celle in cui i vincoli della struttura MRZ hanno cambiato il simbolo piu' vicino (correzioni) e
// quelle in cui la lettura resta incerta (tplUnsure).
func tplNotes(rd *tplReading) []string {
	var notes []string
	for r := 0; r < 2; r++ {
		for k := 0; k < tplCells; k++ {
			got, raw := rd.line[r][k], rd.raw[r][k]
			switch {
			case rd.synth[r][k]:
				notes = append(notes, fmt.Sprintf("riga%d pos %d: %c riconosciuta solo da un glifo sintetico (mai vista su una scansione): da verificare", r+1, k+1, got))
			case got != raw:
				notes = append(notes, fmt.Sprintf("riga%d pos %d: %c→%c (vincoli MRZ del lettore a template)", r+1, k+1, raw, got))
			case tplUnsure(rd, r, k):
				notes = append(notes, fmt.Sprintf("riga%d pos %d: lettura incerta (%c)", r+1, k+1, got))
			}
		}
	}
	return notes
}

// readMRZCells legge le due righe MRZ da una immagine gia' orientata (testo dritto) confrontando ogni cella con la
// libreria di glifi (glyphs.bin), senza il motore OCR di Windows. Stesse convenzioni di readMRZBand: dati, correzioni
// applicate, punteggio come scoreMRZ (0 = MRZ non trovata o lettura non plausibile). Su un'immagine senza una coppia
// di righe MRZ (rotazione sbagliata, altra pagina) ritorna subito.
//
// Procedura: ritrova la coppia di righe piu' in basso come readMRZBand (binarizzazione, proiezione dell'inchiostro),
// adatta la griglia di 44 celle a passo costante, misura altezza e inclinazione di ogni riga e ritaglia ogni cella
// in un riquadro canonico; classifica le celle sui glifi della libreria e cerca le due righe piu' probabili che
// rispettano la struttura della MRZ di Panama e i cinque check digit (mrzcells_decode.go).
func readMRZCells(img image.Image, debugDir string) (d *mrz.Data, notes []string, score int) {
	defer func() { // il contratto con Extract: mai un panic, al peggio "non trovato"
		if r := recover(); r != nil {
			d, notes, score = nil, nil, 0
		}
	}()
	b := img.Bounds()
	if b.Dx() < 300 || b.Dy() < 100 {
		return nil, nil, 0
	}
	lib := tplEmbedded()
	if len(lib.glyphs) == 0 {
		return nil, nil, 0
	}
	rd, _ := tplReadImage(img, lib, debugDir)
	if rd == nil || rd.score == 0 {
		return nil, nil, 0
	}
	return &rd.d, rd.notes, rd.score
}

// tplReadImage e' il corpo di readMRZCells con la libreria a scelta (la valutazione usa librerie senza il libretto
// letto). Restituisce la lettura migliore tra le prime due coppie di righe (nil se nessuna ha una griglia valida) e
// la banda corrispondente.
func tplReadImage(img image.Image, lib *tplLibrary, debugDir string) (*tplReading, *tplBand) {
	img, bin, pairs := tplFindRows(img)
	var best *tplReading
	var bestBand *tplBand
	for i, p := range pairs {
		if i >= 2 {
			break
		}
		band, ok := tplMakeBand(img, bin, p[0], p[1])
		if !ok || band.quality > tplMaxGridQuality {
			continue
		}
		rd := tplRead(band, lib)
		if debugDir != "" {
			tplDebug(debugDir, band, &rd)
		}
		if best == nil || rd.score > best.score {
			r := rd
			best, bestBand = &r, band
		}
		if rd.score > 0 {
			break
		}
	}
	return best, bestBand
}

// tplDebug salva in dir i riquadri delle celle (un foglio per riga) e il testo letto. Contiene dati personali.
func tplDebug(dir string, band *tplBand, rd *tplReading) {
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	for r := 0; r < 2; r++ {
		sheet := image.NewGray(image.Rect(0, 0, tplCells*tplFrame/2, tplFrame))
		for k := 0; k < tplCells; k++ {
			for y := 0; y < tplFrame; y++ {
				for x := tplFrame / 4; x < 3*tplFrame/4; x++ {
					sheet.SetGray(k*tplFrame/2+x-tplFrame/4, y, color.Gray{Y: 255 - band.frames[r][k][y*tplFrame+x]})
				}
			}
		}
		_ = imgproc.SavePNG(sheet, filepath.Join(dir, fmt.Sprintf("celle_riga%d.png", r+1)))
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "passo %.2f px, qualita' griglia %.3f, altezza caratteri %.1f/%.1f px, pendenza %.4f/%.4f px per cella\n",
		band.pitch, band.quality, band.height[0], band.height[1], band.slope[0], band.slope[1])
	fmt.Fprintf(&sb, "distanza mediana dal glifo piu' vicino %.3f, punteggio %d\n", rd.med, rd.score)
	for r := 0; r < 2; r++ {
		fmt.Fprintf(&sb, "riga%d  piu' vicino: %s\nriga%d  decodifica: %s  (probabilita' %.3f)\n", r+1, rd.raw[r], r+1, rd.line[r], rd.post[r])
	}
	for _, n := range rd.notes {
		sb.WriteString(n + "\n")
	}
	_ = os.WriteFile(filepath.Join(dir, "mrz_celle.txt"), []byte(sb.String()), 0o644)
}
