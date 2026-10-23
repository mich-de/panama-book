//go:build !go1.23

package main

// installCrashOutput: con un Go anteriore alla 1.23 non c'e' debug.SetCrashOutput (si usa solo stderr).
func installCrashOutput(base string) {}
