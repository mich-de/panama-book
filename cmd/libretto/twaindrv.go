package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"scanlibretto/internal/wia"
)

// defaultDriver e' il driver usato senza -driver. Si passa a "twain" dopo la prova con un libretto vero.
const defaultDriver = "wia"

// Programmi che parlano con il data source TWAIN. Oggi e' installato solo il driver a 32 bit; la versione a 64 bit
// si usa solo se esiste il driver "PaperStream IP (TWAIN x64)" (cartella C:\Windows\twain_64) ed e' stata compilata.
const (
	helperTwain32 = "twainscan32.exe"
	helperTwain64 = "twainscan64.exe"
)

var (
	errTwainNoPaper = errors.New("carta assente (TWAIN)")
	errTwainJam     = errors.New("inceppamento (TWAIN)")
)

// acquire scansiona con il driver scelto e scrive il PNG grezzo in c.opts.OutPath.
func acquire(c scanConfig) error {
	switch c.driver {
	case "wia", "":
		return wia.Scan(c.opts)
	case "twain":
		return scanTwain(c.opts)
	}
	return fmt.Errorf("driver %q sconosciuto: usa wia o twain", c.driver)
}

// findTwainHelper cerca il programma TWAIN accanto all'eseguibile, poi nella cartella corrente e in _out:
// quello a 64 bit se c'e' il driver a 64 bit, altrimenti quello a 32 bit.
func findTwainHelper() (string, error) {
	names := []string{helperTwain32}
	if st, err := os.Stat(`C:\Windows\twain_64`); err == nil && st.IsDir() {
		names = []string{helperTwain64, helperTwain32}
	}
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	dirs = append(dirs, ".", "_out")
	for _, name := range names {
		for _, d := range dirs {
			p := filepath.Join(d, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return filepath.Abs(p)
			}
		}
	}
	return "", fmt.Errorf("manca %s (programma a 32 bit per TWAIN): compilalo con GOARCH=386 go build -o %s ./cmd/twainscan", helperTwain32, helperTwain32)
}

// scanTwain lancia il programma a 32 bit. Il suo testo va su stderr: lo stdout di libretto e' il JSON.
func scanTwain(o wia.Options) error {
	exe, err := findTwainHelper()
	if err != nil {
		return err
	}
	args := []string{"-yes", "-ignora-sensore", "-o", o.OutPath, "-dpi", strconv.Itoa(o.DPI),
		"-from", strconv.FormatFloat(o.StartMM, 'f', -1, 64),
		"-len", strconv.FormatFloat(o.HeightMM, 'f', -1, 64),
		"-width", strconv.FormatFloat(o.WidthMM, 'f', -1, 64)}
	if o.Gray {
		args = append(args, "-gray")
	}
	if o.Force {
		args = append(args, "-force")
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		switch ee.ExitCode() {
		case exitNoPaper:
			return errTwainNoPaper
		case exitJam:
			return errTwainJam
		}
		return fmt.Errorf("scansione TWAIN fallita (uscita %d)", ee.ExitCode())
	}
	return err
}
