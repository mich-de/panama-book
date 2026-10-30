package qrcode

import (
	"image"
	"math"
	"sort"
)

// bitmap e' l'immagine in bianco e nero: dark[y*w+x] vero = punto scuro.
type bitmap struct {
	w, h int
	dark []bool
	gray []uint8 // i grigi della stessa finestra: servono al campionamento adattivo
}

func (b *bitmap) at(x, y int) bool {
	if x < 0 || y < 0 || x >= b.w || y >= b.h {
		return false
	}
	return b.dark[y*b.w+x]
}

// gray e' l'immagine in grigi (una sola conversione, poi si guardano le finestre).
type gray struct {
	w, h int
	pix  []uint8
}

func toGray(img image.Image) *gray {
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	if w < 21 || h < 21 || w*h > 80_000_000 {
		return nil
	}
	g := &gray{w: w, h: h, pix: make([]uint8, w*h)}
	if im, ok := img.(*image.Gray); ok && im.Stride == w {
		copy(g.pix, im.Pix[:w*h])
		return g
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			R, G, B, _ := img.At(r.Min.X+x, r.Min.Y+y).RGBA()
			// canale piu' chiaro: il nero resta nero, mentre una tinta colorata (ologramma) non lo scurisce
			g.pix[y*w+x] = uint8(max(R, G, B) >> 8)
		}
	}
	return g
}

// bitmap divide in bianco e nero la finestra rc con la soglia di Otsu calcolata sulla finestra stessa: cosi' un codice piccolo
// su uno sfondo che cambia (ologramma, foglio intero) ha la sua soglia.
func (g *gray) bitmap(rc image.Rectangle) *bitmap {
	rc = rc.Intersect(image.Rect(0, 0, g.w, g.h))
	w, h := rc.Dx(), rc.Dy()
	if w < 21 || h < 21 {
		return nil
	}
	var hist [256]int
	for y := rc.Min.Y; y < rc.Max.Y; y++ {
		for _, v := range g.pix[y*g.w+rc.Min.X : y*g.w+rc.Max.X] {
			hist[v]++
		}
	}
	total := w * h
	var sum float64
	for i, c := range hist {
		sum += float64(i * c)
	}
	var sumB, wB, best float64
	thr := 128
	for t := 0; t < 256; t++ {
		wB += float64(hist[t])
		if wB == 0 {
			continue
		}
		wF := float64(total) - wB
		if wF == 0 {
			break
		}
		sumB += float64(t * hist[t])
		mB, mF := sumB/wB, (sum-sumB)/wF
		if v := wB * wF * (mB - mF) * (mB - mF); v > best {
			best, thr = v, t
		}
	}
	b := &bitmap{w: w, h: h, dark: make([]bool, w*h), gray: make([]uint8, w*h)}
	for y := 0; y < h; y++ {
		row := g.pix[(rc.Min.Y+y)*g.w+rc.Min.X : (rc.Min.Y+y)*g.w+rc.Max.X]
		for x, v := range row {
			b.dark[y*w+x] = int(v) <= thr
			b.gray[y*w+x] = v
		}
	}
	return b
}

// finder e' un quadrato di riferimento candidato (centro e lato del modulo in pixel).
type finder struct {
	x, y, m float64
	n       int
}

var finderRatio = [5]float64{1, 1, 3, 1, 1}

// matches dice se cinque lunghezze seguono il rapporto 1:1:3:1:1; restituisce il modulo.
func matches(runs [5]int) (float64, bool) {
	total := 0
	for _, r := range runs {
		if r == 0 {
			return 0, false
		}
		total += r
	}
	if total < 7 {
		return 0, false
	}
	m := float64(total) / 7
	for i, r := range runs {
		if math.Abs(float64(r)-finderRatio[i]*m) > finderRatio[i]*m*0.55 {
			return 0, false
		}
	}
	return m, true
}

// findFinders cerca i quadrati di riferimento: righe con il rapporto 1:1:3:1:1, confermate in verticale, poi raggruppate.
func findFinders(b *bitmap) []*finder {
	var out []*finder
	for y := 0; y < b.h; y++ {
		// lunghezze delle sequenze di punti dello stesso colore sulla riga
		type run struct {
			start, n int
			dark     bool
		}
		var runs []run
		x := 0
		for x < b.w {
			d := b.dark[y*b.w+x]
			s := x
			for x < b.w && b.dark[y*b.w+x] == d {
				x++
			}
			runs = append(runs, run{s, x - s, d})
		}
		for i := 0; i+4 < len(runs); i++ {
			if !runs[i].dark {
				continue
			}
			var five [5]int
			for k := 0; k < 5; k++ {
				five[k] = runs[i+k].n
			}
			m, ok := matches(five)
			if !ok {
				continue
			}
			cx := float64(runs[i+2].start) + float64(runs[i+2].n)/2
			cy, vm, ok := crossVertical(b, int(cx), y, m)
			if !ok {
				continue
			}
			out = add(out, cx, cy, (m+vm)/2)
		}
	}
	var keep []*finder
	for _, f := range out {
		if f.n >= 2 {
			keep = append(keep, f)
		}
	}
	return keep
}

// crossVertical controlla la colonna x attorno alla riga y: stesso rapporto 1:1:3:1:1. Restituisce il centro verticale.
func crossVertical(b *bitmap, x, y int, m float64) (float64, float64, bool) {
	limit := int(m*4) + 2
	run := func(dy int, color bool, from int) (int, int) { // lunghezza e posizione di arrivo
		n, yy := 0, from
		for n < limit && b.at(x, yy) == color && yy >= 0 && yy < b.h {
			n++
			yy += dy
		}
		return n, yy
	}
	// centro scuro
	up, yu := run(-1, true, y)
	dn, yd := run(1, true, y+1)
	if up+dn == 0 {
		return 0, 0, false
	}
	l1, yu2 := run(-1, false, yu)
	d1, _ := run(-1, true, yu2)
	l2, yd2 := run(1, false, yd)
	d2, _ := run(1, true, yd2)
	mm, ok := matches([5]int{d1, l1, up + dn, l2, d2})
	if !ok {
		return 0, 0, false
	}
	top := yu + 1
	return float64(top) + float64(up+dn)/2, mm, true
}

// add unisce il candidato a uno vicino (stesso quadrato visto su altre righe) oppure ne crea uno nuovo.
func add(fs []*finder, x, y, m float64) []*finder {
	for _, f := range fs {
		if math.Abs(f.x-x) < f.m*1.5 && math.Abs(f.y-y) < f.m*1.5 && math.Abs(f.m-m) < f.m*0.5 {
			n := float64(f.n)
			f.x, f.y, f.m = (f.x*n+x)/(n+1), (f.y*n+y)/(n+1), (f.m*n+m)/(n+1)
			f.n++
			return fs
		}
	}
	return append(fs, &finder{x: x, y: y, m: m, n: 1})
}

// triple e' un possibile codice: angolo in alto a sinistra (tl), alto a destra (tr), basso a sinistra (bl).
type triple struct{ tl, tr, bl *finder }

// triples sceglie le terne di quadrati che formano un triangolo rettangolo isoscele, dalla piu' regolare.
func triples(fs []*finder) []triple {
	sort.Slice(fs, func(i, j int) bool { return fs[i].n > fs[j].n })
	if len(fs) > 12 {
		fs = fs[:12]
	}
	type sc struct {
		t   triple
		dev float64
	}
	var all []sc
	for i := 0; i < len(fs); i++ {
		for j := i + 1; j < len(fs); j++ {
			for k := j + 1; k < len(fs); k++ {
				p := [3]*finder{fs[i], fs[j], fs[k]}
				// il vertice dell'angolo retto e' opposto al lato piu' lungo
				best, bd := 0, -1.0
				for a := 0; a < 3; a++ {
					d := dist(p[(a+1)%3], p[(a+2)%3])
					if d > bd {
						best, bd = a, d
					}
				}
				tl, b, c := p[best], p[(best+1)%3], p[(best+2)%3]
				d1, d2 := dist(tl, b), dist(tl, c)
				if d1 == 0 || d2 == 0 {
					continue
				}
				dev := math.Abs(d1-d2) / math.Max(d1, d2)
				// angolo retto: prodotto scalare vicino a 0
				dot := ((b.x-tl.x)*(c.x-tl.x) + (b.y-tl.y)*(c.y-tl.y)) / (d1 * d2)
				if dev > 0.3 || math.Abs(dot) > 0.3 {
					continue
				}
				// moduli simili tra i tre quadrati
				if math.Max(tl.m, math.Max(b.m, c.m))/math.Min(tl.m, math.Min(b.m, c.m)) > 1.6 {
					continue
				}
				// orientamento: con y verso il basso, TR = b se il prodotto vettoriale (b-tl)x(c-tl) e' positivo
				if (b.x-tl.x)*(c.y-tl.y)-(b.y-tl.y)*(c.x-tl.x) < 0 {
					b, c = c, b
				}
				all = append(all, sc{triple{tl, b, c}, dev + math.Abs(dot)})
			}
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].dev < all[j].dev })
	out := make([]triple, len(all))
	for i, s := range all {
		out[i] = s.t
	}
	return append(out, pairTriples(fs)...)
}

// pairTriples ricostruisce il terzo quadrato quando se ne sono visti solo due (il terzo e' coperto da un ologramma o da una
// macchia): per ogni coppia con modulo simile e distanza plausibile si provano le quattro disposizioni ad angolo retto; il
// Reed-Solomon scarta quelle sbagliate.
func pairTriples(fs []*finder) []triple {
	var out []triple
	for i := 0; i < len(fs); i++ {
		for j := i + 1; j < len(fs); j++ {
			p, q := fs[i], fs[j]
			d := dist(p, q)
			m := (p.m + q.m) / 2
			if math.Max(p.m, q.m)/math.Min(p.m, q.m) > 1.4 || d < 14*m || d > 50*m {
				continue
			}
			mk := func(x, y float64) *finder { return &finder{x: x, y: y, m: m, n: 1} }
			for _, ends := range [][2]*finder{{p, q}, {q, p}} {
				a, b := ends[0], ends[1]
				vx, vy := b.x-a.x, b.y-a.y
				// angolo retto in a, b e' il vicino a destra: il terzo sta a +90 gradi; oppure b e' quello in basso: il terzo a -90
				out = append(out, triple{tl: a, tr: b, bl: mk(a.x-vy, a.y+vx)})
				out = append(out, triple{tl: a, tr: mk(a.x+vy, a.y-vx), bl: b})
			}
		}
	}
	return out
}

func dist(a, b *finder) float64 { return math.Hypot(a.x-b.x, a.y-b.y) }

// versions elenca le versioni possibili per un codice: quella stimata dalla distanza tra i quadrati e le vicine (la stima sbaglia
// di un modulo o due su scansioni sporche; il Reed-Solomon dice poi quale e' giusta).
func versions(t triple) []int {
	m := (t.tl.m + t.tr.m + t.bl.m) / 3
	d := (dist(t.tl, t.tr) + dist(t.tl, t.bl)) / 2
	est := (int(math.Round(d/m)) + 7 - 17 + 2) / 4 // arrotondamento al piu' vicino di (dim-17)/4
	var out []int
	for _, v := range []int{est, est - 1, est + 1, est - 2, est + 2} {
		if v >= 1 && v <= maxVersion {
			out = append(out, v)
		}
	}
	return out
}

// sampleAdaptive campiona la griglia facendo la media dei grigi di ogni modulo e confrontandola con la media del vicinato
// (9x9 moduli): regge gli sfondi che cambiano, come l'ologramma sotto il QR del libretto.
func sampleAdaptive(b *bitmap, t triple, ver int) [][]bool {
	dim := 17 + 4*ver
	ex := [2]float64{(t.tr.x - t.tl.x) / float64(dim-7), (t.tr.y - t.tl.y) / float64(dim-7)}
	ey := [2]float64{(t.bl.x - t.tl.x) / float64(dim-7), (t.bl.y - t.tl.y) / float64(dim-7)}
	val := make([][]float64, dim)
	for r := range val {
		val[r] = make([]float64, dim)
		for c := range val[r] {
			fx, fy := float64(c)+0.5-3.5, float64(r)+0.5-3.5
			px := t.tl.x + fx*ex[0] + fy*ey[0]
			py := t.tl.y + fx*ex[1] + fy*ey[1]
			sum, n := 0.0, 0
			for _, o := range [9][2]float64{{-.25, -.25}, {0, -.25}, {.25, -.25}, {-.25, 0}, {0, 0}, {.25, 0}, {-.25, .25}, {0, .25}, {.25, .25}} {
				x := int(math.Floor(px + o[0]*ex[0] + o[1]*ey[0]))
				y := int(math.Floor(py + o[0]*ex[1] + o[1]*ey[1]))
				if x >= 0 && y >= 0 && x < b.w && y < b.h {
					sum += float64(b.gray[y*b.w+x])
					n++
				}
			}
			if n > 0 {
				val[r][c] = sum / float64(n)
			} else {
				val[r][c] = 255
			}
		}
	}
	mat := make([][]bool, dim)
	const rad = 4
	for r := range mat {
		mat[r] = make([]bool, dim)
		for c := range mat[r] {
			sum, n := 0.0, 0
			for rr := max(0, r-rad); rr <= min(dim-1, r+rad); rr++ {
				for cc := max(0, c-rad); cc <= min(dim-1, c+rad); cc++ {
					sum += val[rr][cc]
					n++
				}
			}
			mat[r][c] = val[r][c] < sum/float64(n)-3
		}
	}
	return mat
}

// sample campiona la griglia dei moduli di versione ver a partire dai tre quadrati.
func sample(b *bitmap, t triple, ver int) ([][]bool, error) {
	dim := 17 + 4*ver
	ex := [2]float64{(t.tr.x - t.tl.x) / float64(dim-7), (t.tr.y - t.tl.y) / float64(dim-7)}
	ey := [2]float64{(t.bl.x - t.tl.x) / float64(dim-7), (t.bl.y - t.tl.y) / float64(dim-7)}
	mat := make([][]bool, dim)
	for r := 0; r < dim; r++ {
		mat[r] = make([]bool, dim)
		for c := 0; c < dim; c++ {
			fx, fy := float64(c)+0.5-3.5, float64(r)+0.5-3.5
			px := t.tl.x + fx*ex[0] + fy*ey[0]
			py := t.tl.y + fx*ex[1] + fy*ey[1]
			// voto a maggioranza su 5 punti (centro e quattro vicini a un quarto di modulo)
			votes := 0
			for _, o := range [5][2]float64{{0, 0}, {-.25, 0}, {.25, 0}, {0, -.25}, {0, .25}} {
				qx := px + o[0]*ex[0] + o[1]*ey[0]
				qy := py + o[0]*ex[1] + o[1]*ey[1]
				if b.at(int(math.Floor(qx)), int(math.Floor(qy))) {
					votes++
				}
			}
			mat[r][c] = votes >= 3
		}
	}
	return mat, nil
}
