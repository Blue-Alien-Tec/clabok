package coordinator

import (
	"strings"
	"testing"
	"time"
)

func TestSortResources(t *testing.T) {
	ordered, err := sortResources([]Resource{
		{Kind: KindBlock, Book: "b", ID: 5},
		{Kind: KindBook, Book: "b"},
		{Kind: KindStore},
		{Kind: KindNode, Book: "a", ID: 2},
		{Kind: KindBlock, Book: "b", ID: 5},
		{Kind: KindNode, Book: "a", ID: 1},
		{Kind: KindIDs},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(ordered))
	for _, resource := range ordered {
		got = append(got, resource.String())
	}
	want := "store,book:b,node:a:1,node:a:2,block:b:5,ids"
	if strings.Join(got, ",") != want {
		t.Fatalf("order %v, want %s", got, want)
	}
}

func TestResourceValidation(t *testing.T) {
	cases := []Resource{
		{Kind: "", Book: "a"},
		{Kind: KindStore, Book: "a"},
		{Kind: KindStore, ID: 1},
		{Kind: KindBook},
		{Kind: KindBook, Book: "a", ID: 1},
		{Kind: KindNode, Book: "a"},
		{Kind: KindBlock, Book: "bad id", ID: 1},
		{Kind: KindIDs, Book: "a"},
		{Kind: KindIDs, ID: 1},
	}
	for _, resource := range cases {
		if err := resource.validate(); err == nil {
			t.Fatalf("%+v must be invalid", resource)
		}
	}
	for _, resource := range []Resource{
		{Kind: KindStore},
		{Kind: KindBook, Book: "a"},
		{Kind: KindNode, Book: "a", ID: 1},
		{Kind: KindBlock, Book: "a", ID: 1},
		{Kind: KindIDs},
	} {
		if err := resource.validate(); err != nil {
			t.Fatalf("%+v must be valid: %v", resource, err)
		}
	}
}

func TestLockNames(t *testing.T) {
	cases := []struct {
		resource Resource
		want     string
	}{
		{Resource{Kind: KindStore}, "store.lock"},
		{Resource{Kind: KindBook, Book: "clabok"}, "book-clabok.lock"},
		{Resource{Kind: KindNode, Book: "clabok", ID: 3}, "node-clabok-3.lock"},
		{Resource{Kind: KindBlock, Book: "clabok", ID: 42}, "block-clabok-42.lock"},
		{Resource{Kind: KindIDs}, "ids.lock"},
	}
	for _, testCase := range cases {
		if got := testCase.resource.lockName(); got != testCase.want {
			t.Fatalf("%+v: %q, want %q", testCase.resource, got, testCase.want)
		}
	}
}

func TestEmptyResourceSet(t *testing.T) {
	backend := newBackend(t)
	release, err := backend.Acquire(nil, Exclusive, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}

func TestNewFileRejectsRelative(t *testing.T) {
	if _, err := NewFile("relative"); err == nil {
		t.Fatal("a relative lock directory must be rejected")
	}
	if _, err := NewFile(""); err == nil {
		t.Fatal("an empty lock directory must be rejected")
	}
}

func TestInvalidResource(t *testing.T) {
	backend := newBackend(t)
	_, err := backend.Acquire([]Resource{{Kind: KindNode, Book: "clabok"}}, Exclusive, time.Second)
	if err == nil {
		t.Fatal("expected an invalid resource error")
	}
	coordinatorErr, ok := err.(*Error)
	if !ok || coordinatorErr.Code != "coordinator.invalid-resource" {
		t.Fatalf("unexpected error %v", err)
	}
}
