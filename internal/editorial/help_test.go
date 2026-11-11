package editorial

import (
	"strings"
	"testing"
)

func TestHelpListsGroupActions(t *testing.T) {
	directory := t.TempDir()
	listed := runOK(t, directory, "help", "badge", "--text")
	if !strings.Contains(listed, "add|remove|show|detect|explain|check|init") {
		t.Fatalf("the group usage must name every action:\n%s", listed)
	}
	for _, action := range []string{"add", "remove", "show", "detect", "explain", "check", "init"} {
		if !strings.Contains(listed, "pecia badge "+action+" ") {
			t.Fatalf("the action %q is missing from the help:\n%s", action, listed)
		}
	}
}

func TestHelpResolvesAnActionTopic(t *testing.T) {
	directory := t.TempDir()
	cases := [][]string{
		{"help", "badge", "init"},
		{"help", "badge init"},
	}
	for _, args := range cases {
		topic := runOK(t, directory, append(args, "--text")...)
		if !strings.Contains(topic, "--preset") || !strings.Contains(topic, "--force") {
			t.Fatalf("%v: the action flags are missing:\n%s", args, topic)
		}
		if strings.Contains(topic, "--cache-seconds") {
			t.Fatalf("%v: the action must show only its own flags:\n%s", args, topic)
		}
	}
}

func TestHelpPrintsStatusFlags(t *testing.T) {
	directory := t.TempDir()
	set := runOK(t, directory, "help", "status", "set", "--text")
	for _, flag := range []string{"--target", "--id", "--state"} {
		if !strings.Contains(set, flag) {
			t.Fatalf("status set must list %s:\n%s", flag, set)
		}
	}
	level := runOK(t, directory, "help", "status", "level", "add", "--text")
	for _, flag := range []string{"--level-id", "--label", "--color", "--first", "--last"} {
		if !strings.Contains(level, flag) {
			t.Fatalf("status level add must list %s:\n%s", flag, level)
		}
	}
}

func TestHelpRejectsAnUnknownAction(t *testing.T) {
	directory := t.TempDir()
	code, _, stderr := run(t, directory, "help", "badge", "nonsense")
	if code == 0 || !strings.Contains(stderr, "unknown action") ||
		!strings.Contains(stderr, "badge") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestHelpResolvesAMultiWordGroupAction(t *testing.T) {
	directory := t.TempDir()
	topic := runOK(t, directory, "help", "library", "defaults", "show", "--text")
	if !strings.Contains(topic, "pecia library defaults show") {
		t.Fatalf("a multi-word group must resolve:\n%s", topic)
	}
}

func TestHelpOfAPlainCommandCarriesNoActions(t *testing.T) {
	directory := t.TempDir()
	info := runOK(t, directory, "help", "config", "get")
	if strings.Contains(info, `"actions"`) {
		t.Fatalf("a plain command must not report actions:\n%s", info)
	}
}

func TestGroupHelpAfterAFlagHelp(t *testing.T) {
	directory := t.TempDir()
	topic := runOK(t, directory, "badge", "init", "--help", "--text")
	if !strings.Contains(topic, "--preset") {
		t.Fatalf("badge init --help must describe the action:\n%s", topic)
	}
}
