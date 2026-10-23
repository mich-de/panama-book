//go:build windows

package ocr

import (
	"context"
	"errors"
	"image"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// skipSeNonNativo salta il test se il motore OCR di Windows non e' disponibile (nessuna lingua installata, API assente).
func skipSeNonNativo(t *testing.T) {
	t.Helper()
	if err := nativeProbe(); err != nil {
		t.Skipf("OCR nativo non disponibile: %v", err)
	}
}

var sintetico = []string{"LIBRETTO 1357", "BELLO 7531"}

func TestNativoLeggeImmagineSintetica(t *testing.T) {
	skipSeNonNativo(t)
	img := textImage(sintetico, 8)
	p := writePNG(t, img)
	rs, err := recognizeNative(context.Background(), []string{p})
	if err != nil {
		t.Fatal(err)
	}
	r := rs[0]
	if r.Width != img.Bounds().Dx() || r.Height != img.Bounds().Dy() {
		t.Errorf("dimensioni %dx%d, attese %v", r.Width, r.Height, img.Bounds().Size())
	}
	if r.Language == "" {
		t.Error("lingua vuota")
	}
	var all []string
	words := 0
	for _, l := range r.Lines {
		all = append(all, l.Text)
		for _, w := range l.Words {
			words++
			if w.W <= 0 || w.H <= 0 || w.X < 0 || w.Y < 0 || w.X+w.W > float64(r.Width)+1 || w.Y+w.H > float64(r.Height)+1 {
				t.Errorf("riquadro fuori dall'immagine: %+v", w)
			}
		}
	}
	text := squash(strings.Join(all, " "))
	t.Logf("letto %q (%d righe, %d parole, lingua %s)", text, len(r.Lines), words, r.Language)
	for _, want := range []string{"1357", "7531"} {
		if !strings.Contains(text, want) {
			t.Errorf("manca %q nel testo letto %q", want, text)
		}
	}
	if len(r.Lines) < 2 {
		t.Errorf("attese almeno 2 righe, lette %d", len(r.Lines))
	}
}

func TestNativoPiuImmaginiInOrdine(t *testing.T) {
	skipSeNonNativo(t)
	var paths []string
	cifre := []string{"1357", "7531", "3157", "5713", "1375"}
	for _, c := range cifre {
		paths = append(paths, writePNG(t, textImage([]string{c}, 6)))
	}
	rs, err := recognizeNative(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != len(cifre) {
		t.Fatalf("%d risultati per %d immagini", len(rs), len(cifre))
	}
	for i, r := range rs {
		var all []string
		for _, l := range r.Lines {
			all = append(all, l.Text)
		}
		if got := squash(strings.Join(all, "")); got != cifre[i] {
			t.Errorf("immagine %d: letto %q, atteso %q (ordine dei risultati?)", i, got, cifre[i])
		}
	}
}

func TestNativoImmagineBiancaDaTestoVuoto(t *testing.T) {
	skipSeNonNativo(t)
	p := writePNG(t, image.NewGray(image.Rect(0, 0, 200, 80)))
	rs, err := recognizeNative(context.Background(), []string{p})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs[0].Lines) != 0 {
		t.Errorf("immagine bianca: %d righe", len(rs[0].Lines))
	}
}

func TestNativoImmagineTroppoGrande(t *testing.T) {
	skipSeNonNativo(t)
	p := writePNG(t, image.NewGray(image.Rect(0, 0, 10001, 2))) // oltre MaxImageDimension (10000)
	_, err := recognizeNative(context.Background(), []string{p})
	var big errTooLarge
	if !errors.As(err, &big) || big.w != 10001 {
		t.Fatalf("atteso errTooLarge, ottenuto %v", err)
	}
	if !strings.Contains(err.Error(), "troppo grande") {
		t.Errorf("messaggio: %v", err)
	}
	// anche tramite l'API pubblica in modalita' automatica: nessun ripiego su PowerShell
	t.Setenv(envBackend, "")
	if _, err := RecognizeMany(context.Background(), []string{p}); !errors.As(err, &big) {
		t.Errorf("RecognizeMany: atteso errTooLarge, ottenuto %v", err)
	}
}

func TestNativoFileInesistente(t *testing.T) {
	skipSeNonNativo(t)
	if _, err := recognizeNative(context.Background(), []string{`C:\non\esiste\x.png`}); err == nil {
		t.Fatal("atteso errore")
	}
}

func TestNativoContestoGiaAnnullato(t *testing.T) {
	skipSeNonNativo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := writePNG(t, textImage(sintetico, 4))
	if _, err := recognizeNative(ctx, []string{p}); !errors.Is(err, context.Canceled) {
		t.Fatalf("atteso context.Canceled, ottenuto %v", err)
	}
}

// Annullamento a meta' lavoro: IAsyncInfo::Cancel, ritorno rapido e il backend resta utilizzabile.
func TestNativoAnnullaDuranteOCR(t *testing.T) {
	skipSeNonNativo(t)
	riga := "LIBRETTO 1357 BELLO 7531"
	big := textImage([]string{riga, riga, riga, riga}, 12)
	var paths []string
	for i := 0; i < 6; i++ {
		paths = append(paths, writePNG(t, big))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	t0 := time.Now()
	_, err := recognizeNative(ctx, paths)
	if err == nil {
		t.Skip("l'OCR e' finito prima della scadenza: macchina troppo veloce per provare l'annullamento")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("atteso DeadlineExceeded, ottenuto %v", err)
	}
	if d := time.Since(t0); d > 5*time.Second {
		t.Errorf("annullamento lento: %v", d)
	}
	// dopo l'annullamento il backend funziona ancora
	rs, err := recognizeNative(context.Background(), []string{writePNG(t, textImage([]string{"1357"}, 6))})
	if err != nil || len(rs[0].Lines) == 0 || !strings.Contains(squash(rs[0].Lines[0].Text), "1357") {
		t.Fatalf("dopo l'annullamento: %v %v", rs, err)
	}
}

func TestBackendAutomaticoUsaIlNativo(t *testing.T) {
	skipSeNonNativo(t)
	t.Setenv(envBackend, "")
	if b := Backend(); b != BackendNative {
		t.Errorf("Backend() = %q", b)
	}
	t.Setenv(envBackend, "powershell")
	if b := Backend(); b != BackendPowerShell {
		t.Errorf("Backend() con LIBRETTO_OCR=powershell = %q", b)
	}
}

// Prove che avviano PowerShell (cio' che il backend nativo evita): solo con OCR_TEST_PS=1.

func TestNativoEPowerShellDannoLoStessoRisultato(t *testing.T) {
	if os.Getenv("OCR_TEST_PS") != "1" {
		t.Skip("avvia PowerShell: attivare con OCR_TEST_PS=1")
	}
	skipSeNonNativo(t)
	p := writePNG(t, textImage(sintetico, 8))
	n, err := recognizeNative(context.Background(), []string{p})
	if err != nil {
		t.Fatal(err)
	}
	ps, err := recognizePowerShell(context.Background(), []string{p})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(n, ps) {
		t.Errorf("risultati diversi:\nnativo     %+v\npowershell %+v", n[0], ps[0])
	}
}

func TestRipiegoSuPowerShellSeIlNativoNonParte(t *testing.T) {
	if os.Getenv("OCR_TEST_PS") != "1" {
		t.Skip("avvia PowerShell: attivare con OCR_TEST_PS=1")
	}
	skipSeNonNativo(t)
	old := probeErr
	probeErr = errors.New("guasto simulato")
	defer func() { probeErr = old }()
	t.Setenv(envBackend, "")
	if b := Backend(); b != BackendPowerShell {
		t.Errorf("Backend() = %q con il nativo guasto", b)
	}
	rs, err := RecognizeMany(context.Background(), []string{writePNG(t, textImage(sintetico, 8))})
	if err != nil || len(rs) != 1 || len(rs[0].Lines) == 0 {
		t.Fatalf("ripiego: %v %v", rs, err)
	}
	// forzando il nativo non si ripiega
	t.Setenv(envBackend, "native")
	if _, err := RecognizeMany(context.Background(), []string{writePNG(t, textImage(sintetico, 8))}); err == nil {
		t.Error("LIBRETTO_OCR=native con il nativo guasto: atteso errore, senza ripiego")
	}
}

// Chiamate da piu' goroutine insieme: ogni chiamata ha thread e motori suoi, i risultati non si mescolano.
func TestNativoChiamateConcorrenti(t *testing.T) {
	skipSeNonNativo(t)
	cifre := []string{"1357", "7531", "3157", "5713", "1375", "7135"}
	paths := make([]string, len(cifre))
	for i, c := range cifre {
		paths[i] = writePNG(t, textImage([]string{c}, 6))
	}
	var wg sync.WaitGroup
	errs := make(chan string, 4*len(cifre))
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rs, err := recognizeNative(context.Background(), paths)
			if err != nil {
				errs <- err.Error()
				return
			}
			for i, r := range rs {
				var all []string
				for _, l := range r.Lines {
					all = append(all, l.Text)
				}
				if got := squash(strings.Join(all, "")); got != cifre[i] {
					errs <- "letto " + got + ", atteso " + cifre[i]
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}
