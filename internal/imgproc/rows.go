package imgproc

import (
	"image"
	"image/color"
	"sort"
)

// Row e' una riga di testo individuata per proiezione dell'inchiostro.
type Row struct {
	Y0, Y1 int // estremi verticali (Y1 esclusa)
	X0, X1 int // estremi orizzontali dell'inchiostro (X1 esclusa)
}

func (r Row) Height() int { return r.Y1 - r.Y0 }
func (r Row) Width() int  { return r.X1 - r.X0 }

// Binarize converte in bianco e nero. La soglia e' a meta' tra il fondo (mediana) e l'inchiostro (0,5% piu' scuro),
// cosi' i disegni di sicurezza chiari (guilloche) spariscono e resta solo il testo stampato.
func Binarize(src image.Image) *image.Gray {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	g := make([]uint8, w*h)
	var hist [256]int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(gray(src.At(b.Min.X+x, b.Min.Y+y)))
			g[y*w+x] = v
			hist[v]++
		}
	}
	total := w * h
	bg, ink, acc := 255, 0, 0
	for v := 0; v < 256; v++ {
		acc += hist[v]
		if acc >= total/200 && ink == 0 {
			ink = v
		}
		if acc >= total/2 {
			bg = v
			break
		}
	}
	thr := uint8((bg + ink) / 2)
	out := image.NewGray(image.Rect(0, 0, w, h))
	for i, v := range g {
		if v > thr {
			out.Pix[i] = 255
		}
	}
	return out
}

// ClearLines cancella (rende bianche) le colonne e le righe quasi interamente scure: bordi neri e ombre dello
// scanner, margini del libretto, che altrimenti fanno risultare inchiostro su ogni scansione e fondono tutte le
// righe di testo in una sola. Le colonne/righe confinanti (sfumatura del bordo) vengono cancellate con loro.
//
// Una colonna "scura" (piu' del 35% di inchiostro) si cancella solo lungo le corse verticali lunghe di inchiostro
// (>= minVerticalRun), non per tutta l'altezza: una colonna scura per la foto del titolare, per esempio, attraversa
// anche le righe MRZ e cancellarla intera le tagliava (la scansione scana2 perdeva "ISPAN" e "PA012"). Le aste dei
// caratteri sono molto piu' corte di una corsa lunga, quindi il testo resta intatto.
func ClearLines(bin *image.Gray) {
	b := bin.Bounds()
	w, h := b.Dx(), b.Dy()
	colInk := make([]int, w)
	rowInk := make([]int, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if bin.Pix[y*bin.Stride+x] == 0 {
				colInk[x]++
				rowInk[y]++
			}
		}
	}
	badCol := make([]bool, w)
	badRow := make([]bool, h)
	for x, n := range colInk {
		if float64(n) > 0.35*float64(h) {
			for k := max(0, x-3); k <= min(w-1, x+3); k++ {
				badCol[k] = true
			}
		}
	}
	for y, n := range rowInk {
		if float64(n) > 0.6*float64(w) {
			for k := max(0, y-3); k <= min(h-1, y+3); k++ {
				badRow[k] = true
			}
		}
	}
	minRun := max(40, int(minVerticalRun*float64(h)))
	for x := 0; x < w; x++ {
		if !badCol[x] {
			continue
		}
		// corse di inchiostro lungo la colonna (con buchi fino a 3 px: bordo sfumato); si cancellano le sole lunghe
		for y := 0; y < h; {
			if bin.Pix[y*bin.Stride+x] != 0 {
				y++
				continue
			}
			end, last := y, y // end: prima riga dopo la corsa; last: ultimo pixel di inchiostro
			for end < h && (bin.Pix[end*bin.Stride+x] == 0 || end-last <= 3) {
				if bin.Pix[end*bin.Stride+x] == 0 {
					last = end
				}
				end++
			}
			if last+1-y >= minRun {
				for k := y; k <= last; k++ {
					bin.Pix[k*bin.Stride+x] = 255
				}
			}
			y = last + 1
		}
	}
	for y := 0; y < h; y++ {
		if badRow[y] {
			for x := 0; x < w; x++ {
				bin.Pix[y*bin.Stride+x] = 255
			}
		}
	}
}

// minVerticalRun: lunghezza minima (frazione dell'altezza dell'immagine) di una corsa verticale di inchiostro perche'
// ClearLines la consideri un bordo. Le righe MRZ sono alte ~3% della pagina, quindi 8% sta ben sopra le aste dei
// caratteri.
const minVerticalRun = 0.08

// TextRows trova le righe di testo di una immagine binarizzata (inchiostro = pixel scuri).
// Scarta le righe piu' strette di minWidthFrac della larghezza dell'immagine.
func TextRows(bin *image.Gray, minWidthFrac float64) []Row {
	b := bin.Bounds()
	w, h := b.Dx(), b.Dy()
	count := make([]int, h)
	minX := make([]int, h)
	maxX := make([]int, h)
	for y := 0; y < h; y++ {
		minX[y], maxX[y] = w, -1
		for x := 0; x < w; x++ {
			if bin.Pix[y*bin.Stride+x] == 0 {
				count[y]++
				if x < minX[y] {
					minX[y] = x
				}
				maxX[y] = x
			}
		}
	}
	var rows []Row
	start := -1
	gap := 0
	flush := func(end int) {
		if start < 0 {
			return
		}
		r := Row{Y0: start, Y1: end, X0: w, X1: 0}
		cols := make([]bool, w)
		for y := start; y < end; y++ {
			if count[y] > 0 {
				for x := minX[y]; x <= maxX[y]; x++ {
					if bin.Pix[y*bin.Stride+x] == 0 {
						cols[x] = true
					}
				}
			}
		}
		r.X0, r.X1 = MainSpan(cols, 2*(end-start))
		if r.X1 > r.X0 && r.Height() >= 12 && float64(r.Width()) >= minWidthFrac*float64(w) {
			rows = append(rows, r)
		}
		start = -1
	}
	for y := 0; y < h; y++ {
		if count[y] >= 3 {
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

// TrimRowEdgeBars toglie dagli estremi orizzontali delle righe le barre verticali sottili e isolate: il bordo
// della pagina (ombra o linea del libretto vero) che finisce nella fascia della riga, a meno di 2 altezze dal testo,
// e che MainSpan fonde con la riga. Una barra e' tolta se e' larga al massimo 1/5 dell'altezza della riga ed e'
// separata dal resto da un vuoto di almeno 0,9 altezze (tra due caratteri vicini il vuoto e' molto minore). Le righe
// senza barre restano identiche. Restituisce una copia di rows.
func TrimRowEdgeBars(bin *image.Gray, rows []Row) []Row {
	out := append([]Row(nil), rows...)
	for i, r := range out {
		h := r.Height()
		type run struct{ a, b int } // colonne [a,b]
		var runs []run
		for x := r.X0; x < r.X1; x++ {
			ink := false
			for y := r.Y0; y < r.Y1 && !ink; y++ {
				ink = bin.Pix[y*bin.Stride+x] == 0
			}
			switch {
			case !ink:
			case len(runs) > 0 && x-runs[len(runs)-1].b <= 3:
				runs[len(runs)-1].b = x
			default:
				runs = append(runs, run{x, x})
			}
		}
		thin := func(q run) bool { return 5*(q.b-q.a+1) <= h }
		bigGap := func(a, b run) bool { return float64(b.a-a.b) >= 0.9*float64(h) }
		for len(runs) >= 3 && thin(runs[len(runs)-1]) && bigGap(runs[len(runs)-2], runs[len(runs)-1]) {
			runs = runs[:len(runs)-1]
		}
		for len(runs) >= 3 && thin(runs[0]) && bigGap(runs[0], runs[1]) {
			runs = runs[1:]
		}
		if len(runs) > 0 {
			out[i].X0, out[i].X1 = runs[0].a, runs[len(runs)-1].b+1
		}
	}
	return out
}

// MRZPairs elenca, dal basso verso l'alto, le coppie di righe consecutive compatibili con le due righe di una MRZ:
// altezza simile, stesso margine sinistro, larghezza simile (entrambe hanno 44 caratteri), poco spazio in mezzo.
// Ordina rows per posizione verticale.
func MRZPairs(rows []Row, imgW int) [][2]Row {
	sort.Slice(rows, func(i, j int) bool { return rows[i].Y0 < rows[j].Y0 })
	var out [][2]Row
	for i := len(rows) - 2; i >= 0; i-- {
		a, b := rows[i], rows[i+1]
		hMax := max(a.Height(), b.Height())
		hMin := min(a.Height(), b.Height())
		if float64(hMin) < 0.7*float64(hMax) {
			continue
		}
		if abs(a.X0-b.X0) > imgW/25 {
			continue
		}
		if a.Width() < 12*a.Height() || b.Width() < 12*b.Height() {
			continue // 44 caratteri affiancati: una riga MRZ e' almeno ~20 volte piu' larga che alta
		}
		wMax := max(a.Width(), b.Width())
		wMin := min(a.Width(), b.Width())
		if float64(wMin) < 0.8*float64(wMax) {
			continue
		}
		if b.Y0-a.Y1 > hMax*3 {
			continue
		}
		out = append(out, [2]Row{a, b})
	}
	return out
}

// MainSpan restituisce l'intervallo [x0,x1) del gruppo piu' largo di colonne con inchiostro, dove due colonne
// appartengono allo stesso gruppo se distano meno di maxGap: scarta segni isolati lontani dal testo
// (bordi, righe verticali del modulo, macchie).
func MainSpan(cols []bool, maxGap int) (int, int) {
	bestX0, bestX1 := 0, 0
	x0, last := -1, -1
	closeGroup := func() {
		if x0 >= 0 && last+1-x0 > bestX1-bestX0 {
			bestX0, bestX1 = x0, last+1
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
	return bestX0, bestX1
}

// LowestPair sceglie la coppia di righe consecutive piu' in basso compatibile con una MRZ (vedi MRZPairs).
// Restituisce ok=false se non c'e'.
func LowestPair(rows []Row, imgW int) (Row, Row, bool) {
	ps := MRZPairs(rows, imgW)
	if len(ps) == 0 {
		return Row{}, Row{}, false
	}
	return ps[0][0], ps[0][1], true
}

// CropRow ritaglia la riga dalla bitmap binarizzata con un margine bianco tutt'intorno.
func CropRow(bin *image.Gray, r Row, padX, padY int) *image.Gray {
	b := bin.Bounds()
	x0, y0 := max(0, r.X0-padX), max(0, r.Y0-padY)
	x1, y1 := min(b.Dx(), r.X1+padX), min(b.Dy(), r.Y1+padY)
	const margin = 30
	out := image.NewGray(image.Rect(0, 0, x1-x0+2*margin, y1-y0+2*margin))
	for i := range out.Pix {
		out.Pix[i] = 255
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			out.SetGray(x-x0+margin, y-y0+margin, color.Gray{Y: bin.Pix[y*bin.Stride+x]})
		}
	}
	return out
}

// StrokeWidth stima lo spessore del tratto (px) di una immagine binarizzata come mediana delle lunghezze dei run
// orizzontali di inchiostro: nel testo prevalgono le aste verticali, quindi la mediana e' lo spessore delle aste.
// Restituisce 0 se non c'e' inchiostro.
func StrokeWidth(bin *image.Gray) float64 {
	b := bin.Bounds()
	w, h := b.Dx(), b.Dy()
	var hist []int // hist[n] = numero di run lunghi n
	total := 0
	for y := 0; y < h; y++ {
		run := 0
		for x := 0; x <= w; x++ {
			if x < w && bin.Pix[y*bin.Stride+x] == 0 {
				run++
				continue
			}
			if run > 0 {
				for len(hist) <= run {
					hist = append(hist, 0)
				}
				hist[run]++
				total++
				run = 0
			}
		}
	}
	if total == 0 {
		return 0
	}
	acc := 0
	for n, c := range hist {
		acc += c
		if 2*acc >= total {
			return float64(n)
		}
	}
	return 0
}

// Erode assottiglia l'inchiostro (pixel a 0) di n pixel per lato (elemento strutturante quadrato (2n+1)x(2n+1)):
// il tratto perde 2n px di spessore. Restituisce una copia; l'originale non cambia. Fuori dall'immagine e' bianco.
func Erode(bin *image.Gray, n int) *image.Gray {
	b := bin.Bounds()
	w, h := b.Dx(), b.Dy()
	if n <= 0 {
		out := image.NewGray(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			copy(out.Pix[y*out.Stride:y*out.Stride+w], bin.Pix[y*bin.Stride:y*bin.Stride+w])
		}
		return out
	}
	// passata orizzontale: un pixel resta inchiostro solo se lo sono tutti i vicini a distanza <= n
	tmp := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		row := bin.Pix[y*bin.Stride : y*bin.Stride+w]
		run := 0 // inchiostri consecutivi che finiscono in x
		for x := 0; x < w; x++ {
			if row[x] == 0 {
				run++
			} else {
				run = 0
			}
			// il pixel x-n e' inchiostro se i 2n+1 pixel centrati su di esso lo sono: run (che termina in x) >= 2n+1
			c := x - n
			if c >= 0 && run >= 2*n+1 {
				tmp.Pix[y*tmp.Stride+c] = 0
			} else if c >= 0 {
				tmp.Pix[y*tmp.Stride+c] = 255
			}
		}
		for c := max(0, w-n); c < w; c++ { // gli ultimi n pixel hanno vicini fuori immagine (bianchi)
			tmp.Pix[y*tmp.Stride+c] = 255
		}
	}
	// passata verticale, stessa logica sulle colonne
	out := image.NewGray(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		run := 0
		for y := 0; y < h; y++ {
			if tmp.Pix[y*tmp.Stride+x] == 0 {
				run++
			} else {
				run = 0
			}
			c := y - n
			if c >= 0 && run >= 2*n+1 {
				out.Pix[c*out.Stride+x] = 0
			} else if c >= 0 {
				out.Pix[c*out.Stride+x] = 255
			}
		}
		for c := max(0, h-n); c < h; c++ {
			out.Pix[c*out.Stride+x] = 255
		}
	}
	return out
}

// GrayCrop ritaglia la riga r dall'immagine in toni di grigio, con padX/padY pixel di contorno (limitati ai bordi).
func GrayCrop(src image.Image, r Row, padX, padY int) *image.Gray {
	b := src.Bounds()
	x0, y0 := max(0, r.X0-padX), max(0, r.Y0-padY)
	x1, y1 := min(b.Dx(), r.X1+padX), min(b.Dy(), r.Y1+padY)
	out := image.NewGray(image.Rect(0, 0, max(0, x1-x0), max(0, y1-y0)))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			out.Pix[(y-y0)*out.Stride+x-x0] = uint8(gray(src.At(b.Min.X+x, b.Min.Y+y)))
		}
	}
	return out
}

// InkFraction e' la frazione di pixel di inchiostro (0) dell'immagine.
func InkFraction(bin *image.Gray) float64 {
	b := bin.Bounds()
	n := 0
	for y := 0; y < b.Dy(); y++ {
		for _, v := range bin.Pix[y*bin.Stride : y*bin.Stride+b.Dx()] {
			if v == 0 {
				n++
			}
		}
	}
	if b.Dx()*b.Dy() == 0 {
		return 0
	}
	return float64(n) / float64(b.Dx()*b.Dy())
}

// ThinToInk binarizza il ritaglio in grigio e lo assottiglia di erode pixel per lato, scegliendo la soglia di
// binarizzazione (la piu' bassa possibile) che dopo l'erosione lascia almeno la frazione target di inchiostro.
// Il motore OCR di Windows legge le stampe laser troppo spesse solo in una finestra stretta di inchiostro residuo:
// regolando la soglia (e non solo l'erosione a gradini interi) si centra la finestra. Restituisce l'immagine e
// la soglia usata.
func ThinToInk(g *image.Gray, erode int, target float64) (*image.Gray, int) {
	at := func(thr int) *image.Gray {
		b := g.Bounds()
		bin := image.NewGray(b)
		for i, v := range g.Pix {
			if int(v) > thr {
				bin.Pix[i] = 255
			}
		}
		return Erode(bin, erode)
	}
	lo, hi := 0, 255
	for lo < hi {
		mid := (lo + hi) / 2
		if InkFraction(at(mid)) >= target {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return at(lo), lo
}

// PadWhite aggiunge m pixel bianchi su ogni lato.
func PadWhite(g *image.Gray, m int) *image.Gray {
	b := g.Bounds()
	out := image.NewGray(image.Rect(0, 0, b.Dx()+2*m, b.Dy()+2*m))
	for i := range out.Pix {
		out.Pix[i] = 255
	}
	for y := 0; y < b.Dy(); y++ {
		copy(out.Pix[(y+m)*out.Stride+m:(y+m)*out.Stride+m+b.Dx()], g.Pix[y*g.Stride:y*g.Stride+b.Dx()])
	}
	return out
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
