//go:build !windows

package ocr

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
