package wia

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestAreaPixelsCentraturaELimiti(t *testing.T) {
	// campo reale a 300 dpi: 2550x4200 px
	const maxW, maxH = 2550, 4200
	cases := []struct {
		nome       string
		w, h       float64
		dpi        int
		extX, extY int
	}{
		{"area massima (0,0)", 0, 0, 300, maxW, maxH},
		{"solo altezza 100 mm", 0, 100, 300, maxW, 1181},   // 100/25.4*300 = 1181,1
		{"solo larghezza 100 mm", 100, 0, 300, 1181, maxH}, // start (2550-1181)/2 = 684
		{"meta' foglio", 108, 178, 300, 1276, 2102},
		{"campo intero: 216x356 mm supera di poco il driver, limitato", 216, 356, 300, maxW, maxH},
		{"un pixel", 0.0847, 0.0847, 300, 1, 1}, // 0,0847/25,4*300 = 1,0004
	}
	for _, c := range cases {
		sx, sy, ex, ey, err := areaPixels(0, c.w, c.h, c.dpi, maxW, maxH)
		if err != nil {
			t.Errorf("%s: %v", c.nome, err)
			continue
		}
		// la centratura e' sempre (maxW-extX)/2 con divisione intera, se la larghezza e' richiesta
		wantSX := 0
		if c.w > 0 {
			wantSX = (maxW - ex) / 2
		}
		if sx != wantSX {
			t.Errorf("%s: startX=%d, atteso %d", c.nome, sx, wantSX)
		}
		if sy != 0 {
			t.Errorf("%s: startY=%d senza offset", c.nome, sy)
		}
		if ex != c.extX || ey != c.extY {
			t.Errorf("%s: extent=%dx%d, attesi %dx%d", c.nome, ex, ey, c.extX, c.extY)
		}
	}
}

func TestAreaPixelsValoriNoti(t *testing.T) {
	sx, _, ex, ey, err := areaPixels(0, 108, 178, 300, 2550, 4200)
	if err != nil {
		t.Fatal(err)
	}
	// 108 mm = 1275,59 -> 1276 ; 178 mm = 2102,36 -> 2102 ; start = (2550-1276)/2 = 637
	if ex != 1276 || ey != 2102 || sx != 637 {
		t.Errorf("got start=%d ext=%dx%d", sx, ex, ey)
	}
	// start + extent non deve mai superare il campo (simmetria entro 1 pixel)
	if sx+ex > 2550 || 2550-(sx+ex)-sx > 1 {
		t.Errorf("centratura non simmetrica: start=%d ext=%d", sx, ex)
	}
	// 600 dpi: il campo raddoppia; 50 mm = 1181,1 -> 1181
	_, _, ex, _, err = areaPixels(0, 50, 0, 600, 5100, 8400)
	if err != nil || ex != 1181 {
		t.Errorf("600 dpi: ext=%d err=%v", ex, err)
	}
	// 63,5 mm = 2,5 pollici a 1 dpi = 2,5 px -> 3 (arrotondamento .5 verso l'alto)
	_, _, ex, _, err = areaPixels(0, 63.5, 0, 1, 10, 10)
	if err != nil || ex != 3 {
		t.Errorf("arrotondamento .5: ext=%d err=%v", ex, err)
	}
}

func TestAreaPixelsOffset(t *testing.T) {
	// pagina dati: da 91 a 178 mm dall'alto -> offset 91, altezza 87
	// 91 mm = 1074,8 -> 1075 px ; 87 mm = 1027,6 -> 1028 px (a 300 dpi)
	_, sy, _, ey, err := areaPixels(91, 0, 87, 300, 2550, 4200)
	if err != nil || sy != 1075 || ey != 1028 {
		t.Errorf("300 dpi: startY=%d extY=%d err=%v", sy, ey, err)
	}
	// 600 dpi: 91 mm = 2149,6 -> 2150 ; 87 mm = 2055,1 -> 2055
	_, sy, _, ey, err = areaPixels(91, 0, 87, 600, 5100, 8400)
	if err != nil || sy != 2150 || ey != 2055 {
		t.Errorf("600 dpi: startY=%d extY=%d err=%v", sy, ey, err)
	}
	// HeightMM=0 con offset: dal punto di start fino al fondo del campo
	_, sy, _, ey, err = areaPixels(91, 0, 0, 300, 2550, 4200)
	if err != nil || sy != 1075 || ey != 4200-1075 {
		t.Errorf("fino al fondo: startY=%d extY=%d err=%v", sy, ey, err)
	}
	// offset + altezza esattamente al limite del campo (356 mm): consentito, e il totale non eccede mai
	_, sy, _, ey, err = areaPixels(100, 0, 256, 300, 2550, 4200)
	if err != nil || sy+ey > 4200 {
		t.Errorf("al limite: startY=%d extY=%d err=%v", sy, ey, err)
	}
	// offset + larghezza insieme: la centratura non dipende dall'offset
	sx, sy, ex, _, err := areaPixels(91, 108, 87, 300, 2550, 4200)
	if err != nil || sy != 1075 || sx != (2550-ex)/2 {
		t.Errorf("offset con larghezza: sx=%d sy=%d ex=%d err=%v", sx, sy, ex, err)
	}
	// offset 0: invariato
	_, sy, _, ey, err = areaPixels(0, 0, 100, 300, 2550, 4200)
	if err != nil || sy != 0 || ey != 1181 {
		t.Errorf("senza offset: startY=%d extY=%d err=%v", sy, ey, err)
	}
}

func TestAreaPixelsErrori(t *testing.T) {
	bad := []struct {
		nome        string
		s, w, h     float64
		dpi, mw, mh int
	}{
		{"larghezza negativa", 0, -1, 0, 300, 2550, 4200},
		{"altezza negativa", 0, 0, -0.1, 300, 2550, 4200},
		{"offset negativo", -1, 0, 50, 300, 2550, 4200},
		{"larghezza oltre il campo", 0, 217, 0, 300, 2550, 4200},
		{"altezza oltre il campo", 0, 0, 357, 300, 2550, 4200},
		{"offset oltre il campo", 357, 0, 0, 300, 2550, 4200},
		{"offset + altezza oltre il campo", 200, 0, 157, 300, 2550, 4200},
		{"NaN", 0, math.NaN(), 0, 300, 2550, 4200},
		{"offset NaN", math.NaN(), 0, 50, 300, 2550, 4200},
		{"infinito", 0, 0, math.Inf(1), 300, 2550, 4200},
		{"dpi zero", 0, 50, 50, 0, 2550, 4200},
		{"limite larghezza sconosciuto", 0, 50, 0, 300, 0, 4200},
		{"limite altezza sconosciuto", 0, 0, 50, 300, 2550, 0},
		{"offset con limite altezza sconosciuto", 50, 0, 0, 300, 2550, 0},
		{"meno di un pixel", 0, 0.01, 0, 300, 2550, 4200},
		{"offset oltre il massimo del driver", 355.9, 0, 0, 300, 2550, 1000},
	}
	for _, c := range bad {
		if _, _, _, _, err := areaPixels(c.s, c.w, c.h, c.dpi, c.mw, c.mh); err == nil {
			t.Errorf("%s: errore atteso", c.nome)
		}
	}
	// limite sconosciuto ma asse non richiesto: nessun errore
	if _, _, _, _, err := areaPixels(0, 0, 50, 300, 0, 4200); err != nil {
		t.Errorf("asse non richiesto con limite ignoto: %v", err)
	}
}

func TestOptionsNormalizedArea(t *testing.T) {
	if _, err := (Options{OutPath: "a.png", StartMM: 91, WidthMM: 100, HeightMM: 87}).normalized(); err != nil {
		t.Errorf("area valida rifiutata: %v", err)
	}
	for _, o := range []Options{
		{OutPath: "a.png", WidthMM: -5},
		{OutPath: "a.png", HeightMM: -5},
		{OutPath: "a.png", StartMM: -5},
		{OutPath: "a.png", WidthMM: 300},
		{OutPath: "a.png", HeightMM: 400},
		{OutPath: "a.png", StartMM: 400},
		{OutPath: "a.png", StartMM: 300, HeightMM: 100},
		{OutPath: "a.png", StartMM: math.NaN()},
	} {
		if _, err := o.normalized(); err == nil {
			t.Errorf("area non valida accettata: %+v", o)
		}
	}
}

// driverFinto riproduce il comportamento misurato del driver: ogni scrittura di uno start fa
// ricalcolare l'extent dello stesso asse (valore arbitrario) e il massimo dell'extent diventa campo-start.
// Registra l'ordine delle scritture.
type driverFinto struct {
	v         map[string]int
	scritture []string
}

func nuovoDriverFinto() *driverFinto {
	return &driverFinto{v: map[string]int{
		"Vertical Start Position": 0, "Vertical Extent": 4200,
		"Horizontal Start Position": 0, "Horizontal Extent": 2550,
	}}
}

func (d *driverFinto) campo(name string) int {
	if name[0] == 'V' {
		return 4200
	}
	return 2550
}

func (d *driverFinto) asse(name string) string { return name[:1] }

func (d *driverFinto) Get(name string) (int, error) { return d.v[name], nil }

func (d *driverFinto) Range(name string) (lo, hi int) {
	a := d.asse(name)
	start, ext := a+"ertical Start Position", a+"ertical Extent"
	if a == "H" {
		start, ext = "Horizontal Start Position", "Horizontal Extent"
	}
	if name == start {
		return 0, d.campo(name) - 600
	}
	if name == ext {
		return 600, d.campo(name) - d.v[start]
	}
	return 0, 0
}

func (d *driverFinto) Set(name string, v int) error {
	lo, hi := d.Range(name)
	if v < lo || v > hi {
		return fmt.Errorf("valore %d fuori da %d-%d", v, lo, hi)
	}
	d.scritture = append(d.scritture, fmt.Sprintf("%s=%d", name, v))
	d.v[name] = v
	if strings.HasSuffix(name, "Start Position") {
		ext := strings.Replace(name, "Start Position", "Extent", 1)
		d.v[ext] = 600 + v%301 // ricalcolo imprevedibile, ma valido
		if max := d.campo(name) - v; d.v[ext] > max {
			d.v[ext] = max
		}
	}
	return nil
}

func TestApplyAreaOrdineEValori(t *testing.T) {
	// caso reale segnalato: da 90 mm, 100 mm, a 300 dpi (start 1063 px, extent 1181 px)
	d := nuovoDriverFinto()
	opts := Options{DPI: 300, StartMM: 90, HeightMM: 100}
	if err := applyArea(d, opts); err != nil {
		t.Fatal(err)
	}
	if d.v["Vertical Start Position"] != 1063 || d.v["Vertical Extent"] != 1181 {
		t.Errorf("valori finali: %v", d.v)
	}
	want := []string{"Vertical Start Position=0", "Vertical Start Position=1063", "Vertical Extent=1181"}
	if strings.Join(d.scritture, ",") != strings.Join(want, ",") {
		t.Errorf("ordine scritture: %v, atteso %v", d.scritture, want)
	}
}

func TestApplyAreaSenzaOffsetImpostaComunqueLoStart(t *testing.T) {
	d := nuovoDriverFinto()
	if err := applyArea(d, Options{DPI: 300, HeightMM: 100}); err != nil {
		t.Fatal(err)
	}
	if len(d.scritture) < 2 || d.scritture[0] != "Vertical Start Position=0" || d.scritture[1] != "Vertical Extent=1181" {
		t.Errorf("scritture: %v", d.scritture)
	}
}

func TestApplyAreaFinoAlFondo(t *testing.T) {
	d := nuovoDriverFinto()
	if err := applyArea(d, Options{DPI: 300, StartMM: 90}); err != nil {
		t.Fatal(err)
	}
	if d.v["Vertical Start Position"] != 1063 || d.v["Vertical Extent"] != 4200-1063 {
		t.Errorf("valori finali: %v", d.v)
	}
}

func TestApplyAreaOrizzontaleStartPrimaDellExtent(t *testing.T) {
	d := nuovoDriverFinto()
	if err := applyArea(d, Options{DPI: 300, StartMM: 90, HeightMM: 100, WidthMM: 108}); err != nil {
		t.Fatal(err)
	}
	if d.v["Horizontal Start Position"] != 637 || d.v["Horizontal Extent"] != 1276 {
		t.Errorf("valori finali: %v", d.v)
	}
	idx := map[string]int{}
	for i, s := range d.scritture {
		idx[strings.SplitN(s, "=", 2)[0]] = i
	}
	if idx["Vertical Start Position"] > idx["Vertical Extent"] || idx["Horizontal Start Position"] > idx["Horizontal Extent"] {
		t.Errorf("start deve precedere extent su ogni asse: %v", d.scritture)
	}
}

func TestApplyAreaOffsetFuoriIntervallo(t *testing.T) {
	d := nuovoDriverFinto()
	// 350 mm = 4134 px > massimo dello start (3600): errore senza scrivere l'extent
	err := applyArea(d, Options{DPI: 300, StartMM: 350})
	if err == nil {
		t.Fatal("errore atteso")
	}
	for _, s := range d.scritture {
		if strings.HasPrefix(s, "Vertical Extent") {
			t.Errorf("extent scritto nonostante l'errore: %v", d.scritture)
		}
	}
}
