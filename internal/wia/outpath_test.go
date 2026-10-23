package wia

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"scanlibretto/internal/imgproc"
)

// Test su cartelle temporanee e immagini sintetiche: nessun contatto con lo scanner.

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func TestCheckOutPath(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "scan.png")

	if err := checkOutPath(out, false); err != nil {
		t.Errorf("file inesistente deve passare: %v", err)
	}
	if err := os.WriteFile(out, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := checkOutPath(out, false)
	if err == nil {
		t.Fatal("file esistente senza force deve essere rifiutato")
	}
	if !strings.Contains(err.Error(), "scan.png") || !strings.Contains(err.Error(), "-force") {
		t.Errorf("il messaggio deve citare file e -force: %v", err)
	}
	if err := checkOutPath(out, true); err != nil {
		t.Errorf("con force deve passare: %v", err)
	}
	if checkOutPath(dir, true) == nil {
		t.Error("una cartella come destinazione deve essere rifiutata anche con force")
	}
	if checkOutPath(filepath.Join(dir, "manca", "a.png"), true) == nil {
		t.Error("cartella inesistente deve essere rifiutata")
	}
	if checkOutPath("", true) == nil {
		t.Error("percorso vuoto accettato")
	}
	if b, _ := os.ReadFile(out); string(b) != "x" {
		t.Error("checkOutPath non deve modificare il file")
	}
}

func TestNewTempName(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "scan.png")
	a, err := newTempName(out, "bmp")
	if err != nil {
		t.Fatal(err)
	}
	b, err := newTempName(out, "bmp")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("due nomi temporanei uguali")
	}
	if filepath.Dir(a) != dir || !strings.HasSuffix(a, ".wia.bmp") || a == out+".wia.bmp" {
		t.Errorf("nome temporaneo inatteso: %s", a)
	}
	if _, err := os.Stat(a); !os.IsNotExist(err) {
		t.Error("il nome riservato non deve lasciare un file (SaveFile non sovrascrive)")
	}
	if _, err := newTempName(filepath.Join(dir, "manca", "a.png"), "bmp"); err == nil {
		t.Error("cartella inesistente: errore atteso")
	}
}

func TestNewTempNameParallel(t *testing.T) {
	out := filepath.Join(t.TempDir(), "scan.png")
	const n = 50
	var mu sync.Mutex
	seen := map[string]bool{}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name, err := newTempName(out, "png")
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if seen[name] {
				t.Errorf("collisione: %s", name)
			}
			seen[name] = true
		}()
	}
	wg.Wait()
}

func TestTempFilesCleanup(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "scan.png")
	var tf tempFiles
	p1, err := tf.new(out, "bmp")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := tf.new(out, "png")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p1, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	// p2 mai creato (o gia' spostato): cleanup non deve fallire
	_ = p2
	tf.cleanup()
	if l := listDir(t, dir); len(l) != 0 {
		t.Errorf("restano file dopo cleanup: %v", l)
	}
}

func synthImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			img.Set(x, y, color.RGBA{uint8(40 * x), uint8(90 * y), 200, 255})
		}
	}
	return img
}

func TestFinalizeImageBMP(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "scan.png")
	var tf tempFiles
	tmp, err := tf.new(out, "bmp")
	if err != nil {
		t.Fatal(err)
	}
	bmp := bmp24(3, 2, func(x, y int) color.RGBA { return color.RGBA{uint8(40 * x), uint8(90 * y), 200, 255} })
	if err := os.WriteFile(tmp, bmp, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := finalizeImage(&tf, tmp, "bmp", out, false); err != nil {
		t.Fatal(err)
	}
	tf.cleanup()
	if l := listDir(t, dir); len(l) != 1 || l[0] != "scan.png" {
		t.Errorf("deve restare solo la destinazione: %v", l)
	}
}

func TestFinalizeImageRifiutaSovrascrittura(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "scan.png")
	if err := os.WriteFile(out, []byte("ORIGINALE"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{"png", "bmp"} {
		var tf tempFiles
		tmp, err := tf.new(out, ext)
		if err != nil {
			t.Fatal(err)
		}
		if ext == "png" {
			if err := imgproc.SavePNG(synthImage(), tmp); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(tmp, bmp24(3, 2, func(x, y int) color.RGBA { return color.RGBA{A: 255} }), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := finalizeImage(&tf, tmp, ext, out, false); err == nil || !strings.Contains(err.Error(), "-force") {
			t.Errorf("%s: atteso rifiuto con -force, ottenuto %v", ext, err)
		}
		tf.cleanup()
		if b, _ := os.ReadFile(out); string(b) != "ORIGINALE" {
			t.Fatalf("%s: il file esistente e' stato modificato", ext)
		}
		if l := listDir(t, dir); len(l) != 1 {
			t.Errorf("%s: temporanei rimasti: %v", ext, l)
		}
	}
}

func TestFinalizeImageForceSostituisce(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "scan.png")
	if err := os.WriteFile(out, []byte("ORIGINALE"), 0o644); err != nil {
		t.Fatal(err)
	}
	var tf tempFiles
	tmp, err := tf.new(out, "png")
	if err != nil {
		t.Fatal(err)
	}
	if err := imgproc.SavePNG(synthImage(), tmp); err != nil {
		t.Fatal(err)
	}
	if err := finalizeImage(&tf, tmp, "png", out, true); err != nil {
		t.Fatal(err)
	}
	tf.cleanup()
	if b, _ := os.ReadFile(out); string(b) == "ORIGINALE" {
		t.Error("con force il file doveva essere sostituito")
	}
	if l := listDir(t, dir); len(l) != 1 {
		t.Errorf("temporanei rimasti: %v", l)
	}
}

func TestFinalizeImageFormatoIgnoto(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "scan.png")
	var tf tempFiles
	tmp, err := tf.new(out, "jpg")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := finalizeImage(&tf, tmp, "jpg", out, true); err == nil {
		t.Error("formato non supportato accettato")
	}
	tf.cleanup()
	if l := listDir(t, dir); len(l) != 0 {
		t.Errorf("temporanei rimasti: %v", l)
	}
}
