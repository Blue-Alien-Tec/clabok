// Package textinput decodes user-supplied text files and stdin. A leading byte-order mark is
// transport metadata, not content: it is stripped for UTF-8 and used to transcode UTF-16 and UTF-32.
// The decoder runs at the CLI boundary; the stored model never receives a byte-order mark.
package textinput

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// CodeInvalid is the stable error code of an undecodable input.
const CodeInvalid = "text.invalid-encoding"

// Error reports why an input cannot be decoded as text.
type Error struct {
	Code    string
	Message string
	Offset  int
}

func (err *Error) Error() string     { return err.Code + ": " + err.Message }
func (err *Error) ErrorCode() string { return err.Code }

var (
	utf8BOM = []byte{0xEF, 0xBB, 0xBF}
	utf16LE = []byte{0xFF, 0xFE}
	utf16BE = []byte{0xFE, 0xFF}
	utf32LE = []byte{0xFF, 0xFE, 0x00, 0x00}
	utf32BE = []byte{0x00, 0x00, 0xFE, 0xFF}
)

// Decode normalizes a text input to UTF-8 without a leading BOM.
func Decode(data []byte) ([]byte, error) {
	switch {
	case bytes.HasPrefix(data, utf32LE):
		return decodeUTF32(data[4:], binary.LittleEndian)
	case bytes.HasPrefix(data, utf32BE):
		return decodeUTF32(data[4:], binary.BigEndian)
	case bytes.HasPrefix(data, utf16LE):
		return decodeUTF16(data[2:], binary.LittleEndian)
	case bytes.HasPrefix(data, utf16BE):
		return decodeUTF16(data[2:], binary.BigEndian)
	case bytes.HasPrefix(data, utf8BOM):
		return normalizeUTF8(data[3:])
	}
	return normalizeUTF8(data)
}

// ReadFile reads and decodes a file. I/O errors pass through unchanged.
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decode(data)
}

// ReadStdin reads and decodes standard input. I/O errors pass through unchanged.
func ReadStdin(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return Decode(data)
}

// normalizeUTF8 validates UTF-8 and refuses NUL bytes, which indicate UTF-16 without a BOM or a
// binary file.
func normalizeUTF8(data []byte) ([]byte, error) {
	for offset := 0; offset < len(data); {
		if data[offset] == 0 {
			return nil, &Error{
				Code: CodeInvalid, Offset: offset,
				Message: fmt.Sprintf(
					"the input contains a NUL byte at byte %d; it is probably UTF-16 without a "+
						"byte-order mark, so save it as UTF-8", offset),
			}
		}
		runeValue, size := utf8.DecodeRune(data[offset:])
		if runeValue == utf8.RuneError && size == 1 {
			return nil, &Error{
				Code: CodeInvalid, Offset: offset,
				Message: fmt.Sprintf("the input is not valid UTF-8 at byte %d; save the file as "+
					"UTF-8", offset),
			}
		}
		offset += size
	}
	return data, nil
}

// decodeUTF16 transcodes UTF-16 code units, rejecting unpaired surrogates.
func decodeUTF16(data []byte, order binary.ByteOrder) ([]byte, error) {
	if len(data)%2 != 0 {
		return nil, &Error{Code: CodeInvalid,
			Message: "the UTF-16 input has an odd byte count"}
	}
	var builder strings.Builder
	for offset := 0; offset < len(data); offset += 2 {
		unit := order.Uint16(data[offset:])
		switch {
		case unit >= 0xD800 && unit <= 0xDBFF:
			if offset+3 >= len(data) {
				return nil, invalidAt(offset, "unpaired high surrogate")
			}
			next := order.Uint16(data[offset+2:])
			if next < 0xDC00 || next > 0xDFFF {
				return nil, invalidAt(offset, "unpaired high surrogate")
			}
			builder.WriteRune(utf16.DecodeRune(rune(unit), rune(next)))
			offset += 2
		case unit >= 0xDC00 && unit <= 0xDFFF:
			return nil, invalidAt(offset, "unpaired low surrogate")
		default:
			builder.WriteRune(rune(unit))
		}
	}
	return []byte(builder.String()), nil
}

// decodeUTF32 transcodes UTF-32 code points, rejecting surrogates and values above the rune range.
func decodeUTF32(data []byte, order binary.ByteOrder) ([]byte, error) {
	if len(data)%4 != 0 {
		return nil, &Error{Code: CodeInvalid,
			Message: "the UTF-32 input has a byte count that is not a multiple of four"}
	}
	var builder strings.Builder
	for offset := 0; offset < len(data); offset += 4 {
		value := order.Uint32(data[offset:])
		if value > utf8.MaxRune || (value >= 0xD800 && value <= 0xDFFF) {
			return nil, invalidAt(offset, fmt.Sprintf("invalid code point U+%04X", value))
		}
		builder.WriteRune(rune(value))
	}
	return []byte(builder.String()), nil
}

func invalidAt(offset int, message string) *Error {
	return &Error{Code: CodeInvalid, Offset: offset,
		Message: fmt.Sprintf("%s at byte %d", message, offset)}
}
