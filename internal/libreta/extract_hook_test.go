package libreta

// Prove del punto di aggancio mrzCellsReader (extract.go) con un lettore finto: niente OCR, niente scansioni.

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"scanlibretto/internal/mrz"
)

// mrzCon costruisce un MRZ TD3 con dati inventati (cognome, nomi, nazionalita' e sesso a scelta) e check digit
// calcolati: tutti validi. Punteggio (scoreMRZ): 13.
func mrzCon(t *testing.T, surname, given, nat, sex string) *mrz.Data {
	t.Helper()
	l1 := "ISPAN" + surname + "<<" + given
	if len(l1) > 44 {
		t.Fatalf("riga 1 troppo lunga: %d", len(l1))
	}
	l1 += strings.Repeat("<", 44-len(l1))
	num, birth, exp := "PA1234567", "800101", "300101"
	l2 := num + string(mrz.CheckDigit(num)) + nat + birth + string(mrz.CheckDigit(birth)) + sex +
		exp + string(mrz.CheckDigit(exp)) + strings.Repeat("<", 14) + "<"
	l2 += string(mrz.CheckDigit(l2[0:10] + l2[13:20] + l2[21:43]))
	d, err := mrz.Parse(l1, l2)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Checks.AllOK() {
		t.Fatalf("MRZ inventato senza tutti i check digit: %+v", d.Checks)
	}
	return &d
}

// mrzInventato costruisce un MRZ TD3 con dati inventati e check digit calcolati. Con rompi=true la cifra di
// controllo del numero documento e' sbagliata: restano validi 3 check su 5 (nascita, scadenza, opzionale).
// Punteggio (scoreMRZ): 13 con tutti i controlli, 9 con rompi=true.
func mrzInventato(t *testing.T, rompi bool) *mrz.Data {
	t.Helper()
	d := mrzCon(t, "ROSSI", "MARIO", "ITA", "M")
	if !rompi {
		return d
	}
	b := []byte(d.Line2)
	b[9] = '0' + (b[9]-'0'+1)%10
	broken, err := mrz.Parse(d.Line1, string(b))
	if err != nil {
		t.Fatal(err)
	}
	if broken.Checks.AllOK() {
		t.Fatal("MRZ inventato: AllOK con rompi=true")
	}
	return &broken
}

// bandaFissa e' una lettura a banda finta che restituisce sempre lo stesso risultato.
func bandaFissa(d *mrz.Data, notes []string, score int, err error) mrzBandFunc {
	return func(context.Context, image.Image, string) (*mrz.Data, []string, int, error) {
		return d, notes, score, err
	}
}

// lettoreFinto conta le chiamate del lettore a celle e ricorda le cartelle di debug ricevute.
type lettoreFinto struct {
	calls int
	dirs  []string
}

// installa registra un lettore a celle finto in mrzCellsReader e lo ripristina a fine prova (il lettore vero, se
// c'e', si e' registrato da solo con init).
func installa(t *testing.T, f func(img image.Image, debugDir string) (*mrz.Data, []string, int)) *lettoreFinto {
	t.Helper()
	l := &lettoreFinto{}
	prima := mrzCellsReader
	mrzCellsReader = func(img image.Image, debugDir string) (*mrz.Data, []string, int) {
		l.calls++
		l.dirs = append(l.dirs, debugDir)
		return f(img, debugDir)
	}
	t.Cleanup(func() { mrzCellsReader = prima })
	return l
}

// senzaLettore toglie il lettore a celle per la durata della prova.
func senzaLettore(t *testing.T) {
	t.Helper()
	prima := mrzCellsReader
	mrzCellsReader = nil
	t.Cleanup(func() { mrzCellsReader = prima })
}

var immagineProva = image.NewRGBA(image.Rect(0, 0, 400, 120))

func TestAggancioSenzaLettoreNonCambiaNulla(t *testing.T) {
	senzaLettore(t)
	parziale := mrzInventato(t, true)
	for _, c := range []struct {
		d     *mrz.Data
		score int
	}{{nil, 0}, {parziale, scoreMRZ(*parziale)}} {
		r, err := readMRZOriented(context.Background(), immagineProva, "", bandaFissa(c.d, []string{"n"}, c.score, nil))
		if err != nil || r.Data != c.d || r.Score != c.score || len(r.Notes) != 1 || len(r.Warnings) != 0 {
			t.Errorf("senza lettore: %+v %v", r, err)
		}
	}
}

func TestAggancioUsatoSeLaBandaNonTrovaNulla(t *testing.T) {
	buono := mrzInventato(t, false)
	l := installa(t, func(image.Image, string) (*mrz.Data, []string, int) {
		return buono, []string{"celle"}, scoreMRZ(*buono)
	})
	r, err := readMRZOriented(context.Background(), immagineProva, "", bandaFissa(nil, nil, 0, nil))
	if err != nil || r.Data != buono || len(r.Notes) != 1 || r.Notes[0] != "celle" || r.Score != scoreMRZ(*buono) {
		t.Errorf("risultato: %+v %v", r, err)
	}
	if l.calls != 1 {
		t.Errorf("chiamate al lettore: %d, attesa 1", l.calls)
	}
}

// Con tutti i check digit dell'OCR il lettore a celle si chiama lo stesso (seconda lettura della riga 1): se le due
// letture coincidono non cambia niente.
func TestAggancioChiamatoAncheSeLaBandaHaTuttiIControlli(t *testing.T) {
	buono := mrzInventato(t, false)
	l := installa(t, func(image.Image, string) (*mrz.Data, []string, int) { return mrzInventato(t, false), nil, 13 })
	r, err := readMRZOriented(context.Background(), immagineProva, "", bandaFissa(buono, []string{"ocr"}, scoreMRZ(*buono)+1, nil))
	if err != nil || r.Data != buono || r.Score != scoreMRZ(*buono)+1 || len(r.Notes) != 1 || len(r.Warnings) != 0 {
		t.Errorf("letture uguali: %+v %v", r, err)
	}
	if l.calls != 1 {
		t.Errorf("chiamate al lettore: %d, attesa 1", l.calls)
	}
	// un lettore senza lettura non cambia niente
	installa(t, func(image.Image, string) (*mrz.Data, []string, int) { return nil, nil, 0 })
	if r, err := readMRZOriented(context.Background(), immagineProva, "", bandaFissa(buono, nil, 13, nil)); err != nil || r.Data != buono {
		t.Errorf("lettore senza lettura: %+v %v", r, err)
	}
}

// Con una banda parziale (9) si tiene il risultato con il punteggio piu' alto; a parita' resta la banda.
func TestAggancioTieneIlMigliore(t *testing.T) {
	buono, parziale := mrzInventato(t, false), mrzInventato(t, true)
	peggiore := *parziale
	peggiore.Checks.Birth = false // 2 check su 5: punteggio 7
	sp := scoreMRZ(*parziale)
	if sb, sw := scoreMRZ(*buono), scoreMRZ(peggiore); !(sb > sp && sp > sw) {
		t.Fatalf("punteggi inattesi: %d %d %d", sb, sp, sw)
	}
	casi := []struct {
		nome       string
		celle      *mrz.Data
		celleScore int
		vinceCelle bool
	}{
		{"celle migliori", buono, scoreMRZ(*buono), true},
		{"banda migliore", &peggiore, scoreMRZ(peggiore), false},
		{"parita': resta la banda", parziale, sp, false},
		{"celle senza dati", nil, 0, false},
		{"punteggio alto ma senza dati", nil, 99, false},
	}
	for _, c := range casi {
		installa(t, func(image.Image, string) (*mrz.Data, []string, int) { return c.celle, []string{"celle"}, c.celleScore })
		r, err := readMRZOriented(context.Background(), immagineProva, "", bandaFissa(parziale, []string{"banda"}, sp, nil))
		if err != nil || r.Data == nil {
			t.Fatalf("%s: %+v %v", c.nome, r, err)
		}
		if got := r.Notes[0] == "celle"; got != c.vinceCelle {
			t.Errorf("%s: vincono le celle=%v, atteso %v (punteggio %d)", c.nome, got, c.vinceCelle, r.Score)
		}
	}
}

func TestAggancioErroreDellaBandaSiPropaga(t *testing.T) {
	l := installa(t, func(image.Image, string) (*mrz.Data, []string, int) { t.Error("lettore chiamato"); return nil, nil, 0 })
	guasto := errors.New("OCR non disponibile")
	if _, err := readMRZOriented(context.Background(), immagineProva, "", bandaFissa(nil, nil, 0, guasto)); !errors.Is(err, guasto) {
		t.Errorf("errore: %v", err)
	}
	if l.calls != 0 {
		t.Errorf("chiamate al lettore: %d", l.calls)
	}
}

func TestAggancioContestoAnnullatoNonChiamaIlLettore(t *testing.T) {
	l := installa(t, func(image.Image, string) (*mrz.Data, []string, int) { return nil, nil, 0 })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readMRZOriented(ctx, immagineProva, "", bandaFissa(nil, nil, 0, nil)); !errors.Is(err, context.Canceled) {
		t.Errorf("errore: %v", err)
	}
	if l.calls != 0 {
		t.Errorf("chiamate al lettore: %d", l.calls)
	}
}

// Un guasto del lettore a celle non deve fermare l'estrazione: resta la lettura a banda.
func TestAggancioPanicDelLettoreNonFermaIlProgramma(t *testing.T) {
	parziale, buono := mrzInventato(t, true), mrzInventato(t, false)
	installa(t, func(image.Image, string) (*mrz.Data, []string, int) { panic("indice fuori dai limiti") })
	for _, banda := range []*mrz.Data{parziale, buono} { // sia con la banda parziale sia con tutti i check digit
		r, err := readMRZOriented(context.Background(), immagineProva, "", bandaFissa(banda, []string{"banda"}, scoreMRZ(*banda), nil))
		if err != nil || r.Data != banda || len(r.Notes) != 1 || r.Notes[0] != "banda" || r.Score != scoreMRZ(*banda) || len(r.Warnings) != 0 {
			t.Errorf("dopo il panic: %+v %v", r, err)
		}
	}
}

// Il lettore a celle scrive i suoi file di debug in una sottocartella "celle", senza toccare quelli della banda.
func TestAggancioCartellaDiDebugSeparata(t *testing.T) {
	l := installa(t, func(image.Image, string) (*mrz.Data, []string, int) { return nil, nil, 0 })
	for _, dbg := range []string{"", filepath.Join("dbg", "rot180")} {
		if _, err := readMRZOriented(context.Background(), immagineProva, dbg, bandaFissa(nil, nil, 0, nil)); err != nil {
			t.Fatal(err)
		}
	}
	if len(l.dirs) != 2 || l.dirs[0] != "" || l.dirs[1] != filepath.Join("dbg", "rot180", "celle") {
		t.Errorf("cartelle di debug passate al lettore: %q", l.dirs)
	}
}

// immagineConSegno e' un'immagine 400x120 bianca con un solo pixel rosso in (5,5): ruotandola il pixel si sposta, e
// il lettore finto riconosce da li' l'orientamento in cui gli viene data l'immagine.
func immagineConSegno() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 400, 120))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	img.Set(5, 5, color.RGBA{R: 255, A: 255})
	return img
}

// orientamentoDi dice di quanti gradi (0, 90, 180, 270) e' stata ruotata l'immagine di immagineConSegno; -1 se non lo sa.
func orientamentoDi(img image.Image) int {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	rosso := func(x, y int) bool {
		r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
		return r > 0xf000 && g < 0x1000 && bl < 0x1000
	}
	switch {
	case w == 400 && h == 120 && rosso(5, 5):
		return 0
	case w == 400 && h == 120 && rosso(w-1-5, h-1-5):
		return 180
	case w == 120 && h == 400 && rosso(w-1-5, 5):
		return 90
	case w == 120 && h == 400 && rosso(5, h-1-5):
		return 270
	}
	return -1
}

// Il lettore a celle trova l'MRZ solo a 180 gradi: findOrientation deve riportare quella rotazione e fermarsi li'.
func TestFindOrientationUsaIlLettoreACelle(t *testing.T) {
	buono := mrzInventato(t, false)
	var viste []int
	installa(t, func(img image.Image, _ string) (*mrz.Data, []string, int) {
		rot := orientamentoDi(img)
		viste = append(viste, rot)
		if rot == 180 {
			return buono, []string{"celle"}, scoreMRZ(*buono)
		}
		return nil, nil, 0
	})
	best, img, err := findOrientation(context.Background(), immagineConSegno(), "", bandaFissa(nil, nil, 0, nil))
	if err != nil || best == nil {
		t.Fatalf("risultato: %v %v", best, err)
	}
	if best.Rotation != 180 || best.MRZ != buono || len(best.MRZCorrections) != 1 || best.Fields == nil {
		t.Errorf("risultato: %+v", best)
	}
	if orientamentoDi(img) != 180 {
		t.Errorf("l'immagine restituita non e' quella ruotata di 180 gradi")
	}
	if got := fmt.Sprint(viste); got != "[0 180]" {
		t.Errorf("orientamenti provati: %s, attesi [0 180] (si ferma al primo con tutti i controlli)", got)
	}
}

// Una lettura parziale a 0 gradi non ferma la ricerca: vince quella completa trovata dopo (a 270).
func TestFindOrientationPreferisceLaLetturaCompleta(t *testing.T) {
	buono, parziale := mrzInventato(t, false), mrzInventato(t, true)
	var viste []int
	installa(t, func(img image.Image, _ string) (*mrz.Data, []string, int) {
		rot := orientamentoDi(img)
		viste = append(viste, rot)
		switch rot {
		case 0:
			return parziale, nil, scoreMRZ(*parziale)
		case 270:
			return buono, nil, scoreMRZ(*buono)
		}
		return nil, nil, 0
	})
	best, _, err := findOrientation(context.Background(), immagineConSegno(), "", bandaFissa(nil, nil, 0, nil))
	if err != nil || best == nil || best.Rotation != 270 || best.MRZ != buono {
		t.Fatalf("risultato: %+v %v", best, err)
	}
	if got := fmt.Sprint(viste); got != "[0 180 90 270]" {
		t.Errorf("orientamenti provati: %s", got)
	}
}

func TestFindOrientationSenzaMRZRestituisceNil(t *testing.T) {
	senzaLettore(t)
	best, img, err := findOrientation(context.Background(), immagineConSegno(), "", bandaFissa(nil, nil, 0, nil))
	if err != nil || best != nil || img != nil {
		t.Errorf("senza MRZ: %v %v %v", best, img, err)
	}
	guasto := errors.New("OCR non disponibile")
	if _, _, err := findOrientation(context.Background(), immagineConSegno(), "", bandaFissa(nil, nil, 0, guasto)); !errors.Is(err, guasto) {
		t.Errorf("errore della banda: %v", err)
	}
}

// Gli avvisi della seconda lettura arrivano fino al risultato di Extract (Result.Warnings).
func TestFindOrientationPortaGliAvvisi(t *testing.T) {
	ocr := mrzCon(t, "ROSSX", "MARIO", "ITA", "M")
	celle := mrzCon(t, "ROSSI", "MARIO", "ITA", "M")
	installa(t, func(image.Image, string) (*mrz.Data, []string, int) {
		return celle, []string{"riga1 pos 10: lettura incerta (I)"}, 13 // la cella diversa (la decima) e' incerta
	})
	best, _, err := findOrientation(context.Background(), immagineConSegno(), "", bandaFissa(ocr, nil, 14, nil))
	if err != nil || best == nil || best.MRZ != ocr {
		t.Fatalf("risultato: %+v %v", best, err)
	}
	if len(best.Warnings) != 1 || best.Warnings[0] != warnMRZLine1 {
		t.Errorf("avvisi: %q", best.Warnings)
	}
}
