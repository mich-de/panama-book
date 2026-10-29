package main

import (
	"os/exec"
	"syscall"
	"unsafe"
)

// createNoWindow: il sottoprocesso non apre una finestra di console propria.
const createNoWindow = 0x08000000

// hideWindow evita che libretto.exe o Edge facciano lampeggiare una finestra di console.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

var procMessageBox = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")

// showFatal mostra un errore che impedisce di partire: il programma e' costruito senza console (-H windowsgui), quindi
// un messaggio su stderr non lo vedrebbe nessuno. Lo stesso testo resta in dati\panamabook.log.
func showFatal(msg string) {
	t, _ := syscall.UTF16PtrFromString("Panama Book")
	m, _ := syscall.UTF16PtrFromString(msg)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10) // MB_ICONERROR
}
