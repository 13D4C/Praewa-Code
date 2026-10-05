package ide

import (
	"github.com/aymanbagabas/go-pty"
	"os/exec"
	"strconv"
)

func killTerminalProcess(cmd *pty.Cmd) error {
	return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}
