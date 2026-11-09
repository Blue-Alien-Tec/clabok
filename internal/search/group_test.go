package search

import (
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

func TestGroupByType(t *testing.T) {
	scene := newScene(t)
	result := findBook(t, scene, "fixture", "Hello", Options{GroupBy: "type"})
	payload := result.Payload(0, 0).(groupedPayload)
	if payload.GroupBy != "type" || payload.Count != 2 || len(payload.Groups) != 2 {
		t.Fatalf("payload %+v", payload)
	}
	if payload.Groups[0].Key != "table" || payload.Groups[0].Count != 2 || payload.Groups[0].FirstLine != 1 {
		t.Fatalf("first group %+v", payload.Groups[0])
	}
	if payload.Groups[1].Key != "unset" || payload.Groups[1].Count != 1 {
		t.Fatalf("second group %+v", payload.Groups[1])
	}
	if len(payload.Matches) != 3 {
		t.Fatalf("matches %+v", payload.Matches)
	}
}

func TestGroupByStatusKindPart(t *testing.T) {
	scene := newScene(t)
	status := findBook(t, scene, "fixture", "Hello", Options{GroupBy: "status"}).Payload(0, 0).(groupedPayload)
	if len(status.Groups) != 2 || status.Groups[0].Key != "final" || status.Groups[1].Key != "wip" {
		t.Fatalf("status groups %+v", status.Groups)
	}
	kind := findBook(t, scene, "fixture", "Hello", Options{GroupBy: "kind"}).Payload(0, 0).(groupedPayload)
	if len(kind.Groups) != 2 || kind.Groups[0].Key != "chapter" || kind.Groups[1].Key != "subchapter" {
		t.Fatalf("kind groups %+v", kind.Groups)
	}
	part := findBook(t, scene, "fixture", "Hello", Options{GroupBy: "part"}).Payload(0, 0).(groupedPayload)
	if len(part.Groups) != 1 || part.Groups[0].Key != "Guide" || part.Groups[0].Count != 3 {
		t.Fatalf("part groups %+v", part.Groups)
	}
}

func TestGroupTitleUsesNodeMetadata(t *testing.T) {
	scene := newScene(t)
	status := findBook(t, scene, "fixture", "Fixture", Options{GroupBy: "status"}).Payload(0, 0).(groupedPayload)
	if len(status.Groups) != 1 || status.Groups[0].Key != "wip" {
		t.Fatalf("title status groups %+v", status.Groups)
	}
	kind := findBook(t, scene, "fixture", "Fixture", Options{GroupBy: "kind"}).Payload(0, 0).(groupedPayload)
	if len(kind.Groups) != 1 || kind.Groups[0].Key != "title" {
		t.Fatalf("title kind groups %+v", kind.Groups)
	}
	part := findBook(t, scene, "fixture", "Fixture", Options{GroupBy: "part"}).Payload(0, 0).(groupedPayload)
	if len(part.Groups) != 1 || part.Groups[0].Key != "unset" {
		t.Fatalf("title part groups %+v", part.Groups)
	}
}

func TestCountByAndPaging(t *testing.T) {
	scene := newScene(t)
	result := findBook(t, scene, "fixture", "Hello", Options{GroupBy: "type", CountBy: true})
	payload := result.Payload(0, 0).(groupedCountPayload)
	if len(payload.Groups) != 2 || payload.Count != 2 || payload.Occurrences != 3 {
		t.Fatalf("count-by %+v", payload)
	}
	paged := findBook(t, scene, "fixture", "Hello", Options{GroupBy: "type"}).Payload(1, 0).(groupedPayload)
	if paged.Returned != 1 || !paged.Truncated || len(paged.Groups) != 1 || paged.Count != 2 {
		t.Fatalf("paged %+v", paged)
	}
}

func TestGroupPerQuery(t *testing.T) {
	scene := newScene(t)
	result, err := scene.Service.Find(Options{
		Book: "fixture", GroupBy: "type",
		Queries: []Query{{Text: "Hello"}, {Text: "world"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.Payload(0, 0).(groupedPayload)
	if len(payload.Groups[0].PerQuery) != 2 {
		t.Fatalf("perQuery %+v", payload.Groups[0])
	}
}

func TestGroupCommandValidation(t *testing.T) {
	scene := newScene(t)
	cases := []struct {
		args []string
		code int
	}{
		{[]string{"find", "--book", "fixture", "--query", "Hello", "--group-by", "status"}, cli.ExitOK},
		{[]string{"find", "--book", "fixture", "--query", "Hello", "--count-by", "type"}, cli.ExitOK},
		{[]string{"find", "--book", "fixture", "--query", "Hello", "--group-by", "status", "--count-by", "type"}, cli.ExitUsage},
		{[]string{"find", "--book", "fixture", "--query", "Hello", "--group-by", "status", "--result", "blocks"}, cli.ExitUsage},
		{[]string{"find", "--book", "fixture", "--query", "Hello", "--group-by", "bogus"}, cli.ExitUsage},
	}
	for _, test := range cases {
		code, stdout, stderr := run(t, scene.Layout.Root, commandArgs(scene, test.args...)...)
		if code != test.code {
			t.Fatalf("%v: exit %d (want %d) %s", test.args, code, test.code, stderr)
		}
		if test.code == cli.ExitOK && !strings.Contains(stdout, `"groups"`) {
			t.Fatalf("%v: payload %s", test.args, stdout)
		}
	}
}
