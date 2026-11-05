package jsonstrict

import (
	"errors"
	"testing"
)

type sample struct {
	Version int      `json:"version"`
	Name    string   `json:"name"`
	Nested  *nested  `json:"nested,omitempty"`
	Items   []string `json:"items,omitempty"`
}

type nested struct {
	Value int `json:"value"`
}

func kindOf(err error) Kind {
	var strict *Error
	if errors.As(err, &strict) {
		return strict.Kind
	}
	return Kind(-1)
}

func TestDecodeAcceptsBOMAndNested(t *testing.T) {
	data := []byte("\xEF\xBB\xBF{\"version\":1,\"nested\":{\"value\":2},\"items\":[\"a\",\"b\"]}")
	var value sample
	if err := Decode(data, &value, Options{DisallowUnknownFields: true}); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if value.Version != 1 || value.Nested == nil || value.Nested.Value != 2 || len(value.Items) != 2 {
		t.Fatalf("unexpected value %+v", value)
	}
}

func TestDecodeRejectsDuplicateKeys(t *testing.T) {
	cases := []string{
		`{"version":1,"version":2}`,
		`{"nested":{"value":1,"value":2}}`,
		`{"items":["a"],"nested":{"value":1},"nested":{"value":2}}`,
	}
	for _, data := range cases {
		var value sample
		err := Decode([]byte(data), &value, Options{})
		if kindOf(err) != KindDuplicateKey {
			t.Fatalf("%s: expected duplicate key, got %v", data, err)
		}
	}
}

func TestDecodeRejectsTrailingData(t *testing.T) {
	var value sample
	if err := Decode([]byte(`{"version":1} {}`), &value, Options{}); kindOf(err) != KindTrailingData {
		t.Fatalf("expected trailing data, got %v", err)
	}
	if err := Decode([]byte(`{"version":1}  `), &value, Options{}); err != nil {
		t.Fatalf("whitespace is not trailing data: %v", err)
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	var value sample
	err := Decode([]byte(`{"version":1,"bogus":true}`), &value, Options{DisallowUnknownFields: true})
	if kindOf(err) != KindUnknownField {
		t.Fatalf("expected unknown field, got %v", err)
	}
	if err := Decode([]byte(`{"version":1,"bogus":true}`), &value, Options{}); err != nil {
		t.Fatalf("unknown fields are allowed when not requested: %v", err)
	}
}

func TestDecodeRejectsEmptyAndBroken(t *testing.T) {
	var value sample
	if err := Decode(nil, &value, Options{}); kindOf(err) != KindEmpty {
		t.Fatalf("expected empty, got %v", err)
	}
	if err := Decode([]byte(`{"version":`), &value, Options{}); kindOf(err) != KindSyntax {
		t.Fatalf("expected syntax error, got %v", err)
	}
	if err := Decode([]byte(`{"nested":[]}`), &value, Options{}); kindOf(err) != KindSyntax {
		t.Fatalf("expected type error, got %v", err)
	}
}

func TestDecodeRootScalar(t *testing.T) {
	var value string
	if err := Decode([]byte(`"text"`), &value, Options{}); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if value != "text" {
		t.Fatalf("unexpected value %q", value)
	}
}
