//go:build windows

package coordinator

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var errUnsupported = errors.New("file locking is not supported on this file system")

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	errorInvalidFunction    = syscall.Errno(1)
	errorLockViolation      = syscall.Errno(33)
	errorNotSupported       = syscall.Errno(50)
)

// tryLock locks byte 0 of the file without blocking.
func tryLock(file *os.File, mode Mode) (bool, error) {
	flags := uintptr(lockfileFailImmediately)
	if mode == Exclusive {
		flags |= lockfileExclusiveLock
	}
	var overlapped syscall.Overlapped
	result, _, callErr := procLockFileEx.Call(
		file.Fd(),
		flags,
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if result != 0 {
		return true, nil
	}
	errno, ok := callErr.(syscall.Errno)
	if !ok || errno == 0 {
		return false, syscall.EINVAL
	}
	switch errno {
	case errorLockViolation:
		return false, nil
	case errorNotSupported, errorInvalidFunction:
		return false, errUnsupported
	}
	return false, errno
}

func unlock(file *os.File) error {
	var overlapped syscall.Overlapped
	result, _, callErr := procUnlockFileEx.Call(
		file.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if result != 0 {
		return nil
	}
	if errno, ok := callErr.(syscall.Errno); ok && errno != 0 {
		return errno
	}
	return nil
}
