package camera

import (
	"os/exec"
	"syscall"
)

func init() {
	configureProcess = func(command *exec.Cmd) {
		command.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	}
}
