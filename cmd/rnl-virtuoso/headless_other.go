//go:build !linux

package main

import (
	"os"
	"os/exec"
)

func configureHeadlessProcess(cmd *exec.Cmd) {}
func stopHeadlessProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
func headlessSignals() []os.Signal { return []os.Signal{os.Interrupt} }
