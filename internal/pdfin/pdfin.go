// Package pdfin trasforma la prima pagina di un PDF in un'immagine PNG con il motore PDF integrato in Windows (Windows.Data.Pdf):
// nessuna libreria esterna. Serve per leggere i certificati che arrivano come PDF con lo stesso percorso (OCR) delle scansioni.
package pdfin

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

//go:embed pdfrender.ps1
var script string

const (
	// Width e' la larghezza in pixel della pagina resa: circa 300 dpi su un A4, abbastanza per l'OCR dei testi piccoli.
	Width   = 2480
	timeout = 60 * time.Second
)

// RenderFirstPage scrive in png la prima pagina del PDF, larga width pixel (0 = Width).
func RenderFirstPage(ctx context.Context, pdf, png string, width int) error {
	if width <= 0 {
		width = Width
	}
	pdf, err := filepath.Abs(pdf)
	if err != nil {
		return err
	}
	if png, err = filepath.Abs(png); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "pdfin")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	ps1 := filepath.Join(dir, "pdfrender.ps1")
	// BOM UTF-8: Windows PowerShell 5.1 legge cosi' correttamente gli accenti
	if err := os.WriteFile(ps1, append([]byte{0xEF, 0xBB, 0xBF}, script...), 0o600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", ps1,
		"-Pdf", pdf, "-Png", png, "-Width", strconv.Itoa(width))
	hideWindow(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("PDF: %w: %s", err, bytes.TrimSpace(errb.Bytes()))
	}
	return nil
}
