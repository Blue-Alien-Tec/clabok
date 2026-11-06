package structure

import (
	"testing"
)

func TestStatusLifecycle(t *testing.T) {
	scene := newScene(t)
	if _, err := scene.service.SetStatus("fixture", 10, "final", false); err == nil {
		t.Fatal("expected structure.status-disabled")
	} else if coded, ok := err.(*Error); !ok || coded.Code != "structure.status-disabled" {
		t.Fatalf("unexpected error %v", err)
	}

	enabled, err := scene.service.SetStatusEnabled("fixture", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled.Changed || !scene.manifest(t).StatusEnabled() {
		t.Fatalf("status not enabled: %+v", enabled)
	}

	if _, err := scene.service.SetStatus("fixture", 10, "final", false); err != nil {
		t.Fatal(err)
	}
	if node := scene.manifest(t).Root.Children[0]; node.Status != "final" {
		t.Fatalf("node status %q", node.Status)
	}
	if _, err := scene.service.SetStatus("fixture", 11, "wip", false); err != nil {
		t.Fatal(err)
	}
	if meta := scene.manifest(t).Meta(11); meta.Status != "wip" {
		t.Fatalf("block status %q", meta.Status)
	}

	if _, err := scene.service.SetStatus("fixture", 10, "ghost", false); err == nil {
		t.Fatal("expected structure.unknown-status")
	} else if coded, ok := err.(*Error); !ok || coded.Code != "structure.unknown-status" {
		t.Fatalf("unexpected error %v", err)
	}

	before := scene.manifestBytes(t)
	if _, err := scene.service.SetStatus("fixture", 10, "draft", true); err != nil {
		t.Fatal(err)
	}
	if string(before) != string(scene.manifestBytes(t)) {
		t.Fatal("a dry run wrote the manifest")
	}

	if _, err := scene.service.SetStatus("fixture", 10, "", false); err != nil {
		t.Fatal(err)
	}
	if node := scene.manifest(t).Root.Children[0]; node.Status != "" {
		t.Fatalf("status not cleared: %q", node.Status)
	}

	// Disabling keeps the assignments, so switching it back on is lossless.
	if _, err := scene.service.SetStatusEnabled("fixture", false, false); err != nil {
		t.Fatal(err)
	}
	if scene.manifest(t).StatusEnabled() {
		t.Fatal("status still enabled")
	}
	if meta := scene.manifest(t).Meta(11); meta.Status != "wip" {
		t.Fatalf("disabling dropped an assignment: %q", meta.Status)
	}
}
