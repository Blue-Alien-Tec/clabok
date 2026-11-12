package grammar

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinTermsSuppressToolName(t *testing.T) {
	server := reportServer(t, func(prepared string) []map[string]any {
		matches := make([]map[string]any, 0, 2)
		if at := strings.Index(prepared, "pecia"); at >= 0 {
			matches = append(matches, matchAt("MORFOLOGIK_RULE_EN_US", "special",
				unitsBefore(prepared, at), 5))
		}
		if at := strings.Index(prepared, "CLABOK"); at >= 0 {
			matches = append(matches, matchAt("MORFOLOGIK_RULE_EN_US", "CLABOK",
				unitsBefore(prepared, at), 6))
		}
		return matches
	})
	defer server.Close()

	client := NewClient(server.URL)
	client.Terms = LoadTermSet(filepath.Join(t.TempDir(), "terms.txt"), t.TempDir(), DefaultLanguage)
	result, err := client.Check(context.Background(),
		"pecia writes a CLABOK book.", DefaultLanguage)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("the tool's own names must not be findings: %+v", result.Findings)
	}
	if len(result.Suppressed) != 2 {
		t.Fatalf("suppression %+v", result.Suppressed)
	}
	for _, entry := range result.Suppressed {
		if entry.Layer != ScopeBuiltin {
			t.Fatalf("suppressed by %q, want %q", entry.Layer, ScopeBuiltin)
		}
	}
}

func TestBuiltinTermKeepsTheStoreTermAsAlsoIn(t *testing.T) {
	directory := t.TempDir()
	termsPath := filepath.Join(directory, "terms.txt")
	if err := os.WriteFile(termsPath, []byte("pecia\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	set := LoadTermSet(termsPath, filepath.Join(directory, "terms"), DefaultLanguage)
	entries := set.Suppress("pecia", []string{})
	if len(entries) != 1 {
		t.Fatalf("suppression %+v", entries)
	}
	if entries[0].Layer != ScopeBuiltin {
		t.Fatalf("the built-in layer must win: %+v", entries[0])
	}
	if len(entries[0].AlsoIn) != 1 || entries[0].AlsoIn[0] != ScopeGlobal {
		t.Fatalf("the store term must stay visible: %+v", entries[0])
	}
}

func TestIsBuiltinTerm(t *testing.T) {
	for _, term := range []string{"pecia", "Pecia", "CLABOK", " clabok "} {
		if !IsBuiltinTerm(term) {
			t.Fatalf("%q must be built in", term)
		}
	}
	for _, term := range []string{"", "Simia", "CLABOOK"} {
		if IsBuiltinTerm(term) {
			t.Fatalf("%q must not be built in", term)
		}
	}
}
