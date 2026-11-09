package migrate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func run(t *testing.T, directory string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(directory)
	registry := cli.NewRegistry()
	config.Register(registry)
	coordinator.Register(registry)
	lifecycle.Register(registry)
	Register(registry)
	var stdout, stderr strings.Builder
	code := cli.Run(registry, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func commandArgs(scene *scene, args ...string) []string {
	return append(args, "--store-root", scene.Layout.Root, "--project", scene.Layout.Root)
}

func TestMigrateStatusCommand(t *testing.T) {
	scene := newScene(t, 2, 1)
	code, stdout, stderr := run(t, scene.Layout.Root, commandArgs(scene, "migrate", "status", "--book", "fixture")...)
	if code != cli.ExitOK {
		t.Fatalf("status exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"steps"`) || !strings.Contains(stdout, "manifest-v2-v3") {
		t.Fatalf("status payload %s", stdout)
	}
	if containerVersion(t, scene.Paths) != 2 {
		t.Fatal("status changed the container")
	}
}

func TestMigratePlanRequiresScope(t *testing.T) {
	scene := newScene(t, 2, 1)
	code, _, _ := run(t, scene.Layout.Root, commandArgs(scene, "migrate", "plan")...)
	if code != cli.ExitReject {
		t.Fatalf("plan without scope exit %d", code)
	}
	code, _, _ = run(t, scene.Layout.Root, commandArgs(scene, "migrate", "plan", "--book", "missing")...)
	if code != cli.ExitReject {
		t.Fatalf("unknown book exit %d", code)
	}
}

func TestMigrateApplyCommand(t *testing.T) {
	scene := newScene(t, 2, 1)
	code, stdout, stderr := run(t, scene.Layout.Root,
		commandArgs(scene, "migrate", "apply", "--book", "fixture", "--write")...)
	if code != cli.ExitOK {
		t.Fatalf("apply exit %d: %s", code, stderr)
	}
	if containerVersion(t, scene.Paths) != 3 {
		t.Fatal("apply did not migrate the container")
	}
	var envelope struct {
		Data Report `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("payload %s: %v", stdout, err)
	}
	report := envelope.Data
	if len(report.Books) != 1 || !report.Books[0].Changed || report.Books[0].Manifest.To != 3 {
		t.Fatalf("report %+v", report.Books)
	}
}

func TestMigrateGlobalDryRunOverridesWrite(t *testing.T) {
	scene := newScene(t, 2, 1)
	code, stdout, stderr := run(t, scene.Layout.Root,
		commandArgs(scene, "migrate", "apply", "--book", "fixture", "--write", "--dry-run")...)
	if code != cli.ExitOK {
		t.Fatalf("dry run exit %d: %s", code, stderr)
	}
	if containerVersion(t, scene.Paths) != 2 {
		t.Fatal("--dry-run wrote the container")
	}
	if !strings.Contains(stdout, `"dryRun": true`) {
		t.Fatalf("dry run payload %s", stdout)
	}
}

func TestMigrateOpenBookCommand(t *testing.T) {
	scene := newScene(t, 2, 1)
	if _, err := scene.Life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, scene.Layout.Root,
		commandArgs(scene, "migrate", "apply", "--book", "fixture", "--write")...)
	if code != cli.ExitReject || !strings.Contains(stderr, "migrate.book-open") {
		t.Fatalf("open book exit %d: %s", code, stderr)
	}
}

func TestMigrateStoreMarkerViaCLI(t *testing.T) {
	scene := newScene(t, 3, 2)
	code, _, stderr := run(t, scene.Layout.Root,
		commandArgs(scene, "migrate", "apply", "--all", "--write")...)
	if code != cli.ExitOK {
		t.Fatalf("apply all exit %d: %s", code, stderr)
	}
	version, err := store.PeekStoreVersion(scene.Layout.StoreMarkerPath())
	if err != nil || version != store.StoreVersion {
		t.Fatalf("store marker %d %v", version, err)
	}
}
