//go:build unix

package searchdata

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryWriterLock(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) // #nosec G115 -- os.File provides a valid descriptor.
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return err == nil, err
}

func releaseWriterLock(file *os.File) {
	_ = unix.Flock(int(file.Fd()), unix.LOCK_UN) // #nosec G115 -- os.File provides a valid descriptor.
}
