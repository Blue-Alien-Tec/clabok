package editorial

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBadgePresetFlow(t *testing.T) {
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "ProjectSettings"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "ProjectSettings", "ProjectVersion.txt"),
		[]byte("m_EditorVersion: 6000.1.0f1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "open", "library")

	detected := runOK(t, directory, "badge", "detect")
	if !strings.Contains(detected, "unity") {
		t.Fatalf("detect %s", detected)
	}

	runOK(t, directory, "badge", "init", "--preset", "unity", "--book", "library")

	shown := runOK(t, directory, "badge", "show", "--book", "library")
	if !strings.Contains(shown, "unity-editor") || !strings.Contains(shown, "6000.1.0f1") {
		t.Fatalf("show %s", shown)
	}
	if strings.Contains(shown, `"id": "go"`) {
		t.Fatalf("the unity preset must not declare a Go badge: %s", shown)
	}

	explained := runOK(t, directory, "badge", "explain", "--book", "library", "--id", "unity-editor")
	if !strings.Contains(explained, `"hash": "value"`) ||
		!strings.Contains(explained, `"kind": "file"`) {
		t.Fatalf("explain %s", explained)
	}

	checked := runOK(t, directory, "badge", "check", "--book", "library")
	if !strings.Contains(checked, "checked") || strings.Contains(checked, "unresolved:") {
		t.Fatalf("check %s", checked)
	}

	// A second init with a changed detector is refused without --force.
	runOK(t, directory, "config", "set", "badges.detectors.unity-editor",
		`{"kind":"static","value":"other"}`)
	code, _, stderr := run(t, directory, "badge", "init", "--preset", "unity", "--book", "library")
	if code == 0 || !strings.Contains(stderr, "badge.detector-exists") {
		t.Fatalf("conflict exit %d: %s", code, stderr)
	}
	runOK(t, directory, "badge", "init", "--preset", "unity", "--book", "library", "--force")
}
