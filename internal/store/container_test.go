package store

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureContainer(t *testing.T) *Container {
	t.Helper()
	container := New()
	if err := container.WriteText("manifest.json", "{\n  \"version\": 1\n}\n"); err != nil {
		t.Fatal(err)
	}
	if err := container.WriteText("blocks/1.md", "First body\n"); err != nil {
		t.Fatal(err)
	}
	if err := container.WriteText("blocks/2.md", ""); err != nil {
		t.Fatal(err)
	}
	return container
}

func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	var storeErr *Error
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected %s, got %v", code, err)
	}
	if storeErr.Code != code {
		t.Fatalf("expected %s, got %s (%s)", code, storeErr.Code, storeErr.Message)
	}
}

func TestRoundTrip(t *testing.T) {
	container := fixtureContainer(t)
	data, err := container.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Len() != 3 {
		t.Fatalf("members %v", parsed.Names())
	}
	for _, name := range container.Names() {
		want, _ := container.Read(name)
		got, ok := parsed.Read(name)
		if !ok || !bytes.Equal(want, got) {
			t.Fatalf("member %q differs", name)
		}
	}
}

func TestDeterministicBytes(t *testing.T) {
	first := New()
	if err := first.WriteText("manifest.json", "{}\n"); err != nil {
		t.Fatal(err)
	}
	if err := first.WriteText("blocks/1.md", "One\n"); err != nil {
		t.Fatal(err)
	}
	if err := first.WriteText("blocks/2.md", "Two\n"); err != nil {
		t.Fatal(err)
	}

	second := New()
	if err := second.WriteText("blocks/2.md", "Two\n"); err != nil {
		t.Fatal(err)
	}
	if err := second.WriteText("manifest.json", "{}\n"); err != nil {
		t.Fatal(err)
	}
	if err := second.WriteText("blocks/1.md", "One\n"); err != nil {
		t.Fatal(err)
	}

	firstData, err := first.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	secondData, err := second.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstData, secondData) {
		t.Fatal("insertion order changed the container bytes")
	}
	again, err := first.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstData, again) {
		t.Fatal("marshal is not idempotent")
	}
}

func TestMemberOrder(t *testing.T) {
	data, err := fixtureContainer(t).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	want := []string{metaMember, checksumsMember, "blocks/1.md", "blocks/2.md", "manifest.json"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("order %v, want %v", names, want)
	}
	for _, file := range reader.File {
		if file.Method != zip.Store {
			t.Fatalf("%s is not stored", file.Name)
		}
		if !file.Modified.Equal(fixedTime) {
			t.Fatalf("%s timestamp %v", file.Name, file.Modified)
		}
	}
}

func TestFixture(t *testing.T) {
	data, err := fixtureContainer(t).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "container-v1.zip")
	if os.Getenv("UPDATE_FIXTURES") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s", path)
		return
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing fixture: %v (run UPDATE_FIXTURES=1 go test ./internal/store)", err)
	}
	if !bytes.Equal(expected, data) {
		t.Fatal("container bytes differ from the committed fixture")
	}
	if _, err := Parse(expected); err != nil {
		t.Fatalf("fixture does not parse: %v", err)
	}
}

func metaBytes(t *testing.T, format int) string {
	t.Helper()
	data, err := json.MarshalIndent(metaFile{Format: format}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + "\n"
}

func checksumBytes(t *testing.T, checksums map[string]string) string {
	t.Helper()
	data, err := json.MarshalIndent(checksums, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + "\n"
}

type rawEntry struct {
	name    string
	content string
	method  uint16
}

func rawArchive(t *testing.T, entries []rawEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{
			Name:     entry.name,
			Method:   entry.method,
			Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		handle, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := handle.Write([]byte(entry.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestParseRejects(t *testing.T) {
	valid, err := fixtureContainer(t).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		data []byte
		code string
	}{
		{
			name: "not an archive",
			data: []byte("nope"),
			code: "store.container-corrupt",
		},
		{
			name: "truncated",
			data: valid[:len(valid)-12],
			code: "store.container-corrupt",
		},
		{
			name: "missing meta",
			data: rawArchive(t, []rawEntry{
				{name: checksumsMember, content: checksumBytes(t, map[string]string{})},
			}),
			code: "store.container-corrupt",
		},
		{
			name: "unsupported format",
			data: rawArchive(t, []rawEntry{
				{name: metaMember, content: metaBytes(t, 2)},
				{name: checksumsMember, content: checksumBytes(t, map[string]string{})},
			}),
			code: "store.container-format",
		},
		{
			name: "missing checksums",
			data: rawArchive(t, []rawEntry{
				{name: metaMember, content: metaBytes(t, Format)},
			}),
			code: "store.checksum-missing",
		},
		{
			name: "member without checksum",
			data: rawArchive(t, []rawEntry{
				{name: metaMember, content: metaBytes(t, Format)},
				{name: checksumsMember, content: checksumBytes(t, map[string]string{})},
				{name: "blocks/1.md", content: "x"},
			}),
			code: "store.checksum-missing",
		},
		{
			name: "checksum mismatch",
			data: rawArchive(t, []rawEntry{
				{name: metaMember, content: metaBytes(t, Format)},
				{name: checksumsMember, content: checksumBytes(t, map[string]string{
					"blocks/1.md": strings.Repeat("0", 64),
				})},
				{name: "blocks/1.md", content: "x"},
			}),
			code: "store.checksum-mismatch",
		},
		{
			name: "orphan checksum",
			data: rawArchive(t, []rawEntry{
				{name: metaMember, content: metaBytes(t, Format)},
				{name: checksumsMember, content: checksumBytes(t, map[string]string{
					"ghost.md": strings.Repeat("0", 64),
				})},
			}),
			code: "store.checksum-orphan",
		},
		{
			name: "duplicate member",
			data: rawArchive(t, []rawEntry{
				{name: metaMember, content: metaBytes(t, Format)},
				{name: checksumsMember, content: checksumBytes(t, map[string]string{})},
				{name: "blocks/1.md", content: "a"},
				{name: "blocks/1.md", content: "b"},
			}),
			code: "store.duplicate-member",
		},
		{
			name: "deflated member",
			data: rawArchive(t, []rawEntry{
				{name: metaMember, content: metaBytes(t, Format)},
				{name: checksumsMember, content: checksumBytes(t, map[string]string{})},
				{name: "blocks/1.md", content: "a", method: zip.Deflate},
			}),
			code: "store.member-method",
		},
		{
			name: "traversal name",
			data: rawArchive(t, []rawEntry{
				{name: metaMember, content: metaBytes(t, Format)},
				{name: checksumsMember, content: checksumBytes(t, map[string]string{})},
				{name: "../evil", content: "a"},
			}),
			code: "store.member-name",
		},
		{
			name: "drive path",
			data: rawArchive(t, []rawEntry{
				{name: metaMember, content: metaBytes(t, Format)},
				{name: checksumsMember, content: checksumBytes(t, map[string]string{})},
				{name: "C:/absolute", content: "a"},
			}),
			code: "store.member-name",
		},
	}
	for _, testCase := range cases {
		_, err := Parse(testCase.data)
		if err == nil {
			t.Fatalf("%s: expected an error", testCase.name)
		}
		expectCode(t, err, testCase.code)
	}
}

func TestWriteRejects(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{name: metaMember, code: "store.reserved-member"},
		{name: checksumsMember, code: "store.reserved-member"},
		{name: "", code: "store.member-name"},
		{name: "/absolute", code: "store.member-name"},
		{name: "../escape", code: "store.member-name"},
		{name: `windows\path`, code: "store.member-name"},
		{name: "double//slash", code: "store.member-name"},
		{name: "trailing/", code: "store.member-name"},
		{name: "C:/drive", code: "store.member-name"},
	}
	for _, testCase := range cases {
		container := New()
		expectCode(t, container.Write(testCase.name, []byte("x")), testCase.code)
	}
}

func TestRemove(t *testing.T) {
	container := fixtureContainer(t)
	if !container.Remove("blocks/2.md") {
		t.Fatal("remove reported no change")
	}
	if container.Remove("blocks/2.md") {
		t.Fatal("second remove reported a change")
	}
	if _, ok := container.Read("blocks/2.md"); ok {
		t.Fatal("member still present")
	}
	if len(container.Names()) != 2 {
		t.Fatalf("names %v", container.Names())
	}
}

func TestSaveLoad(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "books", "fixture.book")
	container := fixtureContainer(t)
	if err := container.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Len() != 3 {
		t.Fatalf("members %v", loaded.Names())
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", entry.Name())
		}
	}

	if err := container.WriteText("blocks/3.md", "Third\n"); err != nil {
		t.Fatal(err)
	}
	if err := container.Save(path); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Read("blocks/3.md"); !ok {
		t.Fatal("overwritten container lost the new member")
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "absent.book"))
	expectCode(t, err, "store.read-failed")
}
