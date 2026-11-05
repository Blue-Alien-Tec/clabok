// Package jsonstrict decodes exactly one JSON document with strict structure checks:
// a BOM is tolerated, duplicate keys are rejected, trailing data is rejected and unknown fields
// can be rejected on request.
package jsonstrict

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// Kind classifies a decode problem without depending on the message text.
type Kind int

const (
	// KindSyntax is a JSON syntax or type error.
	KindSyntax Kind = iota
	// KindEmpty is an empty document.
	KindEmpty
	// KindDuplicateKey is a key that appears twice in one object.
	KindDuplicateKey
	// KindTrailingData is content after the first document.
	KindTrailingData
	// KindUnknownField is a field the target type does not declare.
	KindUnknownField
)

// Error is a strict decoding failure.
type Error struct {
	Kind    Kind
	Message string
}

// Error returns the message.
func (e *Error) Error() string { return e.Message }

// Options controls the optional checks.
type Options struct {
	DisallowUnknownFields bool
}

// Decode reads one JSON document into dst.
func Decode(data []byte, dst any, options Options) error {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(data)) == 0 {
		return &Error{Kind: KindEmpty, Message: "empty JSON document"}
	}
	if err := checkDocument(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if options.DisallowUnknownFields {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(dst); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return &Error{Kind: KindUnknownField, Message: err.Error()}
		}
		return &Error{Kind: KindSyntax, Message: err.Error()}
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return &Error{Kind: KindTrailingData, Message: "trailing data after the JSON document"}
	}
	return nil
}

// checkDocument walks the token stream once to find duplicate keys and trailing data. Token order
// is enough: in an object the decoder yields keys and values alternately, and a container token is
// always a value of its parent.
func checkDocument(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	type frame struct {
		object    bool
		expectKey bool
		keys      map[string]struct{}
	}
	var stack []frame
	completed := false

	advance := func() {
		if len(stack) == 0 {
			return
		}
		top := &stack[len(stack)-1]
		if top.object {
			top.expectKey = true
		}
	}

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return &Error{Kind: KindSyntax, Message: err.Error()}
		}
		if completed {
			return &Error{Kind: KindTrailingData, Message: "trailing data after the JSON document"}
		}
		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				advance()
				stack = append(stack, frame{object: true, expectKey: true, keys: map[string]struct{}{}})
			case '[':
				advance()
				stack = append(stack, frame{object: false})
			case '}', ']':
				if len(stack) == 0 {
					return &Error{Kind: KindSyntax, Message: "unbalanced JSON delimiter"}
				}
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					completed = true
				}
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].expectKey {
				top := &stack[len(stack)-1]
				if _, exists := top.keys[value]; exists {
					return &Error{Kind: KindDuplicateKey, Message: "duplicate key " + value}
				}
				top.keys[value] = struct{}{}
				top.expectKey = false
				continue
			}
			advance()
			if len(stack) == 0 {
				completed = true
			}
		default:
			advance()
			if len(stack) == 0 {
				completed = true
			}
		}
	}
	if len(stack) != 0 {
		return &Error{Kind: KindSyntax, Message: "unexpected end of JSON document"}
	}
	if !completed {
		return &Error{Kind: KindEmpty, Message: "empty JSON document"}
	}
	return nil
}
