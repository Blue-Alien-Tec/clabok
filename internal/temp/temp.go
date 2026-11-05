// Package temp manages the project temp area for transient authoring inputs. Nothing in the store
// is touched.
package temp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// Error is a temp failure with a stable code.
type Error struct {
	Code    string
	Message string
}

// Error returns the human message.
func (err *Error) Error() string { return err.Message }

// ErrorCode returns the stable code for the CLI boundary.
func (err *Error) ErrorCode() string { return err.Code }

func fail(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Entry is one file in the temp area.
type Entry struct {
	Path     string
	Book     string
	Size     int64
	Modified time.Time
}

// Service manages the temp root.
type Service struct {
	Root string
}

// New returns a temp service for an absolute root.
func New(root string) (*Service, error) {
	if root == "" || !filepath.IsAbs(root) {
		return nil, fail("temp.invalid-root", "the temp root must be an absolute path")
	}
	return &Service{Root: filepath.Clean(root)}, nil
}

// List returns the files of one book, or of every book when all is set.
func (service *Service) List(book model.BookID, all bool) ([]Entry, error) {
	if !all && !book.Valid() {
		return nil, fail("temp.scope-required", "pass a book or --all")
	}
	base := service.Root
	if !all {
		base = filepath.Join(service.Root, string(book))
	}
	entries := make([]Entry, 0)
	walkErr := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		bookName := ""
		if relative, relErr := filepath.Rel(service.Root, path); relErr == nil {
			parts := strings.Split(filepath.ToSlash(relative), "/")
			if len(parts) > 1 {
				bookName = parts[0]
			}
		}
		entries = append(entries, Entry{
			Path: path, Book: bookName, Size: info.Size(), Modified: info.ModTime(),
		})
		return nil
	})
	if walkErr != nil {
		return nil, fail("temp.read-failed", "%v", walkErr)
	}
	return entries, nil
}

// Delete removes the listed files. Every path must stay inside the temp root.
func (service *Service) Delete(entries []Entry) (int, error) {
	deleted := 0
	for _, entry := range entries {
		if !within(service.Root, entry.Path) {
			return deleted, fail("temp.invalid-path", "%s is outside the temp root", entry.Path)
		}
		if err := os.Remove(entry.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return deleted, fail("temp.write-failed", "%v", err)
		}
		deleted++
	}
	return deleted, nil
}

func within(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
