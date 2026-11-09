package search

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden files")

func checkGolden(t *testing.T, name string, data []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v (run with -update)", name, err)
	}
	if !bytes.Equal(data, want) {
		t.Fatalf("%s differs:\n%s", name, data)
	}
}

func goldenPayload(t *testing.T, payload any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func TestGoldenPayloads(t *testing.T) {
	scene := newScene(t)
	single := findBook(t, scene, "fixture", "Hello", Options{IndexMode: "off"})
	checkGolden(t, "matches_single.json", goldenPayload(t, single.Payload(0, 0)))

	multi, err := scene.Service.Find(Options{
		Book: "fixture", IndexMode: "off", Join: "or",
		Queries: []Query{{Text: "Hello"}, {Text: "wrold", Mode: ModeFuzzy, Fuzzy: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "matches_multi_fuzzy.json", goldenPayload(t, multi.Payload(0, 0)))

	count := findBook(t, scene, "fixture", "Hello", Options{Result: "count", IndexMode: "off"})
	checkGolden(t, "count.json", goldenPayload(t, count.Payload(0, 0)))

	ndjson, err := single.NDJSON(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "ndjson_single.ndjson", ndjson)
}
