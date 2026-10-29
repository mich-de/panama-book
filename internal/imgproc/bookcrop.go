package imgproc

// Ritaglio dell'intero passaporto aperto (informativa + pagina dati) da un foglio A4, senza OCR.
// Il libretto aperto con il dorso in orizzontale e' alto due pagine TD3: 2 x 88 = 176 mm, largo 125 mm.
// La pagina dati sta in basso (MRZ sotto), l'informativa sopra: si parte dalla geometria della pagina dati
// (DataPageGeometry) e si estende verso l'alto di 88 mm, poi si cerca il bordo superiore reale.

import (
	"image"
	"image/color"
	"math"
)

const (
	bookH = 2 * td3PageH // altezza del libretto aperto (mm)

	bookTopWinMM     = 14.0 // il bordo superiore si cerca entro +-bookTopWinMM dalla posizione attesa
	bookHeightTol    = 0.06 // tolleranza sull'altezza del libretto (+-6% di 176 mm)
	bookOutsideSlack = 3.0  // mm di rettangolo ammessi fuori dall'immagine (bordo tagliato dallo scanner)
	bookTopCheckMM   = 8.0  // la ricerca del bordo si ferma a questa distanza sotto la posizione attesa
)

// BookGeometry descrive il rettangolo del libretto aperto.
type BookGeometry struct {
	Page  PageGeometry    // geometria della sola pagina dati da cui si parte
	Rect  image.Rectangle // libretto intero, nelle coordinate dell'immagine (non limitato ai bordi)
	EdgeT bool            // bordo superiore trovato nell'immagine (altrimenti ricavato da 2 x 88 mm)
}

// BookGeometryOf trova l'MRZ e calcola il rettangolo dell'intero libretto. img deve essere gia' orientata
// (testo dritto, pagina dati in basso, informativa sopra). ok=false se non c'e' una coppia di righe MRZ.
func BookGeometryOf(img image.Image) (BookGeometry, bool) {
	g, ok := DataPageGeometry(img)
	if !ok {
		return BookGeometry{}, false
	}
	s := g.Scale
	bottom := g.Rect.Max.Y
	top := bottom - roundInt(bookH*s)
	bg := BookGeometry{Page: g}
	if t, found := findBookTop(img, g, top); found {
		// l'altezza trovata deve essere coerente con 2 pagine, altrimenti non e' il bordo del libretto
		want := bookH * s
		if math.Abs(float64(bottom-t)-want) <= bookHeightTol*want {
			top, bg.EdgeT = t, true
		}
	}
	bg.Rect = image.Rect(g.Rect.Min.X, top, g.Rect.Max.X, bottom)
	return bg, true
}

// findBookTop cerca, attorno a expected, la prima riga di libretto sotto il fondo chiaro e piatto dello scanner.
// Si guardano le colonne centrali (l'80% della larghezza della pagina dati).
func findBookTop(img image.Image, g PageGeometry, expected int) (int, bool) {
	b := img.Bounds()
	rd := rgbReader(img)
	w := g.Rect.Dx()
	xa, xb := max(0, g.Rect.Min.X+w/10), min(b.Dx(), g.Rect.Max.X-w/10)
	if xb-xa <= 10 {
		return 0, false
	}
	rowStat := func(y int) (mean, tex float64) {
		n, prev := 0, -1
		for x := xa; x < xb; x += 2 {
			r, gg, bb := rd(b.Min.X+x, b.Min.Y+y)
			l := lum(r, gg, bb)
			mean += float64(l)
			if prev >= 0 {
				tex += float64(abs(l - prev))
			}
			prev = l
			n++
		}
		return mean / float64(n), tex / float64(n)
	}
	rowBG := func(y int) bool {
		m, t := rowStat(y)
		return m >= bgMinLum && t < 2*bgMaxTex
	}
	rowPage := func(y int) bool {
		m, t := rowStat(y)
		return m < pageMaxLum || t >= pageMinTex
	}
	s := g.Scale
	return findEdge(expected-int(bookTopWinMM*s), expected+int(bookTopCheckMM*s), b.Dy(), +1, rowBG, rowPage)
}

// fits dice se il rettangolo sta nell'immagine, con la tolleranza bookOutsideSlack (mm) per lato. Serve a
// scartare le orientazioni sbagliate: con il libretto capovolto l'MRZ e' in alto e il rettangolo uscirebbe dal foglio.
func (bg BookGeometry) fits(b image.Rectangle) bool {
	sl := int(bookOutsideSlack * bg.Page.Scale)
	return bg.Rect.Min.X >= b.Min.X-sl && bg.Rect.Min.Y >= b.Min.Y-sl &&
		bg.Rect.Max.X <= b.Max.X+sl && bg.Rect.Max.Y <= b.Max.Y+sl
}

// BookRect trova il libretto aperto nell'immagine cosi' com'e' (senza raddrizzare ne' ruotare): se il foglio e' dritto e il
// libretto e' in piedi, il rettangolo e' preciso al pixel; altrimenti ok=false. Coordinate dell'immagine.
func BookRect(img image.Image) (image.Rectangle, bool) {
	bg, ok := BookGeometryOf(img)
	if !ok || !bg.fits(img.Bounds()) {
		return image.Rectangle{}, false
	}
	return bg.Rect.Intersect(img.Bounds()), true
}

// BookFrame dice quanto foglio lasciare attorno al libretto. Con CanvasW e CanvasH (mm) il risultato e' un riquadro di quella
// misura centrato sul libretto (es. 140 x 187 mm = proporzione 3:4): dove il riquadro esce dal foglio si aggiunge bianco, cosi'
// la misura e' sempre la stessa. Senza, si lasciano MarginMM mm per lato, limitati al foglio.
type BookFrame struct {
	MarginMM         float64
	CanvasW, CanvasH float64
}

// rect calcola il riquadro (coordinate dell'immagine, non limitato ai bordi) attorno al libretto.
func (f BookFrame) rect(bg BookGeometry) image.Rectangle {
	s := bg.Page.Scale
	if f.CanvasW > 0 && f.CanvasH > 0 {
		cx, cy := (bg.Rect.Min.X+bg.Rect.Max.X)/2, (bg.Rect.Min.Y+bg.Rect.Max.Y)/2
		w, h := roundInt(f.CanvasW*s), roundInt(f.CanvasH*s)
		return image.Rect(cx-w/2, cy-h/2, cx-w/2+w, cy-h/2+h)
	}
	m := roundInt(f.MarginMM * s)
	return image.Rect(bg.Rect.Min.X-m, bg.Rect.Min.Y-m, bg.Rect.Max.X+m, bg.Rect.Max.Y+m)
}

// CropBook ritaglia il libretto aperto da un'immagine gia' orientata, con marginMM di carta attorno (limitato ai
// bordi dell'immagine). Ritorna (nil, false) se non trova l'MRZ o se il libretto non sta nell'immagine.
func CropBook(img image.Image, marginMM float64) (image.Image, bool) {
	return CropBookFrame(img, BookFrame{MarginMM: marginMM})
}

// CropBookFrame e' CropBook con la cornice a scelta (margine o riquadro di misura fissa).
func CropBookFrame(img image.Image, f BookFrame) (image.Image, bool) {
	bg, ok := BookGeometryOf(img)
	if !ok || !bg.fits(img.Bounds()) {
		return nil, false
	}
	return cropBookGeom(img, bg, f)
}

// cropBookGeom ritaglia img secondo la geometria gia' trovata.
func cropBookGeom(img image.Image, bg BookGeometry, f BookFrame) (image.Image, bool) {
	b := img.Bounds()
	r := f.rect(bg).Add(b.Min)
	if f.CanvasW > 0 && f.CanvasH > 0 {
		return copyRectPadded(img, r), true
	}
	if r = r.Intersect(b); r.Empty() {
		return nil, false
	}
	return copyRect(img, r), true
}

// copyRectPadded copia il rettangolo r (coordinate assolute di src) in una nuova immagine con origine in (0,0); la parte
// di r fuori da src e' bianca.
func copyRectPadded(src image.Image, r image.Rectangle) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for i := range dst.Pix {
		dst.Pix[i] = 255
	}
	in := r.Intersect(src.Bounds())
	if in.Empty() {
		return dst
	}
	rd := rgbReader(src)
	for y := in.Min.Y; y < in.Max.Y; y++ {
		for x := in.Min.X; x < in.Max.X; x++ {
			R, G, B := rd(x, y)
			dst.SetRGBA(x-r.Min.X, y-r.Min.Y, color.RGBA{R, G, B, 255})
		}
	}
	return dst
}

// CropBookOriented porta a dritto il foglio (CropContent, raddrizzamento, poi le 4 rotazioni come libreta.Extract:
// 0, 180, 90, 270) e ritaglia il libretto aperto nella prima orientazione in cui MRZ e rettangolo sono coerenti.
// Restituisce anche la rotazione usata. ok=false se in nessuna orientazione il libretto sta nel foglio.
func CropBookOriented(src image.Image, marginMM float64) (out image.Image, rotation int, ok bool) {
	return CropBookOrientedFrame(src, BookFrame{MarginMM: marginMM})
}

// CropBookOrientedFrame e' CropBookOriented con la cornice a scelta.
func CropBookOrientedFrame(src image.Image, f BookFrame) (out image.Image, rotation int, ok bool) {
	base, _ := Deskew(CropContent(src, 10))
	for _, rot := range []int{0, 180, 90, 270} {
		img := Rotate(base, rot)
		bg, ok := BookGeometryOf(img)
		if !ok || !bg.fits(img.Bounds()) {
			continue
		}
		// inclinazione residua (sotto la soglia di Deskew): si misura sulle righe MRZ e si raddrizza il libretto. Il rettangolo
		// si sposta con la rotazione (l'MRZ non si rilegge: l'interpolazione sfoca le righe e non si troverebbero piu').
		if t := mrzTilt(img); math.Abs(t) >= bookMinTilt && math.Abs(t) <= bookMaxTilt {
			rot2 := RotateAngle(img, -t)
			bg.Rect = rotatedRect(bg.Rect, img.Bounds(), rot2.Bounds(), -t)
			img = rot2
		}
		if c, ok := cropBookGeom(img, bg, f); ok {
			return c, rot, true
		}
	}
	return nil, 0, false
}

// rotatedRect da' dove va a finire il rettangolo r (stessa misura, stesso centro ruotato) quando RotateAngle ruota di deg gradi
// l'immagine di dimensioni from, producendo l'immagine di dimensioni to.
func rotatedRect(r, from, to image.Rectangle, deg float64) image.Rectangle {
	s, c := math.Sincos(deg * math.Pi / 180)
	ux := float64(r.Min.X+r.Max.X)/2 - float64(from.Min.X) - float64(from.Dx())/2
	uy := float64(r.Min.Y+r.Max.Y)/2 - float64(from.Min.Y) - float64(from.Dy())/2
	cx := ux*c - uy*s + float64(to.Min.X) + float64(to.Dx())/2
	cy := ux*s + uy*c + float64(to.Min.Y) + float64(to.Dy())/2
	x0, y0 := roundInt(cx)-r.Dx()/2, roundInt(cy)-r.Dy()/2
	return image.Rect(x0, y0, x0+r.Dx(), y0+r.Dy())
}

// bookMinTilt: inclinazione (gradi) delle righe MRZ da cui il ritaglio del libretto lo raddrizza.
const (
	bookMinTilt = 0.3
	bookMaxTilt = 5.0
)

// mrzTilt misura l'inclinazione (gradi, positiva se le righe scendono verso destra) delle due righe MRZ di un'immagine gia'
// orientata; 0 se non le trova.
func mrzTilt(img image.Image) float64 {
	b := img.Bounds()
	if b.Dx() < 300 || b.Dy() < 100 {
		return 0
	}
	bin := BinarizeDoc(img)
	ClearLines(bin)
	pairs := MRZPairs(TrimRowEdgeBars(bin, TextRows(bin, 0.35)), b.Dx())
	if len(pairs) == 0 {
		return 0
	}
	return EstimateRowsSkew(bin, pairs[0][0], pairs[0][1])
}
