//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func hideWindow(cmd *exec.Cmd) {}

func showFatal(msg string) { fmt.Fprintln(os.Stderr, msg) }
