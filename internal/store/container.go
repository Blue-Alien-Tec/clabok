// Package store owns the deterministic book container and the working store layout.
//
// A container is a ZIP archive with a fixed member order, a fixed timestamp, stored (uncompressed)
// members and a SHA-256 per member. Re-saving an unchanged container produces identical bytes on
// every platform and across Go versions, because no compressor output is involved.
package store

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// Format is the container format version written into container.json.
const Format = 1

const (
	metaMember      = "container.json"
	checksumsMember = "checksums.json"
	maxMemberSize   = 64 << 20
)

var fixedTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

var reservedMembers = map[string]struct{}{
	metaMember:      {},
	checksumsMember: {},
}

// Error is a container failure with a stable code.
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

type metaFile struct {
	Format int `json:"format"`
}

// Container holds the members of one book. The zero value is not usable; call New.
type Container struct {
	members map[string][]byte
}

// New returns an empty container.
func New() *Container {
	return &Container{members: map[string][]byte{}}
}

// Names returns the member names sorted bytewise.
func (container *Container) Names() []string {
	names := make([]string, 0, len(container.members))
	for name := range container.members {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Len returns the number of non-reserved members.
func (container *Container) Len() int { return len(container.members) }

// Read returns a copy of a member.
func (container *Container) Read(name string) ([]byte, bool) {
	data, exists := container.members[name]
	if !exists {
		return nil, false
	}
	return append([]byte(nil), data...), true
}

// Text returns a member as text.
func (container *Container) Text(name string) (string, bool) {
	data, exists := container.members[name]
	if !exists {
		return "", false
	}
	return string(data), true
}

// Write stores a member. The data is copied.
func (container *Container) Write(name string, data []byte) error {
	if _, reserved := reservedMembers[name]; reserved {
		return fail("store.reserved-member", "member name %q is reserved", name)
	}
	if !validMemberName(name) {
		return fail("store.member-name", "invalid member name %q", name)
	}
	if len(data) > maxMemberSize {
		return fail("store.member-too-large", "member %q exceeds %d bytes", name, maxMemberSize)
	}
	container.members[name] = append([]byte(nil), data...)
	return nil
}

// WriteText stores a member from text.
func (container *Container) WriteText(name, text string) error {
	return container.Write(name, []byte(text))
}

// Remove deletes a member and reports whether it existed.
func (container *Container) Remove(name string) bool {
	if _, exists := container.members[name]; !exists {
		return false
	}
	delete(container.members, name)
	return true
}

// Marshal encodes the container deterministically.
func (container *Container) Marshal() ([]byte, error) {
	return writeArchive(container.members)
}

func writeArchive(members map[string][]byte) ([]byte, error) {
	checksums := make(map[string]string, len(members))
	for name, data := range members {
		if _, reserved := reservedMembers[name]; reserved {
			return nil, fail("store.reserved-member", "member name %q is reserved", name)
		}
		if !validMemberName(name) {
			return nil, fail("store.member-name", "invalid member name %q", name)
		}
		sum := sha256.Sum256(data)
		checksums[name] = hex.EncodeToString(sum[:])
	}

	meta, err := json.MarshalIndent(metaFile{Format: Format}, "", "  ")
	if err != nil {
		return nil, fail("store.write-failed", "encode %s: %v", metaMember, err)
	}
	checksumData, err := json.MarshalIndent(checksums, "", "  ")
	if err != nil {
		return nil, fail("store.write-failed", "encode %s: %v", checksumsMember, err)
	}

	files := make(map[string][]byte, len(members)+2)
	for name, data := range members {
		files[name] = data
	}
	files[metaMember] = append(meta, '\n')
	files[checksumsMember] = append(checksumData, '\n')

	order := make([]string, 0, len(files))
	order = append(order, metaMember, checksumsMember)
	order = append(order, sortedNames(members)...)

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range order {
		header := &zip.FileHeader{Name: name, Method: zip.Store, Modified: fixedTime}
		header.SetMode(0o644)
		entry, createErr := writer.CreateHeader(header)
		if createErr != nil {
			return nil, fail("store.write-failed", "create %s: %v", name, createErr)
		}
		if _, writeErr := entry.Write(files[name]); writeErr != nil {
			return nil, fail("store.write-failed", "write %s: %v", name, writeErr)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fail("store.write-failed", "close archive: %v", err)
	}
	return buffer.Bytes(), nil
}

// Parse reads and verifies a container. It checks the format, the member methods, the member set
// and every checksum before it returns.
func Parse(data []byte) (*Container, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fail("store.container-corrupt", "not a container archive: %v", err)
	}
	members := map[string][]byte{}
	seen := map[string]struct{}{}
	var meta *metaFile
	var checksums map[string]string

	for _, file := range reader.File {
		if !validMemberName(file.Name) {
			return nil, fail("store.member-name", "invalid member name %q", file.Name)
		}
		if _, duplicate := seen[file.Name]; duplicate {
			return nil, fail("store.duplicate-member", "member %q appears twice", file.Name)
		}
		seen[file.Name] = struct{}{}
		if file.Method != zip.Store {
			return nil, fail("store.member-method", "member %q is not stored (method %d)", file.Name, file.Method)
		}
		if file.UncompressedSize64 > maxMemberSize {
			return nil, fail("store.member-too-large", "member %q exceeds %d bytes", file.Name, maxMemberSize)
		}
		content, readErr := readMember(file)
		if readErr != nil {
			return nil, readErr
		}
		switch file.Name {
		case metaMember:
			var decoded metaFile
			if decodeErr := json.Unmarshal(content, &decoded); decodeErr != nil {
				return nil, fail("store.container-corrupt", "invalid %s: %v", metaMember, decodeErr)
			}
			meta = &decoded
		case checksumsMember:
			var decoded map[string]string
			if decodeErr := json.Unmarshal(content, &decoded); decodeErr != nil {
				return nil, fail("store.container-corrupt", "invalid %s: %v", checksumsMember, decodeErr)
			}
			checksums = decoded
		default:
			members[file.Name] = content
		}
	}

	if meta == nil {
		return nil, fail("store.container-corrupt", "missing %s", metaMember)
	}
	if meta.Format != Format {
		return nil, fail("store.container-format",
			"unsupported container format %d (expected %d)", meta.Format, Format)
	}
	if checksums == nil {
		return nil, fail("store.checksum-missing", "missing %s", checksumsMember)
	}
	for name, content := range members {
		expected, exists := checksums[name]
		if !exists {
			return nil, fail("store.checksum-missing", "member %q has no checksum", name)
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != expected {
			return nil, fail("store.checksum-mismatch", "checksum mismatch for %q", name)
		}
	}
	for name := range checksums {
		if _, exists := members[name]; !exists {
			return nil, fail("store.checksum-orphan", "checksum for absent member %q", name)
		}
	}
	return &Container{members: members}, nil
}

func readMember(file *zip.File) ([]byte, *Error) {
	reader, err := file.Open()
	if err != nil {
		return nil, fail("store.container-corrupt", "open %q: %v", file.Name, err)
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, maxMemberSize+1))
	if err != nil {
		return nil, fail("store.container-corrupt", "read %q: %v", file.Name, err)
	}
	if len(content) > maxMemberSize {
		return nil, fail("store.member-too-large", "member %q exceeds %d bytes", file.Name, maxMemberSize)
	}
	return content, nil
}

// Load reads and verifies a container from disk.
func Load(path string) (*Container, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fail("store.read-failed", "%v", err)
	}
	return Parse(data)
}

// Save writes the container atomically: a temp sibling is written, synced and renamed.
func (container *Container) Save(path string) error {
	data, err := container.Marshal()
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

// validMemberName keeps names portable and inside the archive. Forward slashes only, no absolute
// names, no traversal, no Windows separators and no drive letters.
func validMemberName(name string) bool {
	if name == "" || len(name) > 1024 {
		return false
	}
	if strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	for _, character := range name {
		if character < 0x20 {
			return false
		}
	}
	return true
}

func sortedNames(members map[string][]byte) []string {
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
