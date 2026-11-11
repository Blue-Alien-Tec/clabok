package textinput

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestReadValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "value.txt")
	if err := os.WriteFile(path, []byte("\xEF\xBB\xBFhello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadValue(path, strings.NewReader(""))
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if string(got) != "hello\n" {
		t.Fatalf("file = %q", got)
	}

	got, err = ReadValue("-", strings.NewReader("\xEF\xBB\xBFstdin"))
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	if string(got) != "stdin" {
		t.Fatalf("stdin = %q", got)
	}

	if _, err := ReadValue(filepath.Join(t.TempDir(), "missing.txt"), strings.NewReader("")); err == nil {
		t.Fatal("a missing file must fail")
	}
}

func TestReadValueRejectsBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "value.bin")
	if err := os.WriteFile(path, []byte{0x00, 0x01}, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadValue(path, strings.NewReader(""))
	if err == nil {
		t.Fatal("binary input must be refused")
	}
	var decodeErr *Error
	if !errors.As(err, &decodeErr) || decodeErr.Code != CodeInvalid {
		t.Fatalf("unexpected error %v", err)
	}
}

func encodeUTF16(text string, order binary.ByteOrder) []byte {
	units := utf16.Encode([]rune(text))
	data := make([]byte, 0, len(units)*2)
	for _, unit := range units {
		var pair [2]byte
		order.PutUint16(pair[:], unit)
		data = append(data, pair[:]...)
	}
	return data
}

func encodeUTF32(text string, order binary.ByteOrder) []byte {
	data := make([]byte, 0, len(text)*4)
	for _, runeValue := range text {
		var quad [4]byte
		order.PutUint32(quad[:], uint32(runeValue))
		data = append(data, quad[:]...)
	}
	return data
}

func TestDecodeUTF8(t *testing.T) {
	text := []byte("plain text\n")
	decoded, err := Decode(text)
	if err != nil || !bytes.Equal(decoded, text) {
		t.Fatalf("%q %v", decoded, err)
	}
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, text...)
	decoded, err = Decode(withBOM)
	if err != nil || !bytes.Equal(decoded, text) {
		t.Fatalf("BOM: %q %v", decoded, err)
	}
	decoded, err = Decode(nil)
	if err != nil || len(decoded) != 0 {
		t.Fatalf("empty: %q %v", decoded, err)
	}
}

func TestDecodeUTF16(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		bom := []byte{0xFF, 0xFE}
		if order == binary.BigEndian {
			bom = []byte{0xFE, 0xFF}
		}
		input := append(bom, encodeUTF16("Héllo €\n", order)...)
		decoded, err := Decode(input)
		if err != nil || string(decoded) != "Héllo €\n" {
			t.Fatalf("%T: %q %v", order, decoded, err)
		}
	}
}

func TestDecodeUTF32(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		bom := []byte{0xFF, 0xFE, 0x00, 0x00}
		if order == binary.BigEndian {
			bom = []byte{0x00, 0x00, 0xFE, 0xFF}
		}
		input := append(bom, encodeUTF32("Héllo €\n", order)...)
		decoded, err := Decode(input)
		if err != nil || string(decoded) != "Héllo €\n" {
			t.Fatalf("%T: %q %v", order, decoded, err)
		}
	}
}

func TestDecodeRejectsBOMlessUTF16(t *testing.T) {
	_, err := Decode([]byte{'a', 0x00, 'b', 0x00})
	code, ok := err.(*Error)
	if !ok || code.Code != CodeInvalid || code.Offset != 1 {
		t.Fatalf("err %v", err)
	}
	if !strings.Contains(code.Message, "UTF-16") {
		t.Fatalf("message %q", code.Message)
	}
}

func TestDecodeRejectsInvalidUTF8(t *testing.T) {
	_, err := Decode([]byte{'a', 0xC3, 0x28})
	code, ok := err.(*Error)
	if !ok || code.Offset != 1 || !strings.Contains(code.Message, "UTF-8") {
		t.Fatalf("err %v", err)
	}
}

func TestDecodeRejectsBadUTF16(t *testing.T) {
	if _, err := Decode([]byte{0xFF, 0xFE, 0x41}); err == nil {
		t.Fatal("an odd UTF-16 byte count must be refused")
	}
	// A lone high surrogate.
	if _, err := Decode([]byte{0xFF, 0xFE, 0x00, 0xD8}); err == nil {
		t.Fatal("an unpaired surrogate must be refused")
	}
}

func TestDecodePreservesInnerByteOrderMark(t *testing.T) {
	text := []byte("a\xEF\xBB\xBFb")
	decoded, err := Decode(text)
	if err != nil || !bytes.Equal(decoded, text) {
		t.Fatalf("%q %v", decoded, err)
	}
}

func TestReadStdin(t *testing.T) {
	decoded, err := ReadStdin(strings.NewReader("\uFEFFstdin\n"))
	if err != nil || string(decoded) != "stdin\n" {
		t.Fatalf("%q %v", decoded, err)
	}
}
