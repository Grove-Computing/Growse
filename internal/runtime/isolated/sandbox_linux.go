//go:build linux

package isolated

import (
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func configureWorkerCommand(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	return nil
}

func applyWorkerPlatformSandbox() ([]string, error) {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return nil, fmt.Errorf("apply Linux no-new-privileges: %w", err)
	}
	value, err := unix.PrctlRetInt(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0)
	if err != nil || value != 1 {
		return nil, fmt.Errorf("verify Linux no-new-privileges: value=%d error=%w", value, err)
	}
	return []string{"linux:no-new-privileges", "linux:parent-death-signal", "parent:ipc-eof"}, nil
}
