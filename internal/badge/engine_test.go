package badge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func projectFixture(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	files := map[string]string{
		"ProjectSettings/ProjectVersion.txt": "m_EditorVersion: 6000.1.0f1\n",
		"package.json":                       `{"name":"demo","engines":{"node":">=22.0.0"}}`,
		"Cargo.toml":                         "[package]\nname = \"demo\"\nrust-version = \"1.80\"\n",
		"rust-toolchain.toml":                "[toolchain]\nchannel = \"stable\"\n",
		"Demo.csproj":                        `<Project><PropertyGroup><TargetFramework>net8.0</TargetFramework></PropertyGroup></Project>`,
		"LICENSE":                            "MIT License\n",
		"docs/a.txt":                         "one\n",
		"docs/b.txt":                         "two\n",
		".git/HEAD":                          "ref: refs/heads/main\n",
		".git/config":                        "[remote \"origin\"]\n\turl = git@github.com:owner/repo.git\n",
	}
	for name, content := range files {
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func TestResolverKinds(t *testing.T) {
	directory := projectFixture(t)
	resolver := Resolver{
		ProjectDir: directory,
		Detectors: map[string]Detector{
			"unity-editor": {Kind: KindFile, Path: "ProjectSettings/ProjectVersion.txt",
				Format: FormatRegex, Pattern: `m_EditorVersion:\s*(\S+)`},
			"node": {Kind: KindPackage, Ecosystem: "node", Field: "engines.node"},
			"rust": {Kind: KindFile, Path: "rust-toolchain.toml", Format: FormatTOML,
				Query: "toolchain.channel"},
			"dotnet":  {Kind: KindPackage, Ecosystem: "dotnet", Field: "TargetFramework"},
			"counted": {Kind: KindCount, Path: "docs/*.txt"},
			"present": {Kind: KindExists, Path: "LICENSE"},
			"static":  {Kind: KindStatic, Value: "3.1"},
			"encoded": {Kind: KindHTTP, URL: "https://example.com/status.svg"},
			"branch":  {Kind: KindGit, Query: "branch"},
			"build":   {Kind: KindBuild, Query: "version"},
			"cargo":   {Kind: KindPackage, Ecosystem: "rust", Field: "rust-version"},
			"yaml": {Kind: KindFile, Path: "package.json", Format: FormatJSON,
				Query: "engines.node"},
		},
	}
	cases := []struct {
		id      string
		message string
		source  string
	}{
		{"unity-editor", "6000.1.0f1", "ProjectSettings/ProjectVersion.txt"},
		{"node", ">=22.0.0", "package.json"},
		{"rust", "stable", "rust-toolchain.toml"},
		{"dotnet", "net8.0", "Demo.csproj"},
		{"counted", "2", "docs/*.txt"},
		{"present", "yes", "LICENSE"},
		{"static", "3.1", "config"},
		{"cargo", "1.80", "Cargo.toml"},
		{"branch", "main", ".git/HEAD"},
	}
	for _, testCase := range cases {
		value, ok, reason := resolver.Resolve(model.Badge{ID: testCase.id})
		if !ok {
			t.Fatalf("%s: %s", testCase.id, reason)
		}
		if value.Message != testCase.message || value.Source != testCase.source {
			t.Fatalf("%s: %+v", testCase.id, value)
		}
	}
	value, ok, _ := resolver.Resolve(model.Badge{ID: "encoded"})
	if !ok || value.URL != "https://example.com/status.svg" || !value.Dynamic {
		t.Fatalf("http: %+v", value)
	}
	// The build detector is the running binary's version.
	if value, ok, _ := resolver.Resolve(model.Badge{ID: "build"}); !ok || value.Message == "" {
		t.Fatalf("build: %+v", value)
	}
}

func TestResolverPresentationsAndGroups(t *testing.T) {
	directory := projectFixture(t)
	resolver := Resolver{ProjectDir: directory}
	declared := model.Badge{
		ID: "license", Style: "plastic", Logo: "go", LabelColor: "blue", Alt: "license",
		Order: 3, Group: "stack",
	}
	value, ok, reason := resolver.Resolve(declared)
	if !ok {
		t.Fatal(reason)
	}
	url := BadgeURL(value)
	if !strings.Contains(url, "style=plastic") || !strings.Contains(url, "logo=go") ||
		!strings.Contains(url, "labelColor=blue") {
		t.Fatalf("url %s", url)
	}
	if AltText(value) != "license" || value.Order != 3 || value.Group != "stack" {
		t.Fatalf("value %+v", value)
	}
}

func TestResolverCommandIsOptIn(t *testing.T) {
	directory := projectFixture(t)
	detector := Detector{Kind: KindCommand, Command: []string{"go", "version"}}
	resolver := Resolver{
		ProjectDir: directory,
		Detectors:  map[string]Detector{"goversion": detector},
	}
	_, ok, reason := resolver.Resolve(model.Badge{ID: "goversion"})
	if ok || !strings.Contains(reason, "badge check") {
		t.Fatalf("a command source must stay opt-in: %v %q", ok, reason)
	}
}

func TestResolverCommandCaches(t *testing.T) {
	directory := projectFixture(t)
	cache := filepath.Join(directory, "temp")
	resolver := Resolver{
		ProjectDir:    directory,
		Detectors:     map[string]Detector{"goversion": {Kind: KindCommand, Command: []string{"go", "version"}}},
		CacheDir:      cache,
		AllowExternal: true,
		Now:           func() time.Time { return time.Now() },
	}
	value, ok, reason := resolver.Resolve(model.Badge{ID: "goversion", CacheSeconds: 300})
	if !ok || !strings.HasPrefix(value.Message, "go version") || value.Source != "command" {
		t.Fatalf("command: %+v %q", value, reason)
	}
	cached, ok, _ := resolver.Resolve(model.Badge{ID: "goversion", CacheSeconds: 300})
	if !ok || cached.Source != "cache" {
		t.Fatalf("cache: %+v", cached)
	}
}

func TestParseXMLQuery(t *testing.T) {
	root, err := parseXML([]byte(
		`<Project><PropertyGroup><TargetFramework>net8.0</TargetFramework></PropertyGroup></Project>`))
	if err != nil {
		t.Fatal(err)
	}
	value, ok := queryValue(root, "Project/PropertyGroup/TargetFramework")
	if !ok || formatValue(value) != "net8.0" {
		t.Fatalf("xml query: %v %v", value, ok)
	}
}

func TestDetectorValidationRejectsSecrets(t *testing.T) {
	cases := []Detector{
		{Kind: KindHTTP, URL: "http://example.com/badge.svg"},
		{Kind: KindHTTP, URL: "https://user:pass@example.com/badge.svg"},
		{Kind: KindHTTP, URL: "https://example.com/badge.svg?token=abc"},
		{Kind: KindHTTP, URL: "https://example.com/${TOKEN}.svg"},
		{Kind: KindFile, Path: "../outside.txt", Format: FormatText},
		{Kind: KindCommand, Command: []string{"env"}},
		{Kind: KindCommand, Command: []string{"echo", "$SECRET"}},
		{Kind: KindFile, Path: "go.mod", Format: FormatYAML},
		{Kind: KindPackage, Ecosystem: "java", Field: "version"},
	}
	for _, detector := range cases {
		if err := ValidateDetector("test", detector); err == nil {
			t.Fatalf("expected a validation error for %+v", detector)
		}
	}
	valid := Detector{Kind: KindFile, Path: "**/*.csproj", Format: FormatXML,
		Query: "Project/PropertyGroup/TargetFramework"}
	if err := ValidateDetector("test", valid); err != nil {
		t.Fatalf("valid detector rejected: %v", err)
	}
}

func TestBuiltinIDsAreReserved(t *testing.T) {
	if !BuiltinID("license") || !BuiltinID("go") || BuiltinID("unity-editor") {
		t.Fatal("builtin ids")
	}
}
