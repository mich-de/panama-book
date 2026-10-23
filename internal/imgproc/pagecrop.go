package imgproc

// Ritaglio della pagina dati TD3 dalla geometria delle due righe MRZ (senza OCR).

import (
	"image"
)

// Geometria ICAO 9303 TD3 (mm).
const (
	td3PageW    = 125.0 // larghezza pagina dati
	td3PageH    = 88.0  // altezza pagina dati
	td3Pitch    = 2.54  // passo dei caratteri OCR-B (10 car/pollice)
	td3Chars    = 44    // caratteri per riga
	td3InkSlack = 0.6   // luce laterale dei glifi: larghezza inchiostro = 44*passo - td3InkSlack

	// td3BottomMM e' la distanza tra il fondo della seconda riga MRZ (inchiostro) e il bordo inferiore della pagina.
	// Calibrata sulla scansione reale (bordo della pagina a 7,0-8,4 mm, linea scura del bordo inclusa) e sulle
	// 4 immagini dei facsimili (7,2-8,5 mm); 5 mm lasciava ~2,5 mm di carta in piu'.
	td3BottomMM = 7.5
)

// PageGeometry descrive il rettangolo della pagina dati ricavato dalle righe MRZ.
type PageGeometry struct {
	Rect   image.Rectangle // rettangolo (non limitato ai bordi dell'immagine) nelle coordinate dell'immagine
	Scale  float64         // pixel per millimetro
	R1, R2 Row             // le due righe MRZ usate

	// Bordi della pagina trovati nell'immagine (contro il fondo piatto e chiaro dello scanner) e usati per il
	// rettangolo; i bordi non trovati sono ricavati dalla geometria dell'MRZ.
	EdgeL, EdgeR, EdgeB bool
}

// DataPageGeometry trova le due righe MRZ piu' in basso e calcola il rettangolo della pagina dati.
// img deve essere gia' orientata (testo dritto). ok=false se non c'e' una coppia di righe compatibile.
func DataPageGeometry(img image.Image) (PageGeometry, bool) {
	b := img.Bounds()
	if b.Dx() < 300 || b.Dy() < 100 {
		return PageGeometry{}, false
	}
	bin := Binarize(img)
	ClearLines(bin)
	pairs := MRZPairs(TrimRowEdgeBars(bin, TextRows(bin, 0.35)), b.Dx())
	if len(pairs) == 0 {
		return PageGeometry{}, false
	}
	g, ok := geometryFromRows(pairs[0][0], pairs[0][1])
	if !ok {
		return g, false
	}
	return refineWithEdges(img, g), true
}

// geometryFromRows applica la geometria TD3 alle due righe MRZ (coordinate relative all'origine dell'immagine).
func geometryFromRows(r1, r2 Row) (PageGeometry, bool) {
	x0, x1 := min(r1.X0, r2.X0), max(r1.X1, r2.X1)
	w := float64(x1 - x0)
	if w <= 0 {
		return PageGeometry{}, false
	}
	s := w / (td3Chars*td3Pitch - td3InkSlack) // px/mm
	// margine laterale: l'inchiostro inizia td3InkSlack/2 dopo il bordo della prima cella
	side := (td3PageW-td3Chars*td3Pitch)/2 + td3InkSlack/2
	left := float64(x0) - side*s
	bottom := float64(r2.Y1) + td3BottomMM*s
	rect := image.Rect(roundInt(left), roundInt(bottom-td3PageH*s), roundInt(left+td3PageW*s), roundInt(bottom))
	return PageGeometry{Rect: rect, Scale: s, R1: r1, R2: r2}, true
}

func roundInt(f float64) int {
	if f < 0 {
		return -int(-f + 0.5)
	}
	return int(f + 0.5)
}

// CropDataPage ritaglia la pagina dati TD3 (125x88 mm) da una immagine gia' orientata (testo dritto, MRZ in
// basso), usando solo la posizione delle due righe MRZ. Il rettangolo e' limitato ai bordi dell'immagine.
// Se non trova una coppia di righe MRZ valida ritorna (nil, false).
func CropDataPage(img image.Image) (image.Image, bool) {
	g, ok := DataPageGeometry(img)
	if !ok {
		return nil, false
	}
	b := img.Bounds()
	r := g.Rect.Add(b.Min).Intersect(b)
	if r.Empty() {
		return nil, false
	}
	return copyRect(img, r), true
}

// Soglie del fondo dello scanner: chiaro (~240, mai bianco puro) e piatto. La pagina del libretto e' piu' scura
// (~190) e con testura; i facsimili digitali hanno fondo bianco pieno (255, testura 0).
const (
	bgMinLum   = 232  // luminosita' media minima del fondo
	bgMaxTex   = 3.0  // testura massima del fondo (media delle differenze tra pixel vicini)
	edgeWinMM  = 14   // i bordi laterali si cercano entro +-edgeWinMM dalla posizione attesa
	edgeWidthT = 0.08 // tolleranza sulla larghezza della pagina (+-8% di 125 mm)
	// una colonna e' di pagina (e non solo ombra del bordo, ~220 e piatta) se e' scura o con testura
	pageMaxLum = 218
	pageMinTex = 5.0
)

func lum(r, g, b uint8) int { return (int(r)*299 + int(g)*587 + int(b)*114) / 1000 }

// refineWithEdges cerca i bordi reali della pagina (sinistro, destro, inferiore) dove il fondo piatto e chiaro
// dello scanner incontra la pagina e, se sono visibili e coerenti con 125 mm di larghezza (+-8%), allinea il
// rettangolo ad essi. Altrimenti lascia la geometria dell'MRZ. La scala diventa quella misurata sulla pagina.
func refineWithEdges(img image.Image, g PageGeometry) PageGeometry {
	b := img.Bounds()
	rd := rgbReader(img)
	s := g.Scale
	// fascia verticale interna alla pagina su cui misurare la testura delle colonne
	yA, yB := max(0, g.R2.Y1-int(70*s)), min(b.Dy(), g.R2.Y1)
	// colonne (coordinate relative all'immagine; rgbReader usa coordinate assolute)
	colStat := func(x int) (mean, tex float64, ok bool) {
		n, prev := 0, -1
		for y := yA; y < yB; y += 2 {
			r, gg, bb := rd(b.Min.X+x, b.Min.Y+y)
			l := lum(r, gg, bb)
			mean += float64(l)
			if prev >= 0 {
				tex += float64(abs(l - prev))
			}
			prev = l
			n++
		}
		if n <= 10 {
			return 0, 0, false
		}
		return mean / float64(n), tex / float64(n), true
	}
	colBG := func(x int) bool {
		m, t, ok := colStat(x)
		return ok && m >= bgMinLum && t < bgMaxTex
	}
	colPage := func(x int) bool {
		m, t, ok := colStat(x)
		return ok && (m < pageMaxLum || t >= pageMinTex)
	}
	margin := int(edgeWinMM * s)
	var left, right int
	okL, okR := false, false
	if yB-yA >= int(20*s) {
		left, okL = findEdge(g.Rect.Min.X-margin, g.Rect.Min.X+int(8*s), b.Dx(), +1, colBG, colPage)
		right, okR = findEdge(g.Rect.Max.X+margin, g.Rect.Max.X-int(8*s), b.Dx(), -1, colBG, colPage)
	}
	if okL && okR {
		if w := right - left; abs(w-int(td3PageW*s+0.5)) > int(edgeWidthT*td3PageW*s) {
			okL, okR = false, false // larghezza incoerente: non sono i bordi della pagina
		}
	}
	sx := s
	l, r := g.Rect.Min.X, g.Rect.Max.X
	switch {
	case okL && okR:
		l, r = left, right
		sx = float64(r-l) / td3PageW
	case okL:
		l = left
		r = l + roundInt(td3PageW*s)
	case okR:
		r = right
		l = r - roundInt(td3PageW*s)
	}
	// bordo inferiore: prima riga chiara e piatta sotto l'MRZ. Si cerca tra 4,5 e 11 mm dal fondo della seconda
	// riga (bordo misurato: 5,8-9 mm) e solo se a 4,5 mm c'e' ancora carta (altrimenti un margine bianco della
	// pagina sarebbe scambiato per fondo).
	bottom, okB := g.Rect.Max.Y, false
	xa, xb := g.R1.X0+(g.R1.X1-g.R1.X0)/10, g.R1.X1-(g.R1.X1-g.R1.X0)/10
	rowBG := func(y int) bool {
		var sum, tex float64
		n, prev := 0, -1
		for x := xa; x < xb; x += 2 {
			rr, gg, bb := rd(b.Min.X+x, b.Min.Y+y)
			v := lum(rr, gg, bb)
			sum += float64(v)
			if prev >= 0 {
				tex += float64(abs(v - prev))
			}
			prev = v
			n++
		}
		return n > 10 && sum/float64(n) >= bgMinLum && tex/float64(n) < 2*bgMaxTex
	}
	if y0 := g.R2.Y1 + int(4.5*s); y0 < b.Dy() && !rowBG(y0) {
		for y := y0 + 1; y < g.R2.Y1+int(11*s) && y < b.Dy(); y++ {
			if rowBG(y) {
				bottom, okB = y, true
				break
			}
		}
	}
	h := roundInt(td3PageH * sx)
	if okL || okR {
		sx = float64(r-l) / td3PageW
		h = roundInt(td3PageH * sx)
	}
	g.Rect = image.Rect(l, bottom-h, r, bottom)
	g.Scale = sx
	g.EdgeL, g.EdgeR, g.EdgeB = okL, okR, okB
	return g
}

// findEdge cerca il bordo di una pagina su un fondo chiaro e piatto. Si parte da from (fuori dalla pagina) e si va
// verso to con passo dir (+1 o -1): le prime colonne devono essere fondo (altrimenti il bordo non e' visibile: la
// pagina arriva al margine dell'immagine o il fondo non e' piatto), poi il primo punto in cui compaiono almeno 3
// colonne consecutive di pagina (page: scure o con testura, non la sola ombra del bordo) e' il bordo. Restituisce la coordinata del bordo (per dir=+1 la prima colonna
// della pagina, per dir=-1 la prima colonna dopo la pagina).
func findEdge(from, to, size, dir int, bg, page func(x int) bool) (int, bool) {
	clamp := func(v int) int { return max(0, min(size-1, v)) }
	from, to = clamp(from), clamp(to)
	if (to-from)*dir <= 6 {
		return 0, false
	}
	for k := 0; k < 3; k++ { // la finestra deve iniziare sul fondo
		if !bg(from + k*dir) {
			return 0, false
		}
	}
	run := 0
	for x := from; x != to+dir; x += dir {
		if !page(x) {
			run = 0
			continue
		}
		run++
		if run == 3 {
			first := x - 2*dir
			if dir > 0 {
				return first, true
			}
			return first + 1, true
		}
	}
	return 0, false
}
