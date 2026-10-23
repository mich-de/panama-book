//go:build go1.23

package main

import (
	"os"
	"path/filepath"
	"runtime/debug"
)

// installCrashOutput manda gli errori fatali del runtime e i panic non recuperati in dati\panamabook-crash.log (oltre che su stderr).
func installCrashOutput(base string) {
	p := filepath.Join(base, "dati", "panamabook-crash.log")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		_ = f.Close()
	}
}
