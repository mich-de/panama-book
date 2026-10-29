//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func hideWindow(cmd *exec.Cmd) {}

func showFatal(msg string) { fmt.Fprintln(os.Stderr, msg) }

func pickFolder(title, start string) (string, error) {
	return "", fmt.Errorf("finestra di scelta non disponibile su questo sistema")
}
