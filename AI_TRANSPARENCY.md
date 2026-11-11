# AI Transparency

How this codebase was built and what was not verified. This file describes the development process
of CLABOK; the tool and its usage are documented in the [README](README.md) and the
[CLABOK book](Docs/clabok/README.md).

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

| Aspect | Human-verified? |
|---|---|
| Rendered Markdown matches expectations | Yes |
| Product behaviour matches the intended use cases | Yes, through the rendered output |
| Go source correctness | No |
| Test validity and coverage | No; the same agent wrote the tests |
| Security properties (CLI input handling, file I/O, dependencies) | No |
| Dependency audit and supply chain | No |
| Commit contents and authorship | No |

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
| `deepseek/deepseek-flash` | DeepSeek | Model of the sessions that produced versions 0.5.5 to 1.1.0; earlier sessions may have used other models and are not recorded individually |

The agent and model are chosen per session; this table is a best-effort account, not an audit log.

## Scope

This disclosure covers version 1.1.0 (2026-11-11) and is based on the maintainer's own record. It is
not an independent audit.
