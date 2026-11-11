package badge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// unityFixture writes a project tree that mirrors a Unity checkout.
func unityFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	directory := t.TempDir()
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

func TestUnityPresetReadsHandAuthoredSources(t *testing.T) {
	directory := unityFixture(t, map[string]string{
		"Directory.Build.targets": "<Project><PropertyGroup><LangVersion>10.0</LangVersion>" +
			"</PropertyGroup></Project>",
		"Assembly-CSharp.csproj": "<Project><PropertyGroup><LangVersion>9.0</LangVersion>" +
			"</PropertyGroup></Project>",
		"ProjectSettings/ProjectSettings.asset": "  scriptingBackend:\n    Android: 1\n" +
			"    Standalone: 1\n",
	})
	preset, _ := Preset("unity")
	resolver := Resolver{ProjectDir: directory, Detectors: preset.Detectors}

	value, ok, reason := resolver.Resolve(model.Badge{ID: "csharp-lang"})
	if !ok || value.Message != "10.0" {
		t.Fatalf("csharp-lang = %q, ok %v, reason %q", value.Message, ok, reason)
	}
	value, ok, reason = resolver.Resolve(model.Badge{ID: "unity-backend"})
	if !ok || value.Message != "IL2CPP" {
		t.Fatalf("unity-backend = %q, ok %v, reason %q", value.Message, ok, reason)
	}
}

func TestUnityPresetIgnoresGeneratedProjectFiles(t *testing.T) {
	directory := unityFixture(t, map[string]string{
		"Assembly-CSharp.csproj": "<Project><PropertyGroup><LangVersion>9.0</LangVersion>" +
			"</PropertyGroup></Project>",
	})
	preset, _ := Preset("unity")
	resolver := Resolver{ProjectDir: directory, Detectors: preset.Detectors}

	if value, ok, _ := resolver.Resolve(model.Badge{ID: "csharp-lang"}); ok {
		t.Fatalf("a generated project file answered the language: %q", value.Message)
	}
}

func TestDetectUnityOwnsTheProject(t *testing.T) {
	directory := unityFixture(t, map[string]string{
		"ProjectSettings/ProjectVersion.txt": "m_EditorVersion: 6000.1.0f1\n",
		"Assembly-CSharp.csproj":             "<Project/>",
		"DNEngine.Unity.csproj":              "<Project/>",
	})
	detections := Detect(directory)
	if len(detections) != 1 || detections[0].Kind != "unity" {
		t.Fatalf("a Unity project must yield one ecosystem: %+v", detections)
	}
}

func TestDetectDotnetProject(t *testing.T) {
	directory := unityFixture(t, map[string]string{"Demo.csproj": "<Project/>"})
	detections := Detect(directory)
	if len(detections) != 1 || detections[0].Kind != "dotnet" ||
		detections[0].Evidence != "Demo.csproj" {
		t.Fatalf("detections %+v", detections)
	}
}

func TestDetectIgnoresGeneratedProjectFiles(t *testing.T) {
	directory := unityFixture(t, map[string]string{
		"Assembly-CSharp-Editor.csproj": "<Project/>",
	})
	if detections := Detect(directory); len(detections) != 0 {
		t.Fatalf("a generated project file is not .NET evidence: %+v", detections)
	}
}

func TestUnityProjectVersionDetector(t *testing.T) {
	directory := unityFixture(t, map[string]string{
		"ProjectSettings/ProjectSettings.asset": "  bundleVersion: 1.2.3\n",
	})
	preset, _ := Preset("unity")
	if BuiltinID("project-version") {
		t.Fatal("project-version must be an ordinary id, not a built-in")
	}
	detector := preset.Detectors["project-version"]
	if detector.Label != "version" || detector.Color != "orange" {
		t.Fatalf("detector %+v", detector)
	}
	resolver := Resolver{ProjectDir: directory, Detectors: preset.Detectors}
	value, ok, reason := resolver.Resolve(model.Badge{ID: "project-version"})
	if !ok || value.Message != "1.2.3" {
		t.Fatalf("project-version = %q, ok %v, reason %q", value.Message, ok, reason)
	}
}

func TestDotnetProjectVersionDetector(t *testing.T) {
	shared := unityFixture(t, map[string]string{
		"Directory.Build.props": "<Project><PropertyGroup><Version>2.0.0</Version>" +
			"</PropertyGroup></Project>",
		"App.csproj": "<Project><PropertyGroup></PropertyGroup></Project>",
	})
	fallback := unityFixture(t, map[string]string{
		"App.csproj": "<Project><PropertyGroup><Version>3.1.0</Version></PropertyGroup></Project>",
	})
	preset, _ := Preset("dotnet")
	cases := map[string]struct {
		directory string
		message   string
	}{
		"shared":   {shared, "2.0.0"},
		"fallback": {fallback, "3.1.0"},
	}
	for name, testCase := range cases {
		resolver := Resolver{ProjectDir: testCase.directory, Detectors: preset.Detectors}
		value, ok, reason := resolver.Resolve(model.Badge{ID: "project-version"})
		if !ok || value.Message != testCase.message {
			t.Fatalf("%s: %q, ok %v, reason %q", name, value.Message, ok, reason)
		}
	}
}

func TestUnityPreset(t *testing.T) {
	preset, ok := Preset("unity")
	if !ok || preset.Name != "unity" {
		t.Fatal("unity preset is missing")
	}
	if len(preset.Detectors) != 4 {
		t.Fatalf("detectors %+v", preset.Detectors)
	}
	ids := map[string]bool{}
	for _, declared := range preset.Badges {
		ids[declared.ID] = true
	}
	if ids["go"] {
		t.Fatal("the unity preset must not carry a Go badge")
	}
	for _, wanted := range []string{"unity-editor", "unity-backend", "csharp-lang", "project-version",
		"license", "remote"} {
		if !ids[wanted] {
			t.Fatalf("missing badge %q", wanted)
		}
	}
	if _, ok := Preset("nonsense"); ok {
		t.Fatal("an unknown preset must not resolve")
	}
}

func TestDetectProjectKinds(t *testing.T) {
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "ProjectSettings"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "ProjectSettings", "ProjectVersion.txt"),
		[]byte("m_EditorVersion: 6000.1.0f1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module x\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	detections := Detect(directory)
	kinds := map[string]bool{}
	for _, detection := range detections {
		kinds[detection.Kind] = true
	}
	if !kinds["unity"] || !kinds["go"] {
		t.Fatalf("detections %+v", detections)
	}
}

func TestDetectorConflicts(t *testing.T) {
	preset, _ := Preset("unity")
	configured := map[string]Detector{
		"unity-editor": {Kind: KindStatic, Value: "other"},
		"extra":        {Kind: KindStatic, Value: "x"},
	}
	conflicts := DetectorConflicts(preset, configured)
	if len(conflicts) != 1 || conflicts[0] != "unity-editor" {
		t.Fatalf("conflicts %+v", conflicts)
	}
	same := preset.Detectors["unity-editor"]
	if conflicts := DetectorConflicts(preset, map[string]Detector{"unity-editor": same}); len(conflicts) != 0 {
		t.Fatalf("identical detector reported as a conflict: %+v", conflicts)
	}
}
