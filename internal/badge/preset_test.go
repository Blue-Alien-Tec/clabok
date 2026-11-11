package badge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnityPreset(t *testing.T) {
	preset, ok := Preset("unity")
	if !ok || preset.Name != "unity" {
		t.Fatal("unity preset is missing")
	}
	if len(preset.Detectors) != 3 {
		t.Fatalf("detectors %+v", preset.Detectors)
	}
	ids := map[string]bool{}
	for _, declared := range preset.Badges {
		ids[declared.ID] = true
	}
	if ids["go"] {
		t.Fatal("the unity preset must not carry a Go badge")
	}
	for _, wanted := range []string{"unity-editor", "unity-backend", "csharp-lang", "license",
		"remote"} {
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
