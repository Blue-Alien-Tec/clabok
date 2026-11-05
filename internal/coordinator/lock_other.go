//go:build !windows && !unix

package coordinator

import (
	"errors"
	"os"
)

var errUnsupported = errors.New("file locking is not supported on this platform")

func tryLock(*os.File, Mode) (bool, error) { return false, errUnsupported }

func unlock(*os.File) error { return nil }
