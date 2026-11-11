package config

import (
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/badge"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

func TestDetectorConfiguration(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	detector := `{"kind":"file","path":"go.mod","format":"regex",` +
		`"pattern":"^go (\\S+)","label":"go"}`
	code, _, stderr := runCommand(t, "config", "set", "badges.detectors.goversion", detector)
	if code != cli.ExitOK {
		t.Fatalf("set exit %d: %s", code, stderr)
	}
	code, stdout, stderr := runCommand(t, "config", "get", "badges.detectors.goversion")
	if code != cli.ExitOK || !strings.Contains(stdout, "goversion") {
		t.Fatalf("get exit %d: %s", code, stderr)
	}
	values, loadErr := Load(testEnv())
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(values.BadgeDetectors) != 1 || values.BadgeDetectors["goversion"].Kind != "file" {
		t.Fatalf("detectors %+v", values.BadgeDetectors)
	}

	if code, _, stderr := runCommand(t, "config", "set", "badges.detectors.license", detector); code == cli.ExitOK {
		t.Fatalf("a built-in id must be reserved: %s", stderr)
	}
	if code, _, stderr := runCommand(t, "config", "set", "badges.detectors.node",
		`{"kind":"nonsense"}`); code == cli.ExitOK {
		t.Fatalf("an unknown kind must be refused: %s", stderr)
	}
	if code, _, stderr := runCommand(t, "config", "set", "badges.detectors.node",
		`{"kind":"static","value":"x","unknown":1}`); code == cli.ExitOK {
		t.Fatalf("an unknown field must be refused: %s", stderr)
	}

	if code, _, stderr := runCommand(t, "config", "unset", "badges.detectors.goversion"); code != cli.ExitOK {
		t.Fatalf("unset exit %d: %s", code, stderr)
	}
	values, loadErr = Load(testEnv())
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(values.BadgeDetectors) != 0 {
		t.Fatalf("detectors after unset %+v", values.BadgeDetectors)
	}
}

func TestDetectorValidation(t *testing.T) {
	if err := badge.ValidateDetector("x", badge.Detector{Kind: badge.KindStatic, Value: "v"}); err != nil {
		t.Fatal(err)
	}
	issues := validate(&file{Badges: &badgesSection{Detectors: map[string]badge.Detector{
		"license": {Kind: badge.KindStatic, Value: "x"},
	}}})
	if len(issues) != 1 || issues[0].Code != "config.invalid-badge" {
		t.Fatalf("issues %+v", issues)
	}
}
