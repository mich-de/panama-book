// Package ocr esegue il riconoscimento del testo con il motore OCR integrato in Windows (Windows.Media.Ocr).
//
// Due backend, stesso motore e stessi risultati:
//   - nativo (predefinito): chiama le API WinRT direttamente dal processo, senza PowerShell e senza cgo
//     (ocr_native_windows.go);
//   - powershell: il vecchio percorso, powershell.exe con winocr.ps1 (ocr_powershell.go), usato come ripiego se il
//     nativo non parte.
//
// La variabile d'ambiente LIBRETTO_OCR=native|powershell forza uno dei due (senza ripiego); vuota o diversa: automatico.
package ocr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// timeout e' il tetto di una chiamata a RecognizeMany (come prima, qualunque sia il backend).
const timeout = 90 * time.Second

// Word e' una parola riconosciuta con il suo riquadro in pixel.
type Word struct {
	Text string  `json:"text"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

// Line e' una riga di testo.
type Line struct {
	Text  string `json:"text"`
	Words []Word `json:"words"`
}

// Result e' l'esito dell'OCR su una immagine.
type Result struct {
	Language string `json:"language"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Lines    []Line `json:"lines"`
}

// Recognize esegue l'OCR su una immagine (PNG/JPEG/BMP).
func Recognize(ctx context.Context, imagePath string) (*Result, error) {
	rs, err := RecognizeMany(ctx, []string{imagePath})
	if err != nil {
		return nil, err
	}
	return rs[0], nil
}

// Nomi dei backend (valori di LIBRETTO_OCR e di Backend).
const (
	BackendNative     = "native"
	BackendPowerShell = "powershell"
	envBackend        = "LIBRETTO_OCR"
)

// errTooLarge: immagine oltre MaxImageDimension del motore; non si ripiega (anche PowerShell darebbe lo stesso errore).
type errTooLarge struct{ w, h int }

func (e errTooLarge) Error() string {
	return fmt.Sprintf("OCR: immagine troppo grande per l'OCR (%dx%d)", e.w, e.h)
}

// requested legge LIBRETTO_OCR: "native", "powershell" o "" (automatico).
func requested() string {
	switch v := strings.ToLower(strings.TrimSpace(os.Getenv(envBackend))); v {
	case BackendNative, BackendPowerShell:
		return v
	}
	return ""
}

var warnOnce sync.Once

// warnFallback scrive su stderr, una sola volta per processo, perche' si e' ripiegato su PowerShell.
func warnFallback(why error) {
	warnOnce.Do(func() {
		fmt.Fprintf(os.Stderr, "libretto: OCR nativo non disponibile (%v): uso PowerShell\n", why)
	})
}

// Backend dice quale backend userebbe ora RecognizeMany: "native" o "powershell" (anche quando e' un ripiego).
func Backend() string {
	switch requested() {
	case BackendPowerShell:
		return BackendPowerShell
	case BackendNative:
		return BackendNative
	}
	if err := nativeProbe(); err != nil {
		return BackendPowerShell
	}
	return BackendNative
}

// RecognizeMany esegue l'OCR su piu' immagini. Con il backend nativo non c'e' costo di avvio; con PowerShell c'e'
// circa 1 s fisso a chiamata, quindi conviene passare tutte le immagini insieme.
func RecognizeMany(ctx context.Context, imagePaths []string) ([]*Result, error) {
	if len(imagePaths) == 0 {
		return nil, nil
	}
	switch requested() {
	case BackendPowerShell:
		return recognizePowerShell(ctx, imagePaths)
	case BackendNative:
		return recognizeNative(ctx, imagePaths)
	}
	// automatico: nativo, con ripiego su PowerShell se non parte o se la chiamata nativa fallisce (non per ctx annullato
	// e non per immagine troppo grande, dove PowerShell darebbe lo stesso errore)
	if err := nativeProbe(); err != nil {
		warnFallback(err)
		return recognizePowerShell(ctx, imagePaths)
	}
	res, err := recognizeNative(ctx, imagePaths)
	if err == nil {
		return res, nil
	}
	var big errTooLarge
	if ctx.Err() != nil || errors.As(err, &big) {
		return nil, err
	}
	warnFallback(err)
	return recognizePowerShell(ctx, imagePaths)
}
