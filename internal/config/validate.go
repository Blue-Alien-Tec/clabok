package config

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Issue is one configuration problem.
type Issue struct {
	Code    string `json:"code"`
	Key     string `json:"key,omitempty"`
	Message string `json:"message"`
}

// issuesPayload carries the aggregated problems as an error detail.
type issuesPayload struct {
	Issues []Issue `json:"issues"`
}

// Text renders the issues for --text output.
func (payload issuesPayload) Text() string {
	var builder strings.Builder
	for _, issue := range payload.Issues {
		fmt.Fprintf(&builder, "%s: %s\n", issue.Key, issue.Message)
	}
	return builder.String()
}

func validate(document *file) []Issue {
	var issues []Issue
	if document.Store != nil && document.Store.Root != nil {
		issues = appendRootIssue(issues, "store.root", *document.Store.Root)
	}
	if document.Temp != nil && document.Temp.Root != nil {
		issues = appendRootIssue(issues, "temp.root", *document.Temp.Root)
	}
	if document.Docs != nil && document.Docs.Root != nil {
		issues = appendRootIssue(issues, "docs.root", *document.Docs.Root)
	}
	if document.Rollback != nil {
		if document.Rollback.UndoLimit != nil && *document.Rollback.UndoLimit < 0 {
			issues = append(issues, Issue{
				Code:    "config.invalid-limit",
				Key:     "rollback.undoLimit",
				Message: "must be zero or greater",
			})
		}
		if document.Rollback.BookPointLimit != nil && *document.Rollback.BookPointLimit < 0 {
			issues = append(issues, Issue{
				Code:    "config.invalid-limit",
				Key:     "rollback.bookPointLimit",
				Message: "must be zero or greater",
			})
		}
	}
	if document.Concurrency != nil && document.Concurrency.Mode != nil {
		mode := *document.Concurrency.Mode
		if mode != "file" && mode != "daemon" {
			issues = append(issues, Issue{
				Code:    "config.invalid-mode",
				Key:     "concurrency.mode",
				Message: "expected file or daemon, got " + mode,
			})
		}
	}
	if document.Grammar != nil && document.Grammar.Endpoint != nil && *document.Grammar.Endpoint != "" {
		endpoint := *document.Grammar.Endpoint
		parsed, err := url.Parse(endpoint)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			issues = append(issues, Issue{
				Code:    "config.invalid-endpoint",
				Key:     "grammar.endpoint",
				Message: "expected an http(s) URL, got " + endpoint,
			})
		}
	}
	if document.Grammar != nil && document.Grammar.Policy != nil {
		policy := *document.Grammar.Policy
		if policy != "block" && policy != "warn" {
			issues = append(issues, Issue{
				Code:    "config.invalid-policy",
				Key:     "grammar.policy",
				Message: "expected block or warn, got " + policy,
			})
		}
	}
	if document.Grammar != nil && document.Grammar.Timeout != nil {
		timeout := *document.Grammar.Timeout
		if duration, err := time.ParseDuration(strings.TrimSpace(timeout)); err != nil || duration <= 0 {
			issues = append(issues, Issue{
				Code:    "config.invalid-timeout",
				Key:     "grammar.timeout",
				Message: "expected a positive duration such as 5s, got " + timeout,
			})
		}
	}
	if document.Asciiui != nil {
		names := make([]string, 0, len(document.Asciiui.Types))
		for name := range document.Asciiui.Types {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if strings.TrimSpace(name) == "" {
				issues = append(issues, Issue{
					Code:    "config.invalid-key",
					Key:     "asciiui.types",
					Message: "a type name must not be empty",
				})
			}
		}
	}
	return issues
}

func appendRootIssue(issues []Issue, key, value string) []Issue {
	trimmed := strings.TrimSpace(value)
	switch trimmed {
	case "":
		return append(issues, Issue{Code: "config.invalid-path", Key: key, Message: "must not be empty"})
	case ".", "..":
		return append(issues, Issue{
			Code:    "config.invalid-path",
			Key:     key,
			Message: "must name a subdirectory, not " + trimmed,
		})
	}
	return issues
}

func checkRoot(key, value string) *Issue {
	issues := appendRootIssue(nil, key, value)
	if len(issues) == 0 {
		return nil
	}
	return &issues[0]
}
