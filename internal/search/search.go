// Package search finds text in manifests and bodies. Reads take no lock: the store writes files
// atomically, so a search always sees complete bodies.
package search

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// Error is a search failure with a stable code.
type Error struct {
	Code    string
	Message string
}

// Error returns the human message.
func (err *Error) Error() string { return err.Message }

// ErrorCode returns the stable code for the CLI boundary.
func (err *Error) ErrorCode() string { return err.Code }

func fail(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Options controls one search.
type Options struct {
	Fields     []string
	IgnoreCase bool
	Word       bool
	Context    int
}

// Match is one matching line or title.
type Match struct {
	Kind        string   `json:"kind"`
	ID          string   `json:"id"`
	Field       string   `json:"field"`
	Line        int      `json:"line"`
	Text        string   `json:"text"`
	Context     []string `json:"context,omitempty"`
	Occurrences int      `json:"occurrences"`
}

// Result is the complete search result.
type Result struct {
	Matches     []Match `json:"matches"`
	Count       int     `json:"count"`
	Occurrences int     `json:"occurrences"`
}

// Service searches one open book.
type Service struct {
	Lifecycle *lifecycle.Lifecycle
}

// New returns a search service.
func New(life *lifecycle.Lifecycle) *Service {
	return &Service{Lifecycle: life}
}

// Find searches the requested fields of one open book.
func (service *Service) Find(book model.BookID, query string, options Options) (Result, error) {
	if strings.TrimSpace(query) == "" {
		return Result{}, fail("search.empty-query", "a query is required")
	}
	paths, err := service.Lifecycle.RequireOpen(book)
	if err != nil {
		return Result{}, err
	}
	manifest, err := loadManifest(paths.Dir)
	if err != nil {
		return Result{}, err
	}
	fields := options.Fields
	if len(fields) == 0 {
		fields = []string{"body", "title"}
	}
	wantBody, wantTitle := false, false
	for _, field := range fields {
		switch field {
		case "body":
			wantBody = true
		case "title":
			wantTitle = true
		default:
			return Result{}, fail("search.unknown-field",
				"unknown field %q; use body or title", field)
		}
	}

	result := Result{Matches: make([]Match, 0)}
	manifest.Walk(func(node *model.Node, depth int, parent *model.Node) bool {
		if wantTitle {
			if count := countOccurrences(node.Title, query, options.IgnoreCase, options.Word); count > 0 {
				result.Matches = append(result.Matches, Match{
					Kind: string(node.Kind), ID: node.ID.String(), Field: "title",
					Line: 1, Text: node.Title, Occurrences: count,
				})
				result.Occurrences += count
			}
		}
		if !wantBody {
			return true
		}
		for _, id := range node.Blocks {
			data, readErr := os.ReadFile(filepath.Join(paths.BlocksDir, id.String()+".md"))
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			if readErr != nil {
				continue
			}
			lines := splitLines(string(data))
			for index, line := range lines {
				count := countOccurrences(line, query, options.IgnoreCase, options.Word)
				if count == 0 {
					continue
				}
				result.Matches = append(result.Matches, Match{
					Kind: "block", ID: id.String(), Field: "body", Line: index + 1,
					Text: line, Context: contextLines(lines, index, options.Context),
					Occurrences: count,
				})
				result.Occurrences += count
			}
		}
		return true
	})
	result.Count = len(result.Matches)
	return result, nil
}

func loadManifest(directory string) (*model.Manifest, error) {
	data, err := os.ReadFile(filepath.Join(directory, lifecycle.ManifestMember))
	if err != nil {
		return nil, fail("search.read-failed", "%v", err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func splitLines(text string) []string {
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func contextLines(lines []string, index, radius int) []string {
	if radius <= 0 {
		return nil
	}
	start := index - radius
	if start < 0 {
		start = 0
	}
	end := index + radius + 1
	if end > len(lines) {
		end = len(lines)
	}
	context := make([]string, 0, end-start)
	context = append(context, lines[start:end]...)
	return context
}

func countOccurrences(text, query string, ignoreCase, word bool) int {
	if query == "" {
		return 0
	}
	if ignoreCase {
		text = strings.ToLower(text)
		query = strings.ToLower(query)
	}
	count := 0
	for index := 0; index+len(query) <= len(text); {
		end := index + len(query)
		if text[index:end] == query && (!word || wordBoundary(text, index, end)) {
			count++
			index = end
			continue
		}
		_, size := utf8.DecodeRuneInString(text[index:])
		if size == 0 {
			size = 1
		}
		index += size
	}
	return count
}

func wordBoundary(text string, start, end int) bool {
	if start > 0 {
		previous, _ := utf8.DecodeLastRuneInString(text[:start])
		if unicode.IsLetter(previous) || unicode.IsDigit(previous) || previous == '_' {
			return false
		}
	}
	if end < len(text) {
		next, _ := utf8.DecodeRuneInString(text[end:])
		if unicode.IsLetter(next) || unicode.IsDigit(next) || next == '_' {
			return false
		}
	}
	return true
}
