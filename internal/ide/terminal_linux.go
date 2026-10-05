package ide

import (
	"github.com/aymanbagabas/go-pty"
	"syscall"
)

func killTerminalProcess(cmd *pty.Cmd) error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
