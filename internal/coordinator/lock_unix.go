//go:build unix

package coordinator

import (
	"errors"
	"os"
	"syscall"
)

var errUnsupported = errors.New("file locking is not supported on this file system")

func tryLock(file *os.File, mode Mode) (bool, error) {
	operation := syscall.LOCK_EX
	if mode == Shared {
		operation = syscall.LOCK_SH
	}
	err := syscall.Flock(int(file.Fd()), operation|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	if errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EOPNOTSUPP) {
		return false, errUnsupported
	}
	return false, err
}

func unlock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
