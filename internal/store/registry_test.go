package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestRegistryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	defaults := model.DefaultSettings()
	defaults.Numbering.Enabled = true
	defaults.Output = model.OutputChapter
	registry := &Registry{Version: RegistryVersion, Books: []RegistryEntry{
		{ID: "clabok", Path: ".", Kind: KindLibrary},
		{ID: "guide", Path: "Docs/guide", Kind: KindBook},
	}, Defaults: &defaults}
	if err := registry.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != RegistryVersion || len(loaded.Books) != 2 {
		t.Fatalf("registry %+v", loaded)
	}
	if loaded.Books[0].ID != "clabok" || loaded.Books[0].Path != "." ||
		loaded.Books[0].Kind != KindLibrary {
		t.Fatalf("entry %+v", loaded.Books[0])
	}
	if loaded.Defaults == nil || !loaded.Defaults.Numbering.Enabled ||
		loaded.Defaults.Output != model.OutputChapter {
		t.Fatalf("defaults %+v", loaded.Defaults)
	}

	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Save(path); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(second) {
		t.Fatal("registry save is not deterministic")
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
}

func TestRegistryMissing(t *testing.T) {
	registry, err := LoadRegistry(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if registry.Version != RegistryVersion || len(registry.Books) != 0 {
		t.Fatalf("registry %+v", registry)
	}
}

func TestRegistryTolerantDecode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	document := `{"version":1,"books":[],"futureField":true}`
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegistry(path); err != nil {
		t.Fatalf("an unknown field is tolerated: %v", err)
	}
}

func TestRegistryRejects(t *testing.T) {
	absolute := filepath.ToSlash(t.TempDir())
	cases := []struct {
		name     string
		document string
		code     string
	}{
		{
			name:     "version",
			document: `{"version":3,"books":[]}`,
			code:     "store.registry-version",
		},
		{
			name: "duplicate id",
			document: `{"version":2,"books":[
				{"id":"a","path":"Docs/a","kind":"book"},
				{"id":"a","path":"Docs/b","kind":"book"}]}`,
			code: "store.registry-invalid",
		},
		{
			name: "duplicate path",
			document: `{"version":2,"books":[
				{"id":"a","path":"Docs/a","kind":"book"},
				{"id":"b","path":"Docs/a/","kind":"book"}]}`,
			code: "store.registry-invalid",
		},
		{
			name: "second library",
			document: `{"version":2,"books":[
				{"id":"a","path":".","kind":"library"},
				{"id":"b","path":"Docs/b","kind":"library"}]}`,
			code: "store.registry-invalid",
		},
		{
			name:     "library path",
			document: `{"version":2,"books":[{"id":"a","path":"Docs/a","kind":"library"}]}`,
			code:     "store.registry-invalid",
		},
		{
			name:     "escaping path",
			document: `{"version":2,"books":[{"id":"a","path":"../a","kind":"book"}]}`,
			code:     "store.registry-invalid",
		},
		{
			name:     "empty id",
			document: `{"version":2,"books":[{"id":"","path":"Docs/a","kind":"book"}]}`,
			code:     "store.registry-invalid",
		},
		{
			name:     "empty path",
			document: `{"version":2,"books":[{"id":"a","path":"","kind":"book"}]}`,
			code:     "store.registry-invalid",
		},
		{
			name:     "absolute path",
			document: `{"version":2,"books":[{"id":"a","path":"` + absolute + `","kind":"book"}]}`,
			code:     "store.registry-invalid",
		},
		{
			name:     "unknown kind",
			document: `{"version":2,"books":[{"id":"a","path":"Docs/a","kind":"manual"}]}`,
			code:     "store.registry-invalid",
		},
		{
			name:     "corrupt",
			document: `{`,
			code:     "store.registry-corrupt",
		},
	}
	for _, testCase := range cases {
		path := filepath.Join(t.TempDir(), "library.json")
		if err := os.WriteFile(path, []byte(testCase.document), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadRegistry(path)
		if err == nil {
			t.Fatalf("%s: expected an error", testCase.name)
		}
		expectCode(t, err, testCase.code)
	}
}

func TestRegistryUpgradesVersion1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	document := `{"version":1,"books":[{"id":"library","path":".","kind":"library"}]}`
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := LoadRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if registry.Version != RegistryVersion || registry.Defaults != nil {
		t.Fatalf("upgraded registry %+v", registry)
	}
	if err := registry.Save(path); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), `"version": 2`) {
		t.Fatalf("the registry was not written as version 2:\n%s", saved)
	}
}

func TestRegistryUpsertAndRemove(t *testing.T) {
	registry := &Registry{Version: RegistryVersion}
	registry.Upsert(RegistryEntry{ID: "a", Path: "Docs/a", Kind: KindBook})
	registry.Upsert(RegistryEntry{ID: "b", Path: "Docs/b", Kind: KindBook})
	if len(registry.Books) != 2 || registry.Books[1].ID != "b" {
		t.Fatalf("books %+v", registry.Books)
	}

	registry.Upsert(RegistryEntry{ID: "a", Path: "Docs/a2", Kind: KindLibrary})
	if len(registry.Books) != 2 || registry.Books[0].Path != "Docs/a2" ||
		registry.Books[0].Kind != KindLibrary {
		t.Fatalf("upsert did not replace in place: %+v", registry.Books)
	}

	if entry, ok := registry.Find("b"); !ok || entry.Path != "Docs/b" {
		t.Fatalf("find %+v %t", entry, ok)
	}
	if _, ok := registry.Find("missing"); ok {
		t.Fatal("found a missing entry")
	}

	if !registry.Remove("a") {
		t.Fatal("remove reported no change")
	}
	if registry.Remove("a") {
		t.Fatal("second remove reported a change")
	}
	if len(registry.Books) != 1 || registry.Books[0].ID != "b" {
		t.Fatalf("books after remove %+v", registry.Books)
	}
}

func TestRegistrySaveValidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	registry := &Registry{Version: RegistryVersion, Books: []RegistryEntry{
		{ID: "", Path: "Docs/a", Kind: KindBook},
	}}
	expectCode(t, registry.Save(path), "store.registry-invalid")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("an invalid registry was written: %v", err)
	}
}
