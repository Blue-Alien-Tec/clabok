package coordinator

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// File is the lock-file backend. It uses OS advisory locks and keeps the holder metadata behind the
// locked byte.
type File struct {
	// Dir is the lock directory. It must be absolute.
	Dir string
	// Timeout is used when Acquire receives no timeout.
	Timeout time.Duration
	// Command names this writer in the holder metadata.
	Command string
}

// NewFile returns a file backend for the given lock directory.
func NewFile(directory string) (*File, error) {
	if directory == "" {
		return nil, fail("coordinator.io", "the lock directory must not be empty")
	}
	if !filepath.IsAbs(directory) {
		return nil, fail("coordinator.io", "the lock directory must be absolute: %s", directory)
	}
	return &File{Dir: filepath.Clean(directory), Timeout: DefaultTimeout}, nil
}

var _ Coordinator = (*File)(nil)

type holder struct {
	PID       int      `json:"pid"`
	StartedAt string   `json:"startedAt"`
	Command   string   `json:"command"`
	Resources []string `json:"resources"`
}

type lockHandle struct {
	file *os.File
}

func (handle *lockHandle) release() error {
	if handle == nil || handle.file == nil {
		return nil
	}
	unlockErr := unlock(handle.file)
	closeErr := handle.file.Close()
	handle.file = nil
	if unlockErr != nil {
		return fail("coordinator.io", "%v", unlockErr)
	}
	if closeErr != nil {
		return fail("coordinator.io", "%v", closeErr)
	}
	return nil
}

// Acquire locks the resource set. Resources are ordered internally, so callers may pass them in any
// order. An empty set succeeds with a no-op release. Shared mode allows several holders; exclusive
// mode excludes them.
func (backend *File) Acquire(resources []Resource, mode Mode, timeout time.Duration) (Release, error) {
	if mode != Exclusive && mode != Shared {
		return nil, fail("coordinator.invalid-mode", "unknown lock mode %d", mode)
	}
	ordered, err := sortResources(resources)
	if err != nil {
		return nil, err
	}
	if len(ordered) == 0 {
		return func() error { return nil }, nil
	}
	if timeout <= 0 {
		timeout = backend.Timeout
		if timeout <= 0 {
			timeout = DefaultTimeout
		}
	}
	if err := os.MkdirAll(backend.Dir, 0o755); err != nil {
		return nil, fail("coordinator.io", "%v", err)
	}

	deadline := time.Now().Add(timeout)
	delay := 50 * time.Millisecond
	acquired := make([]*lockHandle, 0, len(ordered))
	releaseAll := func() {
		for index := len(acquired) - 1; index >= 0; index-- {
			_ = acquired[index].release()
		}
	}

	for _, resource := range ordered {
		handle, acquireErr := backend.acquireOne(resource, mode, deadline, &delay, timeout)
		if acquireErr != nil {
			releaseAll()
			return nil, acquireErr
		}
		acquired = append(acquired, handle)
	}

	released := false
	var releaseErr error
	return func() error {
		if released {
			return releaseErr
		}
		released = true
		for index := len(acquired) - 1; index >= 0; index-- {
			if err := acquired[index].release(); err != nil && releaseErr == nil {
				releaseErr = err
			}
		}
		return releaseErr
	}, nil
}

func (backend *File) acquireOne(resource Resource, mode Mode, deadline time.Time, delay *time.Duration, waited time.Duration) (*lockHandle, error) {
	path := filepath.Join(backend.Dir, resource.lockName())
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fail("coordinator.io", "%s: %v", path, err)
	}

	for {
		locked, lockErr := tryLock(file, mode)
		if lockErr != nil {
			_ = file.Close()
			if errors.Is(lockErr, errUnsupported) {
				return nil, fail("coordinator.unsupported", "%s: %v", path, lockErr)
			}
			return nil, fail("coordinator.io", "%s: %v", path, lockErr)
		}
		if locked {
			break
		}
		if !time.Now().Before(deadline) {
			holderText := readHolder(path)
			_ = file.Close()
			return nil, fail("coordinator.busy",
				"%s is held by %s; waited %s", resource, holderText, waited)
		}
		time.Sleep(*delay)
		if *delay < 250*time.Millisecond {
			*delay *= 2
			if *delay > 250*time.Millisecond {
				*delay = 250 * time.Millisecond
			}
		}
	}

	if mode == Exclusive {
		if err := writeHolder(file, backend, resource); err != nil {
			_ = unlock(file)
			_ = file.Close()
			return nil, fail("coordinator.io", "%s: %v", path, err)
		}
	}
	return &lockHandle{file: file}, nil
}

// writeHolder records the holder behind the locked byte. Byte 0 stays zero, so a reader can skip
// the mandatory-locked byte on Windows.
func writeHolder(file *os.File, backend *File, resource Resource) error {
	payload, err := json.Marshal(holder{
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Command:   backend.Command,
		Resources: []string{resource.String()},
	})
	if err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Write(append([]byte{0}, payload...)); err != nil {
		return err
	}
	return file.Sync()
}

func readHolder(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return "another process"
	}
	defer file.Close()
	if _, err := file.Seek(1, io.SeekStart); err != nil {
		return "another process"
	}
	data, err := io.ReadAll(io.LimitReader(file, 4096))
	if err != nil {
		return "another process"
	}
	var meta holder
	if err := json.Unmarshal(data, &meta); err != nil {
		return "another process"
	}
	if meta.Command != "" {
		return fmt.Sprintf("pid %d (%s, started %s)", meta.PID, meta.Command, meta.StartedAt)
	}
	return fmt.Sprintf("pid %d", meta.PID)
}
