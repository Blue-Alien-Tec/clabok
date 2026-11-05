package model

import (
	"errors"
	"strconv"
)

// ID is an opaque, library-managed identifier. The zero value is the creation sentinel and is
// never valid in stored data. IDs are encoded as decimal strings in JSON so that tooling in any
// language can read them without precision loss.
type ID uint64

// ZeroID is the reserved creation sentinel. It is never a valid stored ID.
const ZeroID ID = 0

// IsZero reports whether the ID is the creation sentinel.
func (id ID) IsZero() bool { return id == ZeroID }

// String renders the decimal form.
func (id ID) String() string { return strconv.FormatUint(uint64(id), 10) }

// ParseID reads a decimal ID.
func ParseID(text string) (ID, error) {
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return ZeroID, err
	}
	return ID(value), nil
}

// MarshalJSON writes the ID as a JSON string.
func (id ID) MarshalJSON() ([]byte, error) {
	return []byte(`"` + id.String() + `"`), nil
}

// UnmarshalJSON reads the decimal string form. A JSON number is rejected: the canonical form is a
// string, and accepting both would hide a producer bug.
func (id *ID) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return errors.New("model: an id must be a decimal JSON string")
	}
	value, err := ParseID(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*id = value
	return nil
}
