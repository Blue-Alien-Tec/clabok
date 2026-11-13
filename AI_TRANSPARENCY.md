# AI Transparency

How this codebase is built and what was not verified. The statements below describe the development
process, not a release: the same workflow produces every version in this repository. The tool and its
usage are documented in the [README](README.md) and the [CLABOK book](Docs/clabok/README.md).

## Who did what

| Aspect | Who |
|---|---|
| Core ideas, product direction, requirements | Human maintainer |
| Continuous agent steering | Human maintainer, ongoing rather than a one-shot prompt |
| Go source code | AI coding agent |
| Tests | AI coding agent, written and executed |
| Documentation, commits, pushes, release tags | AI coding agent |
| Line-by-line code review | None; the maintainer is a C# developer and does not read Go |
| Output verification | Human maintainer read the rendered Markdown and checked it against expectations |

## Verified and unverified

The middle column is what the maintainer actually did, not what the project claims. The right column
names what limits the risk in each case.

| Aspect | What the maintainer did | Mitigation |
|---|---|---|
| Rendered Markdown matches expectations | read and spot-checked | deterministic rendering; `pecia sync --check` fails on drift |
| Product behaviour matches the intended use cases | ran on real books | the tool produces the project's own documentation |
| Go source correctness | nothing | `go vet` and `scripts/check.ps1` (format, vet, build, tests) |
| Test validity and coverage | nothing | none; the tests share the author's blind spots |
| Security properties (CLI input handling, file I/O, dependencies) | nothing | two pinned parsers, otherwise the standard library; LanguageTool stays optional |
| Dependency audit and supply chain | nothing | pinned versions; a new dependency needs a reason |
| Commit contents and authorship | nothing | the commit form is fixed by the planning policy; authorship is not review |

## Tests are not independent evidence

The suite was written by the same agent as the implementation. Passing tests show that the agent's
expectations are self-consistent, not that the code is correct in situations the agent did not
consider. Treat a green suite as a smoke signal, not a guarantee.

## Implications

- The code is not independently audited.
- The git history lists a human author for commits that human did not read; do not read commit
  authorship as code review.
- Fixing a defect will likely need AI assistance, because the maintainer cannot debug the Go code
  unaided.
- Documentation that must be correct or secure deserves the same scrutiny you would apply to any
  unreviewed tool.

## Tools

| Tool / model | Provider | Used for |
|---|---|---|
| Kilo CLI coding agent (`ucoder`) | Kilo | Conception, planning, Go implementation, tests, documentation, git operations and releases |
| `deepseek/deepseek-flash` | DeepSeek | Model of the agent sessions recorded in this repository (2026-11-05 to 2026-11-12); earlier sessions may have used other models and are not recorded individually |

The agent and model are chosen per session; this table is a best-effort account, not an audit log.
`deepseek/deepseek-flash` is a provider alias, not a pinned snapshot; the resolved model can change
between sessions.

## Scope

This is the maintainer's own account of how the project is built and what has been verified. It has
no version scope and is not an independent audit.
