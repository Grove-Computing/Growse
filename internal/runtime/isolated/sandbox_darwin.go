//go:build darwin

package isolated

import (
	"os/exec"
	"syscall"
)

func configureWorkerCommand(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

func applyWorkerPlatformSandbox() ([]string, error) {
	return []string{"darwin:dedicated-process-group", "parent:ipc-eof"}, nil
}
