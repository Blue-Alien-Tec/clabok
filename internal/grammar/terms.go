package grammar

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// PendingTerm is one unconfirmed term-list change. Requests never touch terms.txt; only a confirmed
// commit does.
type PendingTerm struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	Term    string `json:"term"`
	Reason  string `json:"reason"`
	Created string `json:"created"`
}

// ReadPending reads the pending requests. A missing store is an empty list.
func ReadPending(path string) ([]PendingTerm, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []PendingTerm
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// AddPending records a term request and returns its pending ID. The term list stays untouched.
func AddPending(path, action, term, reason string, now time.Time) (PendingTerm, error) {
	term = strings.TrimSpace(term)
	reason = strings.TrimSpace(reason)
	if action != "add" && action != "remove" {
		return PendingTerm{}, &ServerError{Code: "terms.invalid-action",
			Message: "the action must be add or remove"}
	}
	if term == "" {
		return PendingTerm{}, &ServerError{Code: "terms.invalid-term",
			Message: "a term is required"}
	}
	if reason == "" {
		return PendingTerm{}, &ServerError{Code: "terms.reason-required",
			Message: "a reason is required for a term request"}
	}
	entries, err := ReadPending(path)
	if err != nil {
		return PendingTerm{}, err
	}
	entry := PendingTerm{
		ID:      pendingID(action, term, reason, now),
		Action:  action,
		Term:    term,
		Reason:  reason,
		Created: now.UTC().Format(time.RFC3339),
	}
	entries = append(entries, entry)
	if err := writePending(path, entries); err != nil {
		return PendingTerm{}, err
	}
	return entry, nil
}

// CommitPending applies one confirmed request and removes it. The term list is written first, so a
// crash leaves the request pending (a retry is idempotent) instead of losing it.
func CommitPending(pendingPath, termsPath, id string) (PendingTerm, int, error) {
	entries, err := ReadPending(pendingPath)
	if err != nil {
		return PendingTerm{}, 0, err
	}
	index := -1
	for position, entry := range entries {
		if entry.ID == id {
			index = position
			break
		}
	}
	if index < 0 {
		return PendingTerm{}, 0, &ServerError{Code: "terms.unknown-request",
			Message: "no pending request with id " + id}
	}
	entry := entries[index]
	terms := ReadTerms(termsPath)
	switch entry.Action {
	case "add":
		terms = addTerm(terms, entry.Term)
	case "remove":
		terms = removeTerm(terms, entry.Term)
	default:
		return PendingTerm{}, 0, &ServerError{Code: "terms.invalid-action",
			Message: "unknown pending action " + entry.Action}
	}
	if err := WriteTerms(termsPath, terms); err != nil {
		return PendingTerm{}, 0, err
	}
	entries = append(entries[:index], entries[index+1:]...)
	if err := writePending(pendingPath, entries); err != nil {
		return PendingTerm{}, 0, err
	}
	return entry, len(ReadTerms(termsPath)), nil
}

// DiscardPending removes one request without touching the term list.
func DiscardPending(pendingPath, id string) (PendingTerm, error) {
	entries, err := ReadPending(pendingPath)
	if err != nil {
		return PendingTerm{}, err
	}
	index := -1
	for position, entry := range entries {
		if entry.ID == id {
			index = position
			break
		}
	}
	if index < 0 {
		return PendingTerm{}, &ServerError{Code: "terms.unknown-request",
			Message: "no pending request with id " + id}
	}
	entry := entries[index]
	entries = append(entries[:index], entries[index+1:]...)
	if err := writePending(pendingPath, entries); err != nil {
		return PendingTerm{}, err
	}
	return entry, nil
}

// WriteTerms writes the term list: one term per line, case-insensitive dedup, first form kept.
func WriteTerms(path string, terms []string) error {
	seen := map[string]bool{}
	lines := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key := strings.ToLower(term)
		if seen[key] {
			continue
		}
		seen[key] = true
		lines = append(lines, term)
	}
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	return store.WriteAtomic(path, []byte(content))
}

func writePending(path string, entries []PendingTerm) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return store.WriteAtomic(path, append(data, '\n'))
}

func addTerm(terms []string, term string) []string {
	for _, existing := range terms {
		if strings.EqualFold(existing, term) {
			return terms
		}
	}
	return append(terms, term)
}

func removeTerm(terms []string, term string) []string {
	result := make([]string, 0, len(terms))
	for _, existing := range terms {
		if strings.EqualFold(existing, term) {
			continue
		}
		result = append(result, existing)
	}
	return result
}

func pendingID(action, term, reason string, now time.Time) string {
	sum := sha256.Sum256([]byte(action + "\x00" + term + "\x00" + reason + "\x00" +
		now.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(sum[:4])
}
