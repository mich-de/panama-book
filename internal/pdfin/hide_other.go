//go:build !windows

package pdfin

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
