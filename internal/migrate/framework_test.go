package migrate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func bumpVersion(from, to int) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		return bytes.Replace(data,
			[]byte(`"version":`+itoa(from)), []byte(`"version":`+itoa(to)), 1), nil
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

func TestConfigMigrationFramework(t *testing.T) {
	scene := newScene(t, 3, 2)
	configPath := filepath.Join(scene.Layout.Root, "clabok.json")
	if err := os.WriteFile(configPath, []byte(`{"version":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scene.Service.ConfigPath = configPath
	scene.Service.ConfigVersion = 2
	scene.Service.ConfigSteps = []StepDef{{
		Name: "config-v1-v2", From: 1, To: 2, Apply: bumpVersion(1, 2),
	}}
	status, err := scene.Service.Status("fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	if status.Config.Version != 1 || !status.Config.Changed || len(status.Config.Steps) != 1 {
		t.Fatalf("config status %+v", status.Config)
	}
	report, err := scene.Service.Apply("", true, 0, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Config.Changed || report.Config.Version != 2 {
		t.Fatalf("config report %+v", report.Config)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"version":2`)) {
		t.Fatalf("config not migrated: %s", data)
	}
	entries, err := os.ReadDir(scene.Layout.MigrationsRoot())
	if err != nil || len(entries) != 1 {
		t.Fatalf("migrations %v %v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(scene.Layout.MigrationsRoot(), entries[0].Name(), "clabok.json")); err != nil {
		t.Fatalf("config backup missing: %v", err)
	}
}

func TestContainerMigrationFramework(t *testing.T) {
	scene := newScene(t, 3, 2)
	invoked := 0
	scene.Service.ContainerVersion = 2
	scene.Service.ContainerSteps = []StepDef{{
		Name: "container-v1-v2", From: 1, To: 2,
		Apply: func(data []byte) ([]byte, error) {
			invoked++
			return data, nil
		},
	}}
	status, err := scene.Service.Status("fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	if status.Container.From != 1 || status.Container.To != 2 || len(status.Container.Steps) != 1 {
		t.Fatalf("container status %+v", status.Container)
	}
	report, err := scene.Service.Apply("fixture", false, 0, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if invoked != 1 {
		t.Fatalf("the container step ran %d times", invoked)
	}
	if !report.Books[0].Changed {
		t.Fatalf("the book was not reported changed: %+v", report.Books[0])
	}
}

func TestOpenStateMigrationFramework(t *testing.T) {
	scene := newScene(t, 3, 2)
	openPath := scene.Layout.OpenState()
	if err := os.WriteFile(openPath, []byte(`{"version":1,"books":[]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scene.Service.OpenStateVersion = 2
	scene.Service.OpenStateSteps = []StepDef{{
		Name: "open-state-v1-v2", From: 1, To: 2, Apply: bumpVersion(1, 2),
	}}
	status, err := scene.Service.Status("fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	if status.OpenState.From != 1 || len(status.OpenState.Steps) != 1 {
		t.Fatalf("open state status %+v", status.OpenState)
	}
	report, err := scene.Service.Apply("", true, 0, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OpenState.Changed {
		t.Fatalf("open state report %+v", report.OpenState)
	}
	data, err := os.ReadFile(openPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"version":2`)) {
		t.Fatalf("open state not migrated: %s", data)
	}
}

func TestOpenStateMigrationSkipsAnOpenBook(t *testing.T) {
	scene := newScene(t, 3, 2)
	if err := os.WriteFile(scene.Layout.OpenState(), []byte(`{"version":1,"books":[]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scene.Service.OpenStateVersion = 2
	scene.Service.OpenStateSteps = []StepDef{{
		Name: "open-state-v1-v2", From: 1, To: 2, Apply: bumpVersion(1, 2),
	}}
	if _, err := scene.Life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	report, err := scene.Service.Apply("", true, 0, true, false)
	if err != nil {
		t.Fatal(err)
	}
	skipped := false
	for _, warning := range report.Warnings {
		if warning.Code == "migrate.book-open" {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("warnings %+v", report.Warnings)
	}
	data, err := os.ReadFile(scene.Layout.OpenState())
	if err != nil {
		t.Fatal(err)
	}
	version, err := rawVersion(data)
	if err != nil || version != 1 {
		t.Fatalf("open state was rewritten to version %d: %s", version, data)
	}
}

func TestCleanupRetention(t *testing.T) {
	scene := newScene(t, 3, 2)
	root := scene.Layout.MigrationsRoot()
	for _, name := range []string{
		"20260101T000000.000000000Z",
		"20260102T000000.000000000Z",
		"20260103T000000.000000000Z",
	} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := scene.Service.Cleanup(1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Kept) != 1 || len(plan.Removed) != 2 || plan.Written {
		t.Fatalf("cleanup plan %+v", plan)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 3 {
		t.Fatal("a dry run removed history entries")
	}
	applied, err := scene.Service.Cleanup(1, true)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Written || len(applied.Removed) != 2 {
		t.Fatalf("cleanup apply %+v", applied)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "20260103T000000.000000000Z" {
		t.Fatalf("kept entries %v %v", entries, err)
	}
}

func TestStatusWithoutRegistry(t *testing.T) {
	scene := newScene(t, 3, 2)
	if err := os.Remove(scene.Layout.Registry()); err != nil {
		t.Fatal(err)
	}
	scene.Service.Registry = nil
	report, err := scene.Service.Status("fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Books) != 0 {
		t.Fatalf("books %+v", report.Books)
	}
	found := false
	for _, warning := range report.Warnings {
		if warning.Code == "migrate.registry-missing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings %+v", report.Warnings)
	}
}

func TestContainerFormatPeek(t *testing.T) {
	scene := newScene(t, 3, 2)
	data, err := os.ReadFile(scene.Paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	format, err := store.PeekFormat(data)
	if err != nil || format != store.Format {
		t.Fatalf("format %d %v", format, err)
	}
}
