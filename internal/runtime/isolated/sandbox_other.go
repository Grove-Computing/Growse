//go:build !linux && !darwin && !windows

package isolated

import (
	"errors"
	"os/exec"
)

func configureWorkerCommand(*exec.Cmd) error {
	return errors.New("runtime sandbox is unsupported on this platform")
}

func applyWorkerPlatformSandbox() ([]string, error) {
	return nil, errors.New("runtime sandbox is unsupported on this platform")
}
