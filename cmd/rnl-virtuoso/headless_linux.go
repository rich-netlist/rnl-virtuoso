package main

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureHeadlessProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
}
func stopHeadlessProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
func headlessSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
