package grammar

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/prose"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

type applyPayload struct {
	DryRun    bool      `json:"dryRun"`
	Book      string    `json:"book"`
	Block     string    `json:"block"`
	Checked   bool      `json:"checked"`
	Reason    string    `json:"reason,omitempty"`
	Degraded  string    `json:"degraded,omitempty"`
	Applied   int       `json:"applied"`
	Changed   bool      `json:"changed"`
	Remaining []Finding `json:"remaining,omitempty"`
	Warnings  []Warning `json:"warnings,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload applyPayload) DryRunResult() bool { return payload.DryRun }

func (payload applyPayload) Text() string {
	var builder strings.Builder
	if payload.Checked {
		fmt.Fprintf(&builder, "block %s: %d finding(s) applied", payload.Block, payload.Applied)
	} else {
		fmt.Fprintf(&builder, "block %s: not checked", payload.Block)
	}
	if payload.Degraded != "" {
		fmt.Fprintf(&builder, "\ngrammar: %s", payload.Degraded)
	}
	for _, finding := range payload.Remaining {
		fmt.Fprintf(&builder, "\n  remaining %d. line %d:%d %s: %s",
			finding.Index, finding.Line, finding.Column, finding.Rule, finding.Message)
	}
	return builder.String() + "\n"
}

// selectFindings resolves the selection flags to concrete findings.
func selectFindings(findings []Finding, allSafe bool, rawIndices string) ([]Finding, error) {
	if allSafe {
		selected := make([]Finding, 0, len(findings))
		for _, finding := range findings {
			if len(finding.Replacements) == 1 {
				selected = append(selected, finding)
			}
		}
		return selected, nil
	}
	indices := map[int]bool{}
	for _, raw := range strings.Split(rawIndices, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		index, err := strconv.Atoi(raw)
		if err != nil || index <= 0 {
			return nil, &ServerError{Code: "grammar.invalid-finding",
				Message: fmt.Sprintf("invalid finding index %q", raw)}
		}
		indices[index] = true
	}
	if len(indices) == 0 {
		return nil, &ServerError{Code: "grammar.invalid-finding",
			Message: "pass --all-safe or --finding <indices>"}
	}
	selected := make([]Finding, 0, len(indices))
	for _, finding := range findings {
		if !indices[finding.Index] {
			continue
		}
		if len(finding.Replacements) == 0 {
			return nil, &ServerError{Code: "grammar.no-replacement",
				Message: fmt.Sprintf("finding %d has no replacement", finding.Index)}
		}
		selected = append(selected, finding)
		delete(indices, finding.Index)
	}
	if len(indices) > 0 {
		return nil, &ServerError{Code: "grammar.unknown-finding",
			Message: fmt.Sprintf("finding indices not in this check: %v", indices)}
	}
	return selected, nil
}

// applySelected applies the replacements right-to-left so earlier offsets stay valid.
func applySelected(text string, findings []Finding, expect int) (string, int, error) {
	ordered := append([]Finding(nil), findings...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].Offset > ordered[right].Offset })
	result := text
	for _, finding := range ordered {
		if finding.Offset < 0 || finding.Offset+finding.Length > len(result) {
			return "", 0, &ServerError{Code: "grammar.stale-finding",
				Message: fmt.Sprintf("finding %d no longer matches the body", finding.Index)}
		}
		if finding.Offending != "" &&
			result[finding.Offset:finding.Offset+finding.Length] != finding.Offending {
			return "", 0, &ServerError{Code: "grammar.stale-finding",
				Message: fmt.Sprintf("finding %d no longer matches the body", finding.Index)}
		}
		result = result[:finding.Offset] + finding.Replacements[0] +
			result[finding.Offset+finding.Length:]
	}
	if expect >= 0 && len(ordered) != expect {
		return "", 0, &ServerError{Code: "grammar.expectation-failed",
			Message: fmt.Sprintf("expected %d replacement(s), got %d; nothing was written",
				expect, len(ordered))}
	}
	return result, len(ordered), nil
}

// runApply checks the body, applies the selected findings and writes once.
func runApply(env *cli.Env, bookRaw, blockRaw, indices string, allSafe bool, expect int, jar string) (*cli.Result, error) {
	bookID, bookErr := requireBook(bookRaw)
	if bookErr != nil {
		return nil, bookErr
	}
	blockID, blockErr := requireBlock(blockRaw)
	if blockErr != nil {
		return nil, blockErr
	}
	if allSafe == (strings.TrimSpace(indices) != "") {
		return nil, cli.Usage("usage.invalid-arguments",
			"pass exactly one of --all-safe or --finding <indices>")
	}
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	life, layout, lifeErr := lifecycle.FromEnv(env)
	if lifeErr != nil {
		return nil, lifeErr
	}
	settings, settingsErr := BookSettings(life, bookID)
	if settingsErr != nil {
		return nil, cli.AsError(settingsErr)
	}
	language := Resolve(settings, values.GrammarLanguage)
	text, readErr := readBlockForWrite(env, bookID, blockID)
	if readErr != nil {
		return nil, cli.AsError(readErr)
	}
	terms := TermSource{GlobalPath: layout.Terms(), Dir: layout.TermsDir()}
	gate := NewGateTimeout(values.GrammarEndpoint, PolicyWarn, terms, values.GrammarTimeout)
	gate.Start = func(endpoint string) error {
		_, startErr := Serve(endpoint, jar)
		return startErr
	}
	payload := applyPayload{
		DryRun: env.Globals.DryRun, Book: string(bookID), Block: blockID.String(),
	}
	report := gate.Evaluate(context.Background(), blockID, text, language)
	if report.Unchecked() {
		payload.Reason = report.Reason
		payload.Degraded = report.Degraded
		payload.Warnings = report.Warnings()
		return &cli.Result{Data: payload}, nil
	}
	selected, selectErr := selectFindings(report.Findings, allSafe, indices)
	if selectErr != nil {
		return nil, cli.AsError(selectErr)
	}
	payload.Checked = true
	if len(selected) == 0 {
		return &cli.Result{Data: payload}, nil
	}
	next, applied, applyErr := applySelected(text, selected, expect)
	if applyErr != nil {
		return nil, cli.AsError(applyErr)
	}
	if issues := prose.CheckProse(next); len(issues) > 0 {
		return nil, cli.Reject("body.forbidden-heading",
			"a body must not contain # or ## headings; use ### or deeper for text").
			WithHint("the replacement would create a structural heading")
	}
	// Re-check the result so remaining findings are reported; apply itself is the repair path and
	// must not block a partial fix. The replacement text is server-driven. An unchecked re-check
	// leaves the body unchanged: a repair that cannot be verified is not written.
	recheck := gate.Evaluate(context.Background(), blockID, next, language)
	if recheck.Unchecked() {
		payload.Checked = false
		payload.Reason = recheck.Reason
		payload.Degraded = recheck.Degraded
		payload.Warnings = recheck.Warnings()
		return &cli.Result{Data: payload}, nil
	}
	payload.Applied = applied
	payload.Changed = applied > 0
	payload.Remaining = recheck.Findings
	if env.Globals.DryRun {
		return &cli.Result{Data: payload}, nil
	}
	if readErr := writeAppliedText(env, bookID, blockID, next); readErr != nil {
		return nil, cli.AsError(readErr)
	}
	return &cli.Result{Data: payload}, nil
}

// readBlockForWrite reads a placed, unlocked block of an open book.
func readBlockForWrite(env *cli.Env, book model.BookID, block model.ID) (string, error) {
	life, _, err := lifecycle.FromEnv(env)
	if err != nil {
		return "", err
	}
	paths, openErr := life.RequireOpen(book)
	if openErr != nil {
		return "", openErr
	}
	data, readErr := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if readErr != nil {
		return "", cli.Fail("grammar.read-failed", "%v", readErr)
	}
	manifest, decodeErr := model.Decode(data)
	if decodeErr != nil {
		return "", decodeErr
	}
	if !manifest.IsPlaced(block) {
		return "", &ServerError{Code: "grammar.unknown-block",
			Message: fmt.Sprintf("block %s is not placed", block)}
	}
	if manifest.IsLocked(block) {
		return "", &ServerError{Code: "grammar.locked",
			Message: fmt.Sprintf("block %s is locked", block)}
	}
	body, bodyErr := os.ReadFile(filepath.Join(paths.BlocksDir, block.String()+".md"))
	if bodyErr != nil && !os.IsNotExist(bodyErr) {
		return "", cli.Fail("grammar.read-failed", "%v", bodyErr)
	}
	return string(body), nil
}

// writeAppliedText snapshots the block and writes the repaired body once.
func writeAppliedText(env *cli.Env, book model.BookID, block model.ID, text string) error {
	life, _, err := lifecycle.FromEnv(env)
	if err != nil {
		return err
	}
	paths, openErr := life.RequireOpen(book)
	if openErr != nil {
		return openErr
	}
	releaseBook, lockErr := life.LockBookShared(book)
	if lockErr != nil {
		return lockErr
	}
	defer releaseBook()
	releaseBlock, lockErr := life.LockBlock(book, block)
	if lockErr != nil {
		return lockErr
	}
	defer releaseBlock()
	if _, snapshotErr := life.SnapshotBody(book, block, "grammar.apply"); snapshotErr != nil {
		return snapshotErr
	}
	return store.WriteAtomic(filepath.Join(paths.BlocksDir, block.String()+".md"), []byte(text))
}
