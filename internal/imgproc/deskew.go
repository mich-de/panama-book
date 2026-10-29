package imgproc

// Raddrizzamento di pagine inclinate (foglio inserito storto nello scanner).

import (
	"image"
	"image/color"
	"math"
)

const (
	skewRange     = 12.0 // gradi massimi cercati per lato
	skewSmallSide = 700  // lato lungo (px) dell'immagine ridotta su cui si stima l'inclinazione
	skewMaxPoints = 60000
	// DeskewMinAngle: sotto questo angolo (gradi) Deskew non ruota. L'OCR tollera inclinazioni piccole e sui
	// facsimili digitali la stima da' 1,3-1,5 gradi: ruotarli li ha resi illeggibili (misurato sul facsimile QR).
	DeskewMinAngle = 2.0
)

// EstimateSkew stima l'inclinazione delle righe di testo della pagina, in gradi entro +-12: positiva se le righe
// scendono verso destra (pagina ruotata in senso orario, asse y verso il basso). Per raddrizzare la pagina si
// ruota di -angolo con RotateAngle (o si usa Deskew). Restituisce 0 se non c'e' testo o non c'e' un massimo chiaro.
//
// Metodo: sull'immagine ridotta si prende l'inchiostro (come Binarize) e si cerca l'angolo che massimizza la
// nitidezza della proiezione orizzontale (le righe di testo si sovrappongono nello stesso bin solo se dritte), a
// passo grosso e poi fine.
func EstimateSkew(img image.Image) float64 {
	pts := skewPoints(img)
	if len(pts) < 200 {
		return 0
	}
	var cx, cy, rad float64
	for _, p := range pts {
		cx += p[0]
		cy += p[1]
	}
	cx /= float64(len(pts))
	cy /= float64(len(pts))
	xs := make([]float64, len(pts))
	ys := make([]float64, len(pts))
	for i, p := range pts {
		xs[i], ys[i] = p[0]-cx, p[1]-cy
		rad = math.Max(rad, math.Hypot(xs[i], ys[i]))
	}
	size := int(2*rad) + 4
	hist := make([]float64, size)
	score := func(deg float64) float64 {
		// y' = y*cos - x*sin: coordinata perpendicolare alle righe che scendono a destra di deg gradi
		s, c := math.Sincos(deg * math.Pi / 180)
		for i := range hist {
			hist[i] = 0
		}
		for i := range xs {
			hist[int(math.Floor(ys[i]*c-xs[i]*s+rad))+1]++
		}
		var sc float64
		for i := 1; i < size; i++ {
			d := hist[i] - hist[i-1]
			sc += d * d
		}
		return sc
	}
	zero := score(0)
	best, bestS := 0.0, zero
	for a := -skewRange; a <= skewRange+1e-9; a += 0.5 {
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

// EstimateRowsSkew stima l'inclinazione (gradi, positiva se le righe scendono verso destra) di righe di testo gia'
// individuate, usando solo l'inchiostro dentro i loro riquadri (allargati verticalmente di mezza altezza). Con le sole
// righe MRZ (testo pulito, tutto maiuscolo) il picco di nitidezza della proiezione orizzontale e' netto anche per
// inclinazioni di 0,5-1 grado, che EstimateSkew sulla pagina intera non rileva (e Deskew non corregge, sotto
// DeskewMinAngle) ma che bastano a far restituire testo vuoto al motore OCR di Windows. Cerca entro +-4 gradi,
// a passo grosso e poi fine; restituisce 0 se non c'e' un miglioramento netto rispetto a "dritto".
func EstimateRowsSkew(bin *image.Gray, rows ...Row) float64 {
	b := bin.Bounds()
	w, h := b.Dx(), b.Dy()
	var xs, ys []float64
	for _, r := range rows {
		pad := r.Height() / 2
		for y := max(0, r.Y0-pad); y < min(h, r.Y1+pad); y++ {
			for x := max(0, r.X0); x < min(w, r.X1); x++ {
				if bin.Pix[y*bin.Stride+x] == 0 {
					xs = append(xs, float64(x))
					ys = append(ys, float64(y))
				}
			}
		}
	}
	if len(xs) < 200 {
		return 0
	}
	var cx, cy, rad float64
	for i := range xs {
		cx += xs[i]
		cy += ys[i]
	}
	cx /= float64(len(xs))
	cy /= float64(len(xs))
	for i := range xs {
		xs[i] -= cx
		ys[i] -= cy
		rad = math.Max(rad, math.Hypot(xs[i], ys[i]))
	}
	size := int(2*rad) + 4
	hist := make([]float64, size)
	score := func(deg float64) float64 {
		s, c := math.Sincos(deg * math.Pi / 180)
		for i := range hist {
			hist[i] = 0
		}
		for i := range xs {
			hist[int(math.Floor(ys[i]*c-xs[i]*s+rad))+1]++
		}
		var sc float64
		for i := 1; i < size; i++ {
			d := hist[i] - hist[i-1]
			sc += d * d
		}
		return sc
	}
	zero := score(0)
	best, bestS := 0.0, zero
	for a := -4.0; a <= 4.0+1e-9; a += 0.25 {
		if s := score(a); s > bestS {
			best, bestS = a, s
		}
	}
	for a := best - 0.25; a <= best+0.25+1e-9; a += 0.05 {
		if s := score(a); s > bestS {
			best, bestS = a, s
		}
	}
	if bestS < zero*1.05 {
		return 0
	}
	return math.Round(best*20) / 20
}

// skewPoints restituisce le coordinate (nell'immagine ridotta) dei pixel di inchiostro, al massimo skewMaxPoints.
func skewPoints(img image.Image) [][2]float64 {
	b := img.Bounds()
	f := 1.0
	if long := max(b.Dx(), b.Dy()); long > skewSmallSide {
		f = float64(skewSmallSide) / float64(long)
	}
	w, h := max(1, int(float64(b.Dx())*f)), max(1, int(float64(b.Dy())*f))
	small := reduceGray(img, w, h)
	bin := Binarize(small)
	ClearLines(bin)
	var pts [][2]float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if bin.Pix[y*bin.Stride+x] == 0 {
				pts = append(pts, [2]float64{float64(x), float64(y)})
			}
		}
	}
	if len(pts) > skewMaxPoints {
		step := float64(len(pts)) / skewMaxPoints
		out := make([][2]float64, 0, skewMaxPoints)
		for i := 0.0; int(i) < len(pts); i += step {
			out = append(out, pts[int(i)])
		}
		pts = out
	}
	return pts
}

// reduceGray riduce a w x h in toni di grigio con media a blocchi (conserva i tratti sottili meglio del campionamento).
func reduceGray(img image.Image, w, h int) *image.Gray {
	b := img.Bounds()
	out := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy0, sy1 := y*b.Dy()/h, max(y*b.Dy()/h+1, (y+1)*b.Dy()/h)
		for x := 0; x < w; x++ {
			sx0, sx1 := x*b.Dx()/w, max(x*b.Dx()/w+1, (x+1)*b.Dx()/w)
			var sum float64
			n := 0
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					sum += gray(img.At(b.Min.X+sx, b.Min.Y+sy))
					n++
				}
			}
			out.Pix[y*out.Stride+x] = uint8(sum/float64(n) + 0.5)
		}
	}
	return out
}

// RotateAngle ruota l'immagine in senso orario di deg gradi (asse y verso il basso) con interpolazione bilineare.
// Le dimensioni si espandono per non perdere i bordi; il nuovo fondo e' bianco.
func RotateAngle(img image.Image, deg float64) image.Image {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	s, c := math.Sincos(deg * math.Pi / 180)
	dw := int(math.Ceil(math.Abs(float64(sw)*c) + math.Abs(float64(sh)*s)))
	dh := int(math.Ceil(math.Abs(float64(sw)*s) + math.Abs(float64(sh)*c)))
	// copia lineare RGB dell'origine (accesso veloce)
	src := make([]uint8, sw*sh*3)
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := (y*sw + x) * 3
			src[i], src[i+1], src[i+2] = uint8(r>>8), uint8(g>>8), uint8(bl>>8)
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	scx, scy := float64(sw)/2, float64(sh)/2
	dcx, dcy := float64(dw)/2, float64(dh)/2
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			dx, dy := float64(x)+0.5-dcx, float64(y)+0.5-dcy
			// posizione sorgente = rotazione inversa
			sx := dx*c + dy*s + scx - 0.5
			sy := -dx*s + dy*c + scy - 0.5
			x0, y0 := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-float64(x0), sy-float64(y0)
			var acc [3]float64
			for j := 0; j < 2; j++ {
				for i := 0; i < 2; i++ {
					wgt := (1 - math.Abs(float64(i)-fx)) * (1 - math.Abs(float64(j)-fy))
					px, py := x0+i, y0+j
					if px < 0 || py < 0 || px >= sw || py >= sh {
						acc[0] += wgt * 255
						acc[1] += wgt * 255
						acc[2] += wgt * 255
						continue
					}
					k := (py*sw + px) * 3
					acc[0] += wgt * float64(src[k])
					acc[1] += wgt * float64(src[k+1])
					acc[2] += wgt * float64(src[k+2])
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(acc[0] + 0.5), uint8(acc[1] + 0.5), uint8(acc[2] + 0.5), 255})
		}
	}
	return dst
}

// ShearRows raddrizza righe di testo inclinate di deg gradi (positivo = righe che scendono verso destra, come
// EstimateRowsSkew) facendo scorrere in verticale ogni colonna di un numero intero di pixel. Per angoli piccoli
// (qualche grado) equivale a una rotazione ma non interpola nulla: i tratti restano nitidi e di spessore invariato,
// mentre RotateAngle li sfoca e li ingrossa (misurato: spessore/altezza 0,125 -> 0,212 e lettura peggiore sul
// facsimile panama.png). I gradini di 1 px ogni 1/tan(deg) colonne non disturbano l'OCR. Il risultato e' piu' alto
// dell'originale (margine bianco sopra e sotto per lo scorrimento).
func ShearRows(img image.Image, deg float64) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	t := math.Tan(deg * math.Pi / 180)
	margin := int(math.Ceil(math.Abs(t)*float64(w)/2)) + 1
	out := image.NewRGBA(image.Rect(0, 0, w, h+2*margin))
	for i := range out.Pix {
		out.Pix[i] = 255
	}
	for x := 0; x < w; x++ {
		dy := int(math.Round((float64(x) - float64(w)/2) * t)) // di quanto il testo e' piu' in basso a questa colonna
		for y := 0; y < h; y++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			out.SetRGBA(x, y+margin-dy, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), 255})
		}
	}
	return out
}

// Deskew raddrizza la pagina se l'inclinazione stimata supera DeskewMinAngle. Restituisce l'immagine (l'originale
// se non serve ruotare) e l'angolo stimato (gradi, vedi EstimateSkew), 0 se non ha ruotato. Pensata per essere
// chiamata UNA volta sull'immagine base, prima delle rotazioni di 90 gradi.
func Deskew(img image.Image) (image.Image, float64) {
	a := EstimateSkew(img)
	if math.Abs(a) <= DeskewMinAngle {
		return img, 0
	}
	return RotateAngle(img, -a), a
}
