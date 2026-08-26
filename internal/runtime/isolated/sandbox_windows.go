//go:build windows

package isolated

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureWorkerCommand(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
	return nil
}

func applyWorkerPlatformSandbox() ([]string, error) {
	return []string{"windows:dedicated-process-group", "parent:ipc-eof"}, nil
}
