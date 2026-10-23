package imgproc

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// Ritaglio automatico del documento su una pagina scansionata 8,5x14".
//
// Rispetto a CropContent (proiezione di righe/colonne con soglia fissa a 225, sensibile a rumore, bordi
// scuri e polvere, e senza correzione dell'inclinazione) qui si procede cosi':
//  1. si riduce l'immagine a ~800 px (media a blocchi: il rumore del sensore si attenua);
//  2. lo sfondo e' la mediana per canale di una cornice sul bordo della pagina;
//  3. e' "documento" cio' che si scosta dallo sfondo oltre una soglia E ha texture (deviazione standard
//     locale alta) oppure un forte contrasto: ombre lisce, gradienti e fondi grigi chiari non contano; si ignora
//     la fascia di ~6 px sul bordo (righe scure dello scanner); la maschera viene dilatata e divisa in
//     componenti connesse, scartando polvere e righe sottili;
//  4. l'inclinazione (+-6 gradi) si stima massimizzando la nitidezza della proiezione per righe;
//  5. se serve si raddrizza solo il ritaglio e si rifa' il ritaglio stretto.
//
// Limite: un documento bianco su sfondo bianco senza stampa ne' ombre non ha contrasto e non e' rilevabile.
// Funzioni pure (nessun I/O): testabili con immagini sintetiche.

const (
	acTargetSize   = 800   // lato maggiore dell'immagine ridotta
	acThreshold    = 16    // scarto massimo per canale dallo sfondo (su immagine ridotta) oltre cui e' "documento"
	acBlankFrac    = 0.005 // un documento occupa almeno questa frazione della pagina (cellule ridotte)
	acMinBoxFrac   = 0.05  // ... e il suo riquadro almeno questa frazione di larghezza e altezza
	acKeepFrac     = 0.05  // componenti minori di questa frazione della maggiore sono polvere
	acMaxSkew      = 6.0   // gradi
	acMinSkew      = 0.3   // sotto questo valore non si ruota (evita sfocature inutili)
	acMarginFrac   = 0.005 // margine attorno al documento, in frazione del lato maggiore
	acDilateFrac   = 0.02  // raggio di dilatazione della maschera, in frazione del lato maggiore ridotto
	acMaxSkewPoint = 60000
	acSpeckCells   = 8 // gruppi di celle documento piu' piccoli di cosi' sono polvere

	acBorderPx   = 6   // px sul bordo dell'immagine ignorati: lo scanner vi lascia righe scure sottili (artefatti)
	acTexRadius  = 2   // raggio (celle) della finestra per la deviazione standard locale
	acTexStd     = 8.0 // sotto questa deviazione standard locale una cella e' fondo liscio (ombra, gradiente), non documento
	acStrongDiff = 40  // scarto dal fondo oltre cui una cella e' documento anche se liscia (carta o copertina piatta e scura)
)

// CropInfo descrive il risultato di AutoCropInfo.
type CropInfo struct {
	Blank   bool            // nessun documento rilevato: l'immagine e' restituita invariata
	Box     image.Rectangle // riquadro del documento nelle coordinate dell'immagine ORIGINALE (prima del raddrizzamento)
	SkewDeg float64         // rotazione oraria applicata per raddrizzare, in gradi (0 = nessuna)
}

// IsBlank dice se la scansione e' vuota (nessun documento). Immagine nil o vuota: true.
func IsBlank(img image.Image) bool {
	return !analyze(img).ok
}

// AutoCrop ritaglia la pagina al solo documento, raddrizzandolo se inclinato di piu' di ~0,3 gradi.
// Se non rileva alcun documento restituisce l'immagine originale.
func AutoCrop(img image.Image) image.Image {
	out, _ := AutoCropInfo(img)
	return out
}

// AutoCropInfo come AutoCrop ma restituisce anche i dettagli.
func AutoCropInfo(img image.Image) (image.Image, CropInfo) {
	a := analyze(img)
	if !a.ok {
		return img, CropInfo{Blank: true}
	}
	info := CropInfo{Box: a.box}
	b := img.Bounds()
	work, workBox, bg := img, a.box, a.bg

	if skew := estimateSkew(a.pts); math.Abs(skew) >= acMinSkew {
		// ritaglio provvisorio con un po' d'aria, poi rotazione attorno al suo centro
		pad := int(0.03*float64(maxi(a.box.Dx(), a.box.Dy()))) + 4
		r := image.Rect(a.box.Min.X-pad, a.box.Min.Y-pad, a.box.Max.X+pad, a.box.Max.Y+pad).Intersect(b)
		rot := rotateFill(img, r, skew, bg)
		if a2 := analyze(rot); a2.ok {
			work, workBox, info.SkewDeg = rot, a2.box, skew
		}
	}
	margin := int(acMarginFrac*float64(maxi(b.Dx(), b.Dy()))) + 1
	wb := work.Bounds()
	r := image.Rect(workBox.Min.X-margin, workBox.Min.Y-margin, workBox.Max.X+margin, workBox.Max.Y+margin).Intersect(wb)
	return copyRect(work, r), info
}

// analysis e' il risultato dell'individuazione del documento.
type analysis struct {
	ok  bool
	box image.Rectangle // coordinate immagine (assolute, come img.Bounds())
	pts []image.Point   // celle ridotte (relative all'angolo dell'immagine) dei pixel documento validi
	bg  color.RGBA
}

// analyze individua il documento. ok=false se la pagina e' vuota.
func analyze(img image.Image) analysis {
	var res analysis
	if img == nil {
		return res
	}
	b := img.Bounds()
	if b.Dx() < 8 || b.Dy() < 8 {
		return res
	}
	lw, lh, f, pix := lowRes(img)
	if lw < 8 || lh < 8 {
		return res
	}
	bg := ringBackground(pix, lw, lh)
	tex := localStd(pix, lw, lh, acTexRadius)
	mask := make([]bool, lw*lh)
	for i := 0; i < lw*lh; i++ {
		d := acAbs(int(pix[3*i]) - int(bg[0]))
		d = maxi(d, acAbs(int(pix[3*i+1])-int(bg[1])))
		d = maxi(d, acAbs(int(pix[3*i+2])-int(bg[2])))
		mask[i] = d > acThreshold && (tex[i] > acTexStd || d > acStrongDiff)
	}
	trimBorder(mask, lw, lh, maxi(1, (acBorderPx+f-1)/f))
	comps := components(removeSpecks(mask, lw, lh), lw, lh, maxi(1, int(acDilateFrac*float64(maxi(lw, lh)))))
	if len(comps) == 0 {
		return res
	}
	// componenti valide: abbastanza grandi in assoluto, con riquadro non filiforme
	largest := 0
	for _, c := range comps {
		if c.count > largest {
			largest = c.count
		}
	}
	minCount := int(acBlankFrac * float64(lw*lh))
	var keep []comp
	for _, c := range comps {
		if c.count >= minCount && float64(c.count) >= acKeepFrac*float64(largest) &&
			float64(c.box.Dx()) >= acMinBoxFrac*float64(lw) && float64(c.box.Dy()) >= acMinBoxFrac*float64(lh) {
			keep = append(keep, c)
		}
	}
	if len(keep) == 0 {
		return res
	}
	u := keep[0].box
	for _, c := range keep {
		u = u.Union(c.box)
		res.pts = append(res.pts, c.pts...)
	}
	res.ok = true
	res.bg = color.RGBA{bg[0], bg[1], bg[2], 255}
	res.box = image.Rect(b.Min.X+u.Min.X*f, b.Min.Y+u.Min.Y*f, b.Min.X+u.Max.X*f, b.Min.Y+u.Max.Y*f).Intersect(b)
	return res
}

// rgbReader restituisce una funzione veloce che legge RGB 8 bit alle coordinate assolute.
func rgbReader(img image.Image) func(x, y int) (r, g, b uint8) {
	switch m := img.(type) {
	case *image.RGBA:
		return func(x, y int) (uint8, uint8, uint8) {
			i := m.PixOffset(x, y)
			return m.Pix[i], m.Pix[i+1], m.Pix[i+2]
		}
	case *image.NRGBA: // lo scanner e' opaco: alfa ignorato
		return func(x, y int) (uint8, uint8, uint8) {
			i := m.PixOffset(x, y)
			return m.Pix[i], m.Pix[i+1], m.Pix[i+2]
		}
	case *image.Gray:
		return func(x, y int) (uint8, uint8, uint8) {
			v := m.Pix[m.PixOffset(x, y)]
			return v, v, v
		}
	}
	return func(x, y int) (uint8, uint8, uint8) {
		r, g, b, _ := img.At(x, y).RGBA()
		return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
	}
}

// lowRes riduce l'immagine con media a blocchi f x f; restituisce 3 byte (RGB) per cella.
func lowRes(img image.Image) (lw, lh, f int, pix []uint8) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	f = maxi(1, (maxi(w, h)+acTargetSize-1)/acTargetSize)
	lw, lh = (w+f-1)/f, (h+f-1)/f
	sum := make([]uint32, 3*lw*lh)
	cnt := make([]uint32, lw*lh)
	rd := rgbReader(img)
	for y := 0; y < h; y++ {
		row := (y / f) * lw
		for x := 0; x < w; x++ {
			r, g, bl := rd(b.Min.X+x, b.Min.Y+y)
			i := row + x/f
			sum[3*i] += uint32(r)
			sum[3*i+1] += uint32(g)
			sum[3*i+2] += uint32(bl)
			cnt[i]++
		}
	}
	pix = make([]uint8, 3*lw*lh)
	for i := 0; i < lw*lh; i++ {
		for c := 0; c < 3; c++ {
			pix[3*i+c] = uint8((sum[3*i+c] + cnt[i]/2) / cnt[i])
		}
	}
	return
}

// ringBackground stima lo sfondo come mediana per canale della cornice esterna (2 celle).
func ringBackground(pix []uint8, lw, lh int) [3]uint8 {
	const t = 2
	var vals [3][]int
	add := func(x, y int) {
		i := 3 * (y*lw + x)
		for c := 0; c < 3; c++ {
			vals[c] = append(vals[c], int(pix[i+c]))
		}
	}
	for y := 0; y < lh; y++ {
		for x := 0; x < lw; x++ {
			if x < t || y < t || x >= lw-t || y >= lh-t {
				add(x, y)
			}
		}
	}
	var bg [3]uint8
	for c := 0; c < 3; c++ {
		sort.Ints(vals[c])
		bg[c] = uint8(vals[c][len(vals[c])/2])
	}
	return bg
}

type comp struct {
	count int             // celle documento (non dilatate) nella componente
	box   image.Rectangle // riquadro delle sole celle documento, Max esclusivo
	pts   []image.Point
}

// components dilata la maschera (per unire testo e zone vuote dello stesso documento) ed estrae le componenti
// connesse (4-connessione); count/box/pts si riferiscono alle celle originali, non dilatate.
func components(mask []bool, w, h, r int) []comp {
	dil := dilate(mask, w, h, r)
	seen := make([]bool, w*h)
	var out []comp
	var queue []int
	for s := 0; s < w*h; s++ {
		if !dil[s] || seen[s] {
			continue
		}
		var c comp
		queue = append(queue[:0], s)
		seen[s] = true
		for len(queue) > 0 {
			i := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			x, y := i%w, i/w
			if mask[i] {
				c.count++
				c.pts = append(c.pts, image.Pt(x, y))
				cell := image.Rect(x, y, x+1, y+1)
				if c.box.Empty() {
					c.box = cell
				} else {
					c.box = c.box.Union(cell)
				}
			}
			for _, n := range [4][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
				if n[0] < 0 || n[1] < 0 || n[0] >= w || n[1] >= h {
					continue
				}
				j := n[1]*w + n[0]
				if dil[j] && !seen[j] {
					seen[j] = true
					queue = append(queue, j)
				}
			}
		}
		if c.count > 0 {
			out = append(out, c)
		}
	}
	return out
}

// dilate: una cella e' vera se c'e' una cella vera entro il raggio r (finestra quadrata), con somme prefisse.
func dilate(mask []bool, w, h, r int) []bool {
	// passata orizzontale
	tmp := make([]bool, w*h)
	for y := 0; y < h; y++ {
		pre := make([]int, w+1)
		for x := 0; x < w; x++ {
			pre[x+1] = pre[x]
			if mask[y*w+x] {
				pre[x+1]++
			}
		}
		for x := 0; x < w; x++ {
			tmp[y*w+x] = pre[mini(w, x+r+1)]-pre[maxi(0, x-r)] > 0
		}
	}
	// passata verticale
	out := make([]bool, w*h)
	pre := make([]int, h+1)
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			pre[y+1] = pre[y]
			if tmp[y*w+x] {
				pre[y+1]++
			}
		}
		for y := 0; y < h; y++ {
			out[y*w+x] = pre[mini(h, y+r+1)]-pre[maxi(0, y-r)] > 0
		}
	}
	return out
}

// estimateSkew restituisce la rotazione oraria (gradi, asse y verso il basso) da applicare per rendere
// orizzontali i bordi/righe del documento, cercando in +-acMaxSkew gradi il massimo di nitidezza
// dell'istogramma per righe. 0 se non c'e' un massimo chiaro.
func estimateSkew(pts []image.Point) float64 {
	if len(pts) < 50 {
		return 0
	}
	step := maxi(1, len(pts)/acMaxSkewPoint)
	var cx, cy float64
	n := 0
	for i := 0; i < len(pts); i += step {
		cx += float64(pts[i].X)
		cy += float64(pts[i].Y)
		n++
	}
	cx /= float64(n)
	cy /= float64(n)
	xs := make([]float64, 0, n)
	ys := make([]float64, 0, n)
	rad := 0.0
	for i := 0; i < len(pts); i += step {
		x, y := float64(pts[i].X)-cx, float64(pts[i].Y)-cy
		xs, ys = append(xs, x), append(ys, y)
		rad = math.Max(rad, math.Hypot(x, y))
	}
	size := int(2*rad) + 4
	hist := make([]float64, size)
	score := func(deg float64) float64 {
		s, c := math.Sincos(deg * math.Pi / 180)
		for i := range hist {
			hist[i] = 0
		}
		for i := range xs {
			hist[int(math.Floor(xs[i]*s+ys[i]*c+rad))+1]++
		}
		var sc float64
		for i := 1; i < size; i++ {
			d := hist[i] - hist[i-1]
			sc += d * d
		}
		return sc
	}
	best, bestS := 0.0, score(0)
	zero := bestS
	for a := -acMaxSkew; a <= acMaxSkew+1e-9; a += 0.5 {
		if s := score(a); s > bestS {
			best, bestS = a, s
		}
	}
	for a := best - 0.5; a <= best+0.5+1e-9; a += 0.1 {
		if s := score(a); s > bestS {
			best, bestS = a, s
		}
	}
	if bestS < zero*1.05 { // nessun miglioramento netto rispetto a "dritto"
		return 0
	}
	return math.Round(best*10) / 10
}

// rotateFill ruota in senso orario di deg gradi il ritaglio r di src attorno al suo centro (interpolazione
// bilineare); l'esterno dell'immagine sorgente e' riempito con fill. Il risultato ha le dimensioni di r.
func rotateFill(src image.Image, r image.Rectangle, deg float64, fill color.RGBA) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	b := src.Bounds()
	rd := rgbReader(src)
	s, c := math.Sincos(deg * math.Pi / 180)
	cx, cy := float64(r.Dx())/2, float64(r.Dy())/2
	at := func(x, y int) (float64, float64, float64) {
		ax, ay := r.Min.X+x, r.Min.Y+y
		if ax < b.Min.X || ay < b.Min.Y || ax >= b.Max.X || ay >= b.Max.Y {
			return float64(fill.R), float64(fill.G), float64(fill.B)
		}
		R, G, B := rd(ax, ay)
		return float64(R), float64(G), float64(B)
	}
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			// posizione sorgente = rotazione inversa del punto di destinazione
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			sx := dx*c + dy*s + cx - 0.5
			sy := -dx*s + dy*c + cy - 0.5
			x0, y0 := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-float64(x0), sy-float64(y0)
			var R, G, B float64
			for j := 0; j < 2; j++ {
				for i := 0; i < 2; i++ {
					wgt := (1 - math.Abs(float64(i)-fx)) * (1 - math.Abs(float64(j)-fy))
					pr, pg, pb := at(x0+i, y0+j)
					R += wgt * pr
					G += wgt * pg
					B += wgt * pb
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(R + 0.5), uint8(G + 0.5), uint8(B + 0.5), 255})
		}
	}
	return dst
}

// copyRect copia il rettangolo r (coordinate assolute di src) in una nuova immagine con origine in (0,0).
func copyRect(src image.Image, r image.Rectangle) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	rd := rgbReader(src)
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			R, G, B := rd(r.Min.X+x, r.Min.Y+y)
			dst.SetRGBA(x, y, color.RGBA{R, G, B, 255})
		}
	}
	return dst
}

func acAbs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// removeSpecks azzera le celle documento che formano gruppi minuscoli (polvere, puntini del sensore):
// altrimenti la dilatazione grande li unirebbe al documento allargando il ritaglio.
func removeSpecks(mask []bool, w, h int) []bool {
	out := append([]bool(nil), mask...)
	for _, c := range components(mask, w, h, 1) {
		if c.count < acSpeckCells {
			for _, p := range c.pts {
				out[p.Y*w+p.X] = false
			}
		}
	}
	return out
}

// localStd restituisce la deviazione standard della luminosita' in una finestra (2r+1)^2 per ogni cella.
// Misura la "texture": un fondo liscio, anche se piu' scuro o con gradiente, ha valori bassi; testo, bordi e
// stampa del documento li hanno alti.
func localStd(pix []uint8, w, h, r int) []float32 {
	W := w + 1
	s1 := make([]float64, W*(h+1))
	s2 := make([]float64, W*(h+1))
	for y := 0; y < h; y++ {
		var r1, r2 float64
		for x := 0; x < w; x++ {
			i := 3 * (y*w + x)
			l := (float64(pix[i]) + float64(pix[i+1]) + float64(pix[i+2])) / 3
			r1 += l
			r2 += l * l
			s1[(y+1)*W+x+1] = s1[y*W+x+1] + r1
			s2[(y+1)*W+x+1] = s2[y*W+x+1] + r2
		}
	}
	out := make([]float32, w*h)
	for y := 0; y < h; y++ {
		y0, y1 := maxi(0, y-r), mini(h, y+r+1)
		for x := 0; x < w; x++ {
			x0, x1 := maxi(0, x-r), mini(w, x+r+1)
			n := float64((x1 - x0) * (y1 - y0))
			a := s1[y1*W+x1] - s1[y0*W+x1] - s1[y1*W+x0] + s1[y0*W+x0]
			b := s2[y1*W+x1] - s2[y0*W+x1] - s2[y1*W+x0] + s2[y0*W+x0]
			v := b/n - (a/n)*(a/n)
			if v < 0 {
				v = 0
			}
			out[y*w+x] = float32(math.Sqrt(v))
		}
	}
	return out
}

// trimBorder azzera le celle documento entro n celle dal bordo dell'immagine (righe scure sottili sul
// bordo estremo: artefatto dello scanner). Un documento che tocca davvero il bordo resta rilevato dalle
// celle interne; il margine finale ricopre la fascia scartata.
func trimBorder(mask []bool, w, h, n int) {
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < n || y < n || x >= w-n || y >= h-n {
				mask[y*w+x] = false
			}
		}
	}
}
