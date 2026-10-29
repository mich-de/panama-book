package ocr

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed winocr.ps1
var script string

// recognizePowerShell e' il vecchio backend: lancia powershell.exe con winocr.ps1 (Windows.Media.Ocr via reflection WinRT).
// Costa circa 1 s di avvio a chiamata e su alcuni PC aziendali un controllo di sicurezza puo' terminare il processo
// (codice 0xC0400065): per questo il predefinito e' il backend nativo (ocr_native_windows.go); resta come ripiego.
func recognizePowerShell(ctx context.Context, imagePaths []string) ([]*Result, error) {
	dir, err := os.MkdirTemp("", "winocr")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	ps1 := filepath.Join(dir, "winocr.ps1")
	// BOM UTF-8: Windows PowerShell 5.1 legge cosi' correttamente gli accenti
	if err := os.WriteFile(ps1, append([]byte{0xEF, 0xBB, 0xBF}, script...), 0o600); err != nil {
		return nil, err
	}
	args := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", ps1}
	for _, p := range imagePaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		args = append(args, abs)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("OCR: %w: %s", err, bytes.TrimSpace(errb.Bytes()))
	}
	var res []*Result
	if err := json.Unmarshal(bytes.TrimPrefix(bytes.TrimSpace(out.Bytes()), []byte{0xEF, 0xBB, 0xBF}), &res); err != nil {
		return nil, fmt.Errorf("OCR: risposta non valida: %w", err)
	}
	if len(res) != len(imagePaths) {
		return nil, fmt.Errorf("OCR: attese %d risposte, ricevute %d", len(imagePaths), len(res))
	}
	return res, nil
}
