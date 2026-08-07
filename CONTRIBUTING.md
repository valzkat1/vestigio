# Contributing

Contributions are welcome. This document is what the review will hold you to, so it is worth the four minutes.

## The bar

Everything reaches `main` through a pull request, from a fork or a branch, with CI green. There are no exceptions and the maintainer does not have one either — the branch rules apply to the owner too. A rule with an escape hatch for the person most likely to be in a hurry is not a rule.

Before you open a pull request:

```bash
gofmt -l .        # must print nothing
go vet ./...
go test ./...
go mod tidy       # must leave go.mod and go.sum unchanged
```

CI runs all of that plus the race detector, `govulncheck`, and CodeQL, across Linux, macOS and Windows. Running it locally first is just faster than finding out from a red check.

## What gets a change accepted

**Three tools, and that number is the product.** `recall`, `remember`, `forget`. A pull request that adds a fourth MCP tool needs to argue why an agent must *call* it, not why the capability is useful. Every tool exposed over MCP is schema text injected into every session of every agent, whether it is ever used or not — that is the cost this project exists to attack. Useful capability that a human triggers belongs in the CLI, where it costs nothing.

**The token budget is enforced in the build.** `TestToolSchemaFitsBudget` fails if the tool schema grows past its ceiling. If your change trips it, the fix is a shorter description, not a larger ceiling. Prose in descriptions was 51% of the cost in the tool this project was measured against.

**Numbers, not adjectives.** "Faster" and "smaller" are claims. Show the measurement and how you took it. Every performance statement in the README came off the wire from a real binary, and it says so where the figures are estimates.

**Fail loudly.** An argument the parser does not understand must stop the command, never fall through into something that happens to accept it. An empty result must be distinguishable from a wrong lookup. This is not style — the two worst bugs found so far were both silent: a `forget` that deleted by `OR` when it should have used `AND`, and a flag parser that read `--map old=new` as an export path. Neither one errored. Both destroyed something.

**Reads are generous, deletes are strict.** Search that feeds a destructive operation does not get to reuse search that feeds a read. `SanitizeFTS` unions terms, `SanitizeFTSAll` intersects them. Keep the asymmetry.

## Tests

New behaviour comes with a test. A bug fix comes with the test that would have caught it — pin the rule that broke, not just the input that revealed it.

Table-driven tests are the house style. Test names say what is guaranteed, not which function is being called.

## Commits and pull requests

Conventional commits: `fix(import): reject unrecognised arguments instead of skipping them`. The subject says what changed; the body says what was wrong before and why this is the fix. Assume the reader is you, in a year, staring at a `git blame`.

Do not add AI co-authorship trailers to commits.

The pull request template asks what changed, what you ran with its real output, and what the change does *not* prove. That last field is not modesty, it is the useful one — see [`.rdd/ledger.md`](.rdd/ledger.md) for how it is used here. "I did not test the import path end to end" is a complete and welcome answer. Silence about it is not.

## Dependencies

The dependency list is short on purpose and the bar for adding to it is high. `modernc.org/sqlite` is pure Go, which is the entire reason the binary is static, CGO-free and cross-compiles anywhere. A dependency that reintroduces cgo will be declined regardless of what else it does.

Actions in CI are pinned to commit SHAs, with the tag in a trailing comment so Dependabot can move them. New workflow steps follow that.

## Reporting bugs

Include `vestigio version`, your OS, and the exact commands. If it involves recall, say what you searched for and what you expected to find — this project has an open question about paraphrase retrieval, and a real miss is more valuable than a bug report.

Security issues do not go in the issue tracker. See [SECURITY.md](SECURITY.md).
