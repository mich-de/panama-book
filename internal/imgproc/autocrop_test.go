package imgproc

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"
)

// Tutte le immagini sono sintetiche: nessun dato personale.

const (
	pageW, pageH = 1275, 2100 // 8,5x14" a 150 dpi
	docW, docH   = 700, 500   // documento (tipo libretto aperto: ~11,7x8,3 cm a 150 dpi non serve la precisione)
)

type docSpec struct {
	cx, cy   float64 // centro del documento sulla pagina
	angleDeg float64 // rotazione oraria del documento
	paper    uint8   // grigio della carta
	noise    int     // ampiezza del rumore uniforme +-noise su ogni pixel
	specks   int     // puntini scuri casuali sullo sfondo
	text     bool    // righe di "testo" scuro sul documento
}

// render disegna pagina bianca con un documento rettangolare (eventualmente inclinato) e rumore.
func render(s docSpec, seed int64) *image.RGBA {
	rng := rand.New(rand.NewSource(seed))
	img := image.NewRGBA(image.Rect(0, 0, pageW, pageH))
	sn, cs := math.Sincos(s.angleDeg * math.Pi / 180)
	for y := 0; y < pageH; y++ {
		for x := 0; x < pageW; x++ {
			dx, dy := float64(x)-s.cx, float64(y)-s.cy
			qx, qy := cs*dx+sn*dy, -sn*dx+cs*dy // coordinate nel riferimento del documento
			v := 255
			if math.Abs(qx) < docW/2 && math.Abs(qy) < docH/2 {
				v = int(s.paper)
				if s.text && int(qy+docH)%40 < 12 && math.Abs(qx) < docW/2-40 {
					v = 60
				}
			}
			if s.noise > 0 {
				v += rng.Intn(2*s.noise+1) - s.noise
			}
			if v < 0 {
				v = 0
			}
			if v > 255 {
				v = 255
			}
			u := uint8(v)
			img.SetRGBA(x, y, color.RGBA{u, u, u, 255})
		}
	}
	for i := 0; i < s.specks; i++ {
		px, py := rng.Intn(pageW-6), rng.Intn(pageH-6)
		for dy := 0; dy < 4; dy++ {
			for dx := 0; dx < 4; dx++ {
				img.SetRGBA(px+dx, py+dy, color.RGBA{30, 30, 30, 255})
			}
		}
	}
	return img
}

func near(got, want, tol int) bool { return got >= want-tol && got <= want+tol }

func TestAutoCropClean(t *testing.T) {
	src := render(docSpec{cx: 600, cy: 900, paper: 180}, 1)
	out, info := AutoCropInfo(src)
	if info.Blank || info.SkewDeg != 0 {
		t.Fatalf("info inattesa: %+v", info)
	}
	b := out.Bounds()
	margin := testMargin()
	if !near(b.Dx(), docW+2*margin, 10) || !near(b.Dy(), docH+2*margin, 10) {
		t.Errorf("dimensioni %dx%d, attese ~%dx%d", b.Dx(), b.Dy(), docW+2*margin, docH+2*margin)
	}
	if b.Min != (image.Point{}) {
		t.Errorf("l'origine deve essere (0,0): %v", b.Min)
	}
	// il riquadro riportato e' nelle coordinate originali e centrato sul documento
	c := image.Pt((info.Box.Min.X+info.Box.Max.X)/2, (info.Box.Min.Y+info.Box.Max.Y)/2)
	if !near(c.X, 600, 6) || !near(c.Y, 900, 6) {
		t.Errorf("centro del riquadro %v, atteso (600,900)", c)
	}
}

func TestAutoCropNoisy(t *testing.T) {
	src := render(docSpec{cx: 500, cy: 1200, paper: 175, noise: 14, specks: 40}, 2)
	out, info := AutoCropInfo(src)
	if info.Blank {
		t.Fatal("documento con rumore scambiato per pagina vuota")
	}
	b := out.Bounds()
	margin := testMargin()
	if !near(b.Dx(), docW+2*margin, 16) || !near(b.Dy(), docH+2*margin, 16) {
		t.Errorf("dimensioni %dx%d, attese ~%dx%d (rumore/puntini non devono allargare il ritaglio)",
			b.Dx(), b.Dy(), docW+2*margin, docH+2*margin)
	}
}

func TestAutoCropSkew(t *testing.T) {
	for _, ang := range []float64{3, -4, 1.5} {
		src := render(docSpec{cx: 640, cy: 1000, angleDeg: ang, paper: 185, noise: 6, text: true}, 3)
		out, info := AutoCropInfo(src)
		if info.Blank {
			t.Fatalf("%v gradi: pagina vuota", ang)
		}
		if math.Abs(info.SkewDeg+ang) > 0.8 {
			t.Errorf("inclinazione %v gradi: correzione stimata %v (attesa ~%v)", ang, info.SkewDeg, -ang)
		}
		b := out.Bounds()
		margin := testMargin()
		// senza raddrizzamento il riquadro sarebbe largo docW*cos+docH*sin: molto di piu' di docW
		if !near(b.Dx(), docW+2*margin, 25) || !near(b.Dy(), docH+2*margin, 25) {
			t.Errorf("%v gradi: dimensioni %dx%d, attese ~%dx%d", ang, b.Dx(), b.Dy(), docW+2*margin, docH+2*margin)
		}
	}
}

func TestAutoCropSkewBeatsCropContent(t *testing.T) {
	src := render(docSpec{cx: 640, cy: 1000, angleDeg: 4, paper: 185, text: true}, 4)
	plain := CropContent(src, 0).Bounds()
	auto := AutoCrop(src).Bounds()
	if auto.Dx()*auto.Dy() >= plain.Dx()*plain.Dy() {
		t.Errorf("AutoCrop (%v) non migliora CropContent (%v) su un documento inclinato", auto, plain)
	}
}

func TestAutoCropTextOnlyContent(t *testing.T) {
	// carta quasi bianca (scarto 10 < soglia): si vedono solo le righe di testo
	src := render(docSpec{cx: 640, cy: 1000, paper: 245, text: true}, 5)
	out, info := AutoCropInfo(src)
	if info.Blank {
		t.Fatal("testo non rilevato")
	}
	b := out.Bounds()
	if b.Dx() < docW-100 || b.Dx() > docW+40 || b.Dy() < docH-120 || b.Dy() > docH+40 {
		t.Errorf("dimensioni %dx%d lontane dal blocco di testo ~%dx%d", b.Dx(), b.Dy(), docW-80, docH-30)
	}
}

func TestIsBlank(t *testing.T) {
	if !IsBlank(nil) {
		t.Error("nil deve essere vuota")
	}
	if !IsBlank(image.NewRGBA(image.Rect(0, 0, 4, 4))) {
		t.Error("immagine minuscola deve essere vuota")
	}
	blankClean := image.NewRGBA(image.Rect(0, 0, pageW, pageH))
	for i := range blankClean.Pix {
		blankClean.Pix[i] = 255
	}
	if !IsBlank(blankClean) {
		t.Error("pagina bianca pulita non riconosciuta come vuota")
	}
	// bianco con rumore e polvere, nessun documento: documento spostato fuori pagina
	noisy := render(docSpec{cx: -5000, cy: -5000, paper: 180, noise: 12, specks: 60}, 7)
	if !IsBlank(noisy) {
		t.Error("pagina con solo rumore e polvere non riconosciuta come vuota")
	}
	// riga verticale sottile (ombra del rullo) non e' un documento
	line := image.NewRGBA(image.Rect(0, 0, pageW, pageH))
	for i := range line.Pix {
		line.Pix[i] = 255
	}
	for y := 0; y < pageH; y++ {
		for x := 600; x < 606; x++ {
			line.SetRGBA(x, y, color.RGBA{90, 90, 90, 255})
		}
	}
	if !IsBlank(line) {
		t.Error("una riga sottile non e' un documento")
	}
	doc := render(docSpec{cx: 600, cy: 900, paper: 180, noise: 10}, 8)
	if IsBlank(doc) {
		t.Error("pagina con documento riconosciuta come vuota")
	}
}

func TestAutoCropBlankReturnsOriginal(t *testing.T) {
	blank := image.NewRGBA(image.Rect(0, 0, 200, 300))
	for i := range blank.Pix {
		blank.Pix[i] = 255
	}
	out, info := AutoCropInfo(blank)
	if !info.Blank || out != image.Image(blank) {
		t.Error("su pagina vuota deve restituire l'immagine originale e Blank=true")
	}
}

func TestAutoCropGrayAndSubImage(t *testing.T) {
	rgba := render(docSpec{cx: 600, cy: 900, paper: 170, noise: 5}, 9)
	g := image.NewGray(rgba.Bounds())
	for y := 0; y < pageH; y++ {
		for x := 0; x < pageW; x++ {
			g.SetGray(x, y, color.Gray{rgba.RGBAAt(x, y).R})
		}
	}
	margin := testMargin()
	out := AutoCrop(g).Bounds()
	if !near(out.Dx(), docW+2*margin, 10) || !near(out.Dy(), docH+2*margin, 10) {
		t.Errorf("Gray: %v", out)
	}
	// SubImage: coordinate con origine non nulla
	sub := rgba.SubImage(image.Rect(100, 200, 1100, 1700))
	out = AutoCrop(sub).Bounds()
	if !near(out.Dx(), docW+2*margin, 10) || !near(out.Dy(), docH+2*margin, 10) {
		t.Errorf("SubImage: %v", out)
	}
}

func TestEstimateSkew(t *testing.T) {
	// rettangolo pieno inclinato di 2 gradi in senso orario: la correzione deve essere ~ -2
	var pts []image.Point
	sn, cs := math.Sincos(2 * math.Pi / 180)
	for y := 0; y < 400; y++ {
		for x := 0; x < 250; x++ {
			dx, dy := float64(x-125), float64(y-200)
			pts = append(pts, image.Pt(int(250+cs*dx-sn*dy), int(300+sn*dx+cs*dy)))
		}
	}
	if got := estimateSkew(pts); math.Abs(got+2) > 0.6 {
		t.Errorf("stima %v, attesa ~-2", got)
	}
	if got := estimateSkew(nil); got != 0 {
		t.Errorf("senza punti: %v", got)
	}
}

// testMargin e' il margine atteso da AutoCrop su una pagina di pageH pixel di lato maggiore.
func testMargin() int {
	longSide := float64(pageH)
	return int(acMarginFrac*longSide) + 1
}

// renderGrayBG: fondo grigio chiaro con gradiente orizzontale, documento texturato centrale che occupa
// tutta l'altezza (come un libretto vero), riga scura sottile sul bordo destro e ombra liscia in basso.
func renderGrayBG(w, h, docX0, docX1, lineW int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(7))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := 237 + 7*x/w + rng.Intn(3) - 1 // 237..244: gradiente leggero con un po' di rumore
			if y >= h-30 {                     // cuneo d'ombra liscio in basso: rampa fino a ~24 livelli, senza texture
				v -= (y - (h - 30)) * 24 / 30
			}
			if x >= docX0 && x < docX1 {
				v = 250
				if (y/9)%3 == 0 && (x/5)%4 != 0 { // righe di "testo"
					v = 40 + rng.Intn(60)
				}
			}
			if x >= w-lineW { // artefatto scuro sul bordo estremo
				v = 50
			}
			u := uint8(v)
			img.SetRGBA(x, y, color.RGBA{u, u, u, 255})
		}
	}
	return img
}

func TestAutoCropGrayGradientBackground(t *testing.T) {
	const w, h, x0, x1 = 1500, 1000, 450, 1050
	for _, lineW := range []int{0, 2, 4} {
		src := renderGrayBG(w, h, x0, x1, lineW)
		if IsBlank(src) {
			t.Fatalf("riga %d px: documento scambiato per vuoto", lineW)
		}
		_, info := AutoCropInfo(src)
		b := info.Box
		if !near(b.Min.X, x0, 25) || !near(b.Max.X, x1, 25) {
			t.Errorf("riga %d px: ritaglio x %d-%d, atteso %d-%d (scarto max 25 px)", lineW, b.Min.X, b.Max.X, x0, x1)
		}
	}
}

func TestAutoCropIgnoresEdgeLineOnly(t *testing.T) {
	// nessun documento: solo fondo grigio con gradiente, ombra e riga scura al bordo = pagina vuota
	src := renderGrayBG(1500, 1000, 0, 0, 3)
	if !IsBlank(src) {
		t.Error("fondo grigio con riga sul bordo scambiato per documento")
	}
}
