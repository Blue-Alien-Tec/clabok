// Package body implements body authoring: reading, writing, replacing and listing block bodies.
//
// A body belongs to a block, never to a structural node. Parallel work is the normal case: a write
// holds the book shared and only the affected block exclusively, so different blocks of the same
// book are written concurrently. Reads take no lock; the store writes atomically.
package body

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/prose"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
)

// Error is a body failure with a stable code.
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

// Issue is one prose violation.
type Issue = prose.Issue

// Slice is a body read.
type Slice struct {
	Block      model.ID
	Text       string
	From       int
	To         int
	TotalLines int
	Truncated  bool
}

// WriteResult reports a body write.
type WriteResult struct {
	Block   model.ID
	Created bool
	Lines   int
	Bytes   int
	Changed bool
}

// Pair is one replacement.
type Pair struct {
	Find    string
	Replace string
	Regex   bool
}

// ReplaceOptions controls a replacement batch.
type ReplaceOptions struct {
	Expect     int
	IgnoreCase bool
	Word       bool
}

// ReplaceResult reports a replacement batch.
type ReplaceResult struct {
	Block        model.ID
	Replacements int
	Lines        int
	Bytes        int
}

// Row is one entry of the body list.
type Row struct {
	ID     model.ID
	Title  string
	Depth  int
	Lines  int
	Bytes  int
	Type   string
	Locked bool
}

// Service authors bodies of open books.
type Service struct {
	Lifecycle *lifecycle.Lifecycle
	Structure *structure.Service
}

// New returns a body service.
func New(life *lifecycle.Lifecycle) *Service {
	return &Service{Lifecycle: life, Structure: structure.New(life, nil)}
}

type mode int

const (
	modeSet mode = iota
	modeAppend
	modePrepend
)

// Get reads a body, optionally limited to a 1-based line range.
func (service *Service) Get(book model.BookID, block model.ID, from, to int) (Slice, error) {
	if block.IsZero() {
		return Slice{}, fail("body.unknown-block", "block 0 is only valid for a write")
	}
	paths, manifest, err := service.open(book)
	if err != nil {
		return Slice{}, err
	}
	if !manifest.IsPlaced(block) {
		return Slice{}, fail("body.unknown-block", "block %s is not placed", block)
	}
	data, readErr := os.ReadFile(blockPath(paths, block))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return Slice{}, fail("body.read-failed", "%v", readErr)
	}
	text, start, end, truncated := sliceText(string(data), from, to)
	return Slice{
		Block:      block,
		Text:       text,
		From:       start,
		To:         end,
		TotalLines: lineCount(string(data)),
		Truncated:  truncated,
	}, nil
}

// List returns every placed block in structure order.
func (service *Service) List(book model.BookID) ([]Row, error) {
	paths, manifest, err := service.open(book)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0)
	manifest.Walk(func(node *model.Node, depth int, parent *model.Node) bool {
		for _, id := range node.Blocks {
			data, readErr := os.ReadFile(blockPath(paths, id))
			if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			rows = append(rows, Row{
				ID:     id,
				Title:  node.Title,
				Depth:  depth,
				Lines:  lineCount(string(data)),
				Bytes:  len(data),
				Type:   manifest.BlockType(id),
				Locked: manifest.IsLocked(id),
			})
		}
		return true
	})
	return rows, nil
}

// Set replaces a body, or appends a new block when block is zero.
func (service *Service) Set(book model.BookID, block, parent model.ID, text string, dryRun bool) (WriteResult, error) {
	return service.write(book, block, parent, text, modeSet, "body.set", dryRun)
}

// Append adds text to the end of a body, or creates a block when block is zero.
func (service *Service) Append(book model.BookID, block, parent model.ID, text string, dryRun bool) (WriteResult, error) {
	return service.write(book, block, parent, text, modeAppend, "body.append", dryRun)
}

// Prepend adds text to the start of a body, or creates a block when block is zero.
func (service *Service) Prepend(book model.BookID, block, parent model.ID, text string, dryRun bool) (WriteResult, error) {
	return service.write(book, block, parent, text, modePrepend, "body.prepend", dryRun)
}

// Replace applies a replacement batch to one body.
func (service *Service) Replace(book model.BookID, block model.ID, pairs []Pair, options ReplaceOptions, dryRun bool) (ReplaceResult, error) {
	if block.IsZero() {
		return ReplaceResult{}, fail("body.unknown-block", "replace needs a placed block")
	}
	apply := func(current string) (string, int, error) {
		next, count, err := applyPairs(current, pairs, options)
		if err != nil {
			return "", 0, err
		}
		if options.Expect >= 0 && count != options.Expect {
			return "", 0, fail("body.expectation-failed",
				"expected %d replacement(s), got %d; nothing was written", options.Expect, count)
		}
		if issues := CheckProse(next); len(issues) > 0 {
			return "", 0, headingError(issues)
		}
		return next, count, nil
	}

	if dryRun {
		paths, manifest, err := service.open(book)
		if err != nil {
			return ReplaceResult{}, err
		}
		if !manifest.IsPlaced(block) {
			return ReplaceResult{}, fail("body.unknown-block", "block %s is not placed", block)
		}
		if manifest.IsLocked(block) {
			return ReplaceResult{}, fail("body.locked", "block %s is locked; its body cannot be changed", block)
		}
		current, readErr := os.ReadFile(blockPath(paths, block))
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return ReplaceResult{}, fail("body.read-failed", "%v", readErr)
		}
		next, count, err := apply(string(current))
		if err != nil {
			return ReplaceResult{}, err
		}
		return ReplaceResult{
			Block: block, Replacements: count, Lines: lineCount(next), Bytes: len(next),
		}, nil
	}

	paths, manifest, release, err := service.lock(book, block)
	if err != nil {
		return ReplaceResult{}, err
	}
	defer release()
	if !manifest.IsPlaced(block) {
		return ReplaceResult{}, fail("body.unknown-block", "block %s is not placed", block)
	}
	if manifest.IsLocked(block) {
		return ReplaceResult{}, fail("body.locked", "block %s is locked; its body cannot be changed", block)
	}
	current, readErr := os.ReadFile(blockPath(paths, block))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return ReplaceResult{}, fail("body.read-failed", "%v", readErr)
	}
	next, count, err := apply(string(current))
	if err != nil {
		return ReplaceResult{}, err
	}
	if _, err := service.Lifecycle.SnapshotBody(book, block, "body.replace"); err != nil {
		return ReplaceResult{}, err
	}
	if err := store.WriteAtomic(blockPath(paths, block), []byte(next)); err != nil {
		return ReplaceResult{}, err
	}
	return ReplaceResult{
		Block: block, Replacements: count, Lines: lineCount(next), Bytes: len(next),
	}, nil
}

func (service *Service) write(book model.BookID, block, parent model.ID, text string, op mode, reason string, dryRun bool) (WriteResult, error) {
	normalized, err := normalize(text)
	if err != nil {
		return WriteResult{}, err
	}
	if block.IsZero() {
		if parent.IsZero() {
			return WriteResult{}, fail("body.parent-required", "block 0 needs a parent node")
		}
		result := combine("", normalized, op)
		if issues := CheckProse(result); len(issues) > 0 {
			return WriteResult{}, headingError(issues)
		}
		if dryRun {
			receipt, addErr := service.Structure.AddBlock(book, parent, true)
			if addErr != nil {
				return WriteResult{}, addErr
			}
			return WriteResult{
				Block: receipt.Target, Created: true, Changed: true,
				Lines: lineCount(result), Bytes: len(result),
			}, nil
		}
		receipt, addErr := service.Structure.AddBlock(book, parent, false)
		if addErr != nil {
			return WriteResult{}, addErr
		}
		if writeErr := service.writeNewBlock(book, receipt.Target, result); writeErr != nil {
			return WriteResult{}, writeErr
		}
		return WriteResult{
			Block: receipt.Target, Created: true, Changed: true,
			Lines: lineCount(result), Bytes: len(result),
		}, nil
	}

	if dryRun {
		paths, manifest, openErr := service.open(book)
		if openErr != nil {
			return WriteResult{}, openErr
		}
		if !manifest.IsPlaced(block) {
			return WriteResult{}, fail("body.unknown-block", "block %s is not placed", block)
		}
		if manifest.IsLocked(block) {
			return WriteResult{}, fail("body.locked", "block %s is locked; its body cannot be changed", block)
		}
		current, readErr := os.ReadFile(blockPath(paths, block))
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return WriteResult{}, fail("body.read-failed", "%v", readErr)
		}
		next := combine(string(current), normalized, op)
		if issues := CheckProse(next); len(issues) > 0 {
			return WriteResult{}, headingError(issues)
		}
		return WriteResult{
			Block: block, Changed: true, Lines: lineCount(next), Bytes: len(next),
		}, nil
	}

	paths, manifest, release, err := service.lock(book, block)
	if err != nil {
		return WriteResult{}, err
	}
	defer release()
	if !manifest.IsPlaced(block) {
		return WriteResult{}, fail("body.unknown-block", "block %s is not placed", block)
	}
	if manifest.IsLocked(block) {
		return WriteResult{}, fail("body.locked", "block %s is locked; its body cannot be changed", block)
	}
	current, readErr := os.ReadFile(blockPath(paths, block))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return WriteResult{}, fail("body.read-failed", "%v", readErr)
	}
	next := combine(string(current), normalized, op)
	if issues := CheckProse(next); len(issues) > 0 {
		return WriteResult{}, headingError(issues)
	}
	if _, err := service.Lifecycle.SnapshotBody(book, block, reason); err != nil {
		return WriteResult{}, err
	}
	if err := store.WriteAtomic(blockPath(paths, block), []byte(next)); err != nil {
		return WriteResult{}, err
	}
	return WriteResult{
		Block: block, Changed: true, Lines: lineCount(next), Bytes: len(next),
	}, nil
}

func (service *Service) writeNewBlock(book model.BookID, block model.ID, text string) error {
	paths, _, release, err := service.lock(book, block)
	if err != nil {
		return err
	}
	defer release()
	if err := store.WriteAtomic(blockPath(paths, block), []byte(text)); err != nil {
		return err
	}
	return nil
}

// lock takes the book shared and the block exclusively, in that order.
func (service *Service) lock(book model.BookID, block model.ID) (store.BookPaths, *model.Manifest, func(), error) {
	if _, err := service.Lifecycle.RequireOpen(book); err != nil {
		return store.BookPaths{}, nil, nil, err
	}
	releaseBook, err := service.Lifecycle.LockBookShared(book)
	if err != nil {
		return store.BookPaths{}, nil, nil, err
	}
	releaseBlock, err := service.Lifecycle.LockBlock(book, block)
	if err != nil {
		_ = releaseBook()
		return store.BookPaths{}, nil, nil, err
	}
	paths, manifest, err := service.open(book)
	if err != nil {
		_ = releaseBlock()
		_ = releaseBook()
		return store.BookPaths{}, nil, nil, err
	}
	return paths, manifest, func() {
		_ = releaseBlock()
		_ = releaseBook()
	}, nil
}

func (service *Service) open(book model.BookID) (store.BookPaths, *model.Manifest, error) {
	paths, err := service.Lifecycle.RequireOpen(book)
	if err != nil {
		return store.BookPaths{}, nil, err
	}
	data, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		return store.BookPaths{}, nil, fail("body.read-failed", "%v", err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		return store.BookPaths{}, nil, err
	}
	return paths, manifest, nil
}

func blockPath(paths store.BookPaths, block model.ID) string {
	return filepath.Join(paths.BlocksDir, block.String()+".md")
}

// CheckProse reports structural headings in a body. It is shared with the render acceptance.
func CheckProse(text string) []Issue {
	return prose.CheckProse(text)
}
func headingError(issues []Issue) error {
	var builder strings.Builder
	builder.WriteString("a body must not contain #, ## or ### headings; use #### or deeper for text")
	for index, issue := range issues {
		if index >= 3 {
			builder.WriteString("; ...")
			break
		}
		builder.WriteString(fmt.Sprintf("; line %d: %s", issue.Line, strings.TrimSpace(issue.Text)))
	}
	return &Error{Code: "body.forbidden-heading", Message: builder.String()}
}

func normalize(text string) (string, error) {
	if !utf8.ValidString(text) {
		return "", fail("body.invalid-encoding", "the body is not valid UTF-8")
	}
	if strings.HasPrefix(text, "\uFEFF") {
		return "", fail("body.invalid-encoding", "the body starts with a byte-order mark")
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text, nil
}

func combine(current, text string, op mode) string {
	switch op {
	case modeAppend:
		if current == "" {
			return text
		}
		if text == "" {
			return current
		}
		if strings.HasSuffix(current, "\n") {
			return current + text
		}
		return current + "\n" + text
	case modePrepend:
		if current == "" {
			return text
		}
		if text == "" {
			return current
		}
		if strings.HasSuffix(text, "\n") {
			return text + current
		}
		return text + "\n" + current
	default:
		return text
	}
}

func lineCount(text string) int {
	if text == "" {
		return 0
	}
	count := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		count++
	}
	return count
}

func sliceText(text string, from, to int) (string, int, int, bool) {
	lines := strings.Split(text, "\n")
	trailing := false
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
		trailing = true
	}
	if text == "" {
		lines = nil
		trailing = false
	}
	total := len(lines)
	start := 1
	end := total
	if from > 0 {
		start = from
	}
	if to > 0 {
		end = to
	}
	if start < 1 {
		start = 1
	}
	if start > total {
		start = total + 1
	}
	if end > total {
		end = total
	}
	if end < start {
		end = start - 1
	}
	selected := lines[start-1 : end]
	result := strings.Join(selected, "\n")
	if trailing && end == total && len(selected) > 0 {
		result += "\n"
	}
	return result, start, end, start > 1 || end < total
}

func applyPairs(text string, pairs []Pair, options ReplaceOptions) (string, int, error) {
	if len(pairs) == 0 {
		return "", 0, fail("body.no-pairs", "at least one find/replace pair is required")
	}
	hasRegex := false
	for _, pair := range pairs {
		if pair.Find == "" {
			return "", 0, fail("body.invalid-pair", "a find value must not be empty")
		}
		if pair.Regex {
			hasRegex = true
		}
	}
	if hasRegex {
		result := text
		count := 0
		for _, pair := range pairs {
			pattern := pair.Find
			if !pair.Regex {
				pattern = regexp.QuoteMeta(pair.Find)
			}
			if options.IgnoreCase {
				pattern = "(?i)" + pattern
			}
			if options.Word {
				pattern = `\b` + pattern + `\b`
			}
			expression, err := regexp.Compile(pattern)
			if err != nil {
				return "", 0, fail("body.invalid-regex", "%v", err)
			}
			count += len(expression.FindAllStringIndex(result, -1))
			result = expression.ReplaceAllString(result, pair.Replace)
		}
		return result, count, nil
	}
	return applyLiteral(text, pairs, options)
}

func applyLiteral(text string, pairs []Pair, options ReplaceOptions) (string, int, error) {
	ordered := append([]Pair(nil), pairs...)
	sort.SliceStable(ordered, func(left, right int) bool {
		return len(ordered[left].Find) > len(ordered[right].Find)
	})
	var builder strings.Builder
	builder.Grow(len(text))
	count := 0
	for index := 0; index < len(text); {
		matched := false
		for _, pair := range ordered {
			end := index + len(pair.Find)
			if end > len(text) {
				continue
			}
			if !equalText(text[index:end], pair.Find, options.IgnoreCase) {
				continue
			}
			if options.Word && !wordBoundary(text, index, end) {
				continue
			}
			builder.WriteString(pair.Replace)
			index = end
			count++
			matched = true
			break
		}
		if matched {
			continue
		}
		_, size := utf8.DecodeRuneInString(text[index:])
		if size == 0 {
			size = 1
		}
		builder.WriteString(text[index : index+size])
		index += size
	}
	return builder.String(), count, nil
}

func equalText(left, right string, ignoreCase bool) bool {
	if ignoreCase {
		return strings.EqualFold(left, right)
	}
	return left == right
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

// ParseLines parses an "A-B" range. An empty string means the whole body.
func ParseLines(value string) (int, int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, 0, nil
	}
	parts := strings.SplitN(trimmed, "-", 2)
	from, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || from < 1 {
		return 0, 0, fail("body.invalid-range", "expected A-B with A >= 1, got %q", value)
	}
	to := from
	if len(parts) == 2 {
		to, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || to < from {
			return 0, 0, fail("body.invalid-range", "expected A-B with B >= A, got %q", value)
		}
	}
	return from, to, nil
}
