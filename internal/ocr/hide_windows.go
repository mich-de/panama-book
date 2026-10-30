//go:build windows

package ocr

import (
	"os/exec"
	"syscall"
)

// hideWindow evita che il sottoprocesso apra una finestra di console (CREATE_NO_WINDOW).
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
