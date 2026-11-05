package search

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

type payload struct {
	Book        string  `json:"book"`
	Query       string  `json:"query"`
	Count       int     `json:"count"`
	Occurrences int     `json:"occurrences"`
	Matches     []Match `json:"matches"`
}

func (payload payload) Text() string {
	var builder strings.Builder
	for _, match := range payload.Matches {
		fmt.Fprintf(&builder, "%s %d: %s\n", match.ID, match.Line, match.Text)
	}
	fmt.Fprintf(&builder, "%d match(es), %d occurrence(s)\n", payload.Count, payload.Occurrences)
	return builder.String()
}

// Register adds the find command.
func Register(registry *cli.Registry) {
	var book, query, field string
	var context int
	var ignoreCase, word, countOnly bool
	registry.Add(&cli.Command{
		Name:    "find",
		Summary: "search titles and bodies",
		Usage:   "pecia find --book <id> --query <text> [--field body,title] [--context N]",
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("find")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&query, "query", "", "text to find")
			set.StringVar(&field, "field", "body,title", "comma-separated fields")
			set.IntVar(&context, "context", 0, "context lines around a body match")
			set.BoolVar(&ignoreCase, "ignore-case", false, "match case-insensitively")
			set.BoolVar(&word, "word", false, "match whole words only")
			set.BoolVar(&countOnly, "count", false, "print counts only")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.unexpected-argument", "find takes flags only")
			}
			if book == "" {
				return nil, cli.Usage("usage.missing-argument", "--book is required")
			}
			if query == "" {
				return nil, cli.Usage("usage.missing-argument", "--query is required")
			}
			bookID := model.BookID(book)
			if !bookID.Valid() {
				return nil, cli.Usage("usage.invalid-value", "invalid book id %q", book)
			}
			fields := make([]string, 0)
			for _, part := range strings.Split(field, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					fields = append(fields, trimmed)
				}
			}
			life, _, envErr := lifecycle.FromEnv(env)
			if envErr != nil {
				return nil, envErr
			}
			service := New(life)
			result, findErr := service.Find(bookID, query, Options{
				Fields: fields, IgnoreCase: ignoreCase, Word: word, Context: context,
			})
			if findErr != nil {
				return nil, cli.AsError(findErr)
			}
			matches := result.Matches
			if countOnly {
				matches = nil
			}
			return &cli.Result{Data: payload{
				Book: string(bookID), Query: query, Count: result.Count,
				Occurrences: result.Occurrences, Matches: matches,
			}}, nil
		},
	})
}
