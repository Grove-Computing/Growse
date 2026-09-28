//go:build unix

package searchprovider

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func tryWriterLock(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) // #nosec G115 -- os.File provides a valid OS descriptor representable by int.
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return err == nil, err
}
func releaseWriterLock(file *os.File) { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN) } // #nosec G115 -- valid OS descriptor from os.File.
