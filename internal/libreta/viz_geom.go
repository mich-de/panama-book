package libreta

// Geometria della MRZ nella scansione. Le due righe MRZ sono il punto fisso da cui dipende la posizione di ogni campo
// (vizBands e' in unita' di larghezza W della MRZ). Nelle scansioni reali le righe vicine (bordo della pagina, ombre,
// texture della plastificazione) sporcano gli estremi o si fondono in righe spurie, e il passo dei caratteri (2,54 mm,
// 44 caratteri) e' l'unica cosa sempre vera: ogni riga candidata riceve un periodo (passo) e una forza (autocorrelazione
// del profilo delle colonne di inchiostro) e la coppia si sceglie tra quelle con righe periodiche, di passo uguale,
// vicine e allineate; la larghezza W e' poi vizMRZChars passi se gli estremi misurati sono sporchi.

import (
	"image"
	"math"
	"sort"
)

// vizMRZRow e' una riga candidata, in pixel dell'immagine originale, con il periodo misurato.
type vizMRZRow struct {
	y0, y1, x0, x1 int
	pitch          float64 // passo in pixel dell'immagine originale (0 = nessuna periodicita')
	strength       float64 // correlazione al passo, 0..1
}

// vizRowPeriod misura il periodo del profilo di colonne dell'inchiostro della riga (ink e' scalata di k): sceglie il passo p
// che massimizza la somma armonica corr(p) + corr(2p)/2 + corr(3p)/3 + ... su tutti i multipli fino a 5 altezze di riga,
// che premia il periodo fondamentale contro i suoi multipli (meno termini) e sottomultipli (corr bassa sui multipli dispari) (i glifi hanno 2-3 aste per cella). Restituisce passo (pixel di ink) e forza.
func vizRowPeriod(ink []bool, w, x0, x1, y0, y1 int) (pitch, strength float64) {
	h := len(ink) / w
	x0, x1 = max(0, x0), min(w, x1)
	n := x1 - x0
	hh := y1 - y0
	if n < 120 || hh < 6 {
		return 0, 0
	}
	prof := make([]float64, n)
	for y := max(0, y0); y < min(h, y1); y++ {
		for x := 0; x < n; x++ {
			if ink[y*w+x0+x] {
				prof[x]++
			}
		}
	}
	var mean float64
	for _, v := range prof {
		mean += v
	}
	mean /= float64(n)
	var den float64
	for _, v := range prof {
		den += (v - mean) * (v - mean)
	}
	if den == 0 {
		return 0, 0
	}
	lo, hi := max(4, int(0.35*float64(hh))), int(1.25*float64(hh))
	maxLag := min(n/2, 5*hi)
	if hi >= n/6 || lo >= hi {
		return 0, 0
	}
	corr := make([]float64, maxLag+2)
	for lag := lo; lag <= maxLag; lag++ {
		var c float64
		for x := 0; x+lag < n; x++ {
			c += (prof[x] - mean) * (prof[x+lag] - mean)
		}
		corr[lag] = c / (den * float64(n-lag) / float64(n))
	}
	at := func(lag int) float64 {
		if lag < lo || lag > maxLag {
			return 0
		}
		return corr[lag]
	}
	bestP, bestS := 0, -math.MaxFloat64
	for p := lo; p <= hi; p++ {
		// somma armonica su tutti i multipli fino a maxLag: il fondamentale ne ha il doppio del suo multiplo doppio
		s := 0.0
		for m := 1; m*p <= maxLag; m++ {
			s += at(m*p) / float64(m)
		}
		if s > bestS {
			bestP, bestS = p, s
		}
	}
	if bestP == 0 || corr[bestP] < 0.12 {
		return 0, 0
	}
	// il massimo vero sta vicino: si riprende il massimo locale di corr entro +-2 e si raffina con una parabola
	p := bestP
	for d := -2; d <= 2; d++ {
		if at(bestP+d) > at(p) {
			p = bestP + d
		}
	}
	pitch = float64(p)
	if p > lo && p < maxLag {
		a, b, c := corr[p-1], corr[p], corr[p+1]
		if d := a - 2*b + c; d < 0 {
			pitch += 0.5 * (a - c) / d
		}
	}
	return pitch, corr[p]
}

// vizCandidateRows elenca le righe candidate (di imgproc, che cancella i bordi lunghi) con il loro periodo.
func vizCandidateRows(g *image.Gray, ink []bool, w, k int) []vizMRZRow {
	imgW := g.Bounds().Dx()
	var out []vizMRZRow
	for _, r := range vizBinRows(g) {
		hh := r.Y1 - r.Y0
		if r.X1-r.X0 < imgW/5 || hh < 8 || float64(hh) > 0.045*float64(imgW) {
			continue
		}
		p, s := vizRowPeriod(ink, w, r.X0/k, r.X1/k, r.Y0/k, r.Y1/k)
		out = append(out, vizMRZRow{y0: r.Y0, y1: r.Y1, x0: r.X0, x1: r.X1, pitch: p * float64(k), strength: s})
	}
	return out
}

// vizPickPair sceglie la coppia di righe MRZ: due righe periodiche, di passo uguale, vicine, con il margine sinistro
// allineato e larghezza compatibile con 44 caratteri. A parita' vince la coppia piu' in basso (la MRZ sta in fondo).
func vizPickPair(rows []vizMRZRow) (a, b vizMRZRow, ok bool) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].y0 < rows[j].y0 })
	bestScore := 0.0
	for i := range rows {
		for j := i + 1; j < len(rows); j++ {
			ra, rb := rows[i], rows[j]
			if ra.pitch == 0 || rb.pitch == 0 || ra.strength < vizMinStrength || rb.strength < vizMinStrength {
				continue
			}
			p := (ra.pitch + rb.pitch) / 2
			if math.Abs(ra.pitch-rb.pitch) > 0.08*p {
				continue
			}
			ha, hb := float64(ra.y1-ra.y0), float64(rb.y1-rb.y0)
			hmax := math.Max(ha, hb)
			if math.Min(ha, hb) < 0.55*hmax || p < 0.4*math.Min(ha, hb) || p > 1.3*math.Max(ha, hb) {
				continue
			}
			gap := float64(rb.y0 - ra.y1)
			if gap < -0.3*hmax || gap > 2.2*hmax {
				continue
			}
			if math.Abs(float64(ra.x0-rb.x0)) > 2*p {
				continue
			}
			wm := float64(min(ra.x1, rb.x1) - max(ra.x0, rb.x0))
			if wm < 36*p || wm > 62*p {
				continue
			}
			// la riga 2 dell'MRZ e' piena di caratteri fino al fondo: nessuna riga sotto la coppia con lo stesso passo
			score := ra.strength + rb.strength + 0.02*float64(rb.y0)/float64(rb.y0+1)
			if score > bestScore+1e-9 || (math.Abs(score-bestScore) <= 1e-9 && rb.y0 > b.y0) {
				a, b, ok, bestScore = ra, rb, true, score
			}
		}
	}
	return
}

// vizMinStrength: forza minima dell'autocorrelazione di una riga MRZ (misurata 0,6-0,7 sulle scansioni, 0,3 o meno sui
// bordi e sulle righe spurie).
const vizMinStrength = 0.3

// vizGeomFromPair costruisce la geometria dalla coppia: margine sinistro dall'intersezione, larghezza dagli estremi se
// coerente con il passo, altrimenti dal passo (estremo destro sporco).
func vizGeomFromPair(a, b vizMRZRow, ink []bool, w, k int) vizGeom {
	x0 := max(a.x0, b.x0)
	x1 := min(a.x1, b.x1)
	p := (a.pitch + b.pitch) / 2
	pw := vizMRZChars * p
	m := float64(x1 - x0)
	width := m
	switch {
	case math.Abs(m-pw) <= (vizPitchTol-1)*pw: // entro il 2,5%: gli estremi misurati sono buoni
	case m > pw:
		width = pw // estremo destro sporco
	}
	if vizTraceWidth != nil {
		vizTraceWidth(m, pw)
	}
	x1 = x0 + int(width+0.5)
	rows := [2]vizRow{{a.y0 / k, a.y1 / k, x0 / k, x1 / k, 0}, {b.y0 / k, b.y1 / k, x0 / k, x1 / k, 0}}
	return vizGeom{
		skew: vizSkew(ink, w, rows),
		x0:   float64(x0), w: float64(x1 - x0), y0: float64(a.y0),
		rows: [2][2]int{{a.y0, a.y1}, {b.y0, b.y1}},
	}
}
