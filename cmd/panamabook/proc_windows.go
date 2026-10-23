package main

import (
	"os/exec"
	"syscall"
)

// createNoWindow: il sottoprocesso non apre una finestra di console propria.
const createNoWindow = 0x08000000

// hideWindow evita che libretto.exe o Edge facciano lampeggiare una finestra di console.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
