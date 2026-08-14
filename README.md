# vestigio

[![CI](https://github.com/valzkat1/vestigio/actions/workflows/ci.yml/badge.svg)](https://github.com/valzkat1/vestigio/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/valzkat1/vestigio.svg)](https://pkg.go.dev/github.com/valzkat1/vestigio)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

> Local memory for AI coding agents. A trace of what happened — not a retelling.

Works with anything that speaks MCP: Claude Code, Cursor, Windsurf, Cline, Zed, Codex CLI.

## Why another memory server

Because the cost of agent memory is not where people look for it.

It is not in what memory returns. It is in the **tool schema**, which is injected into every session of every agent whether or not memory is ever used. That cost is invisible, it recurs forever, and almost nobody measures it.

So we measured it. Engram v1.11.0, `--tools=agent`, read straight off the wire with a JSON-RPC probe:

| | Engram (`agent`) | vestigio |
|---|---|---|
| tools exposed | 11 | **3** |
| `tools/list` payload | 8,835 chars (~2,209 tok) | budgeted **< 1,500 chars** |
| session-start dump | 7,929 chars (~1,982 tok) | none — recall is on demand |

Two of Engram's tools accounted for **48% of its whole profile**, and **51% of the cost was prose in descriptions**, not JSON Schema.

Token figures are estimates at 4 chars/token; character counts are exact.

## The three rules

**1. Three tools.** `recall`, `remember`, `forget`. Everything operational — stats, export, timeline — is a human command and lives in the CLI, where it costs zero context. A tool that an agent doesn't need to *call* should never enter its context.

**2. One round trip.** `recall` returns the content itself. No search-then-fetch dance where the first call only buys you ids.

**3. `budget_tokens` is a promise.** The caller sets a ceiling and the server respects it. Memory stops being an unpredictable source of context bloat and becomes a line item you control.

That budget is enforced in the build, not by good intentions — `TestToolSchemaFitsBudget` fails if the schema grows.

## Install

```bash
go install github.com/valzkat1/vestigio/cmd/vestigio@latest
```

Single static binary, no CGO, no runtime. SQLite is [`modernc.org/sqlite`](https://modernc.org/sqlite) — pure Go, so it cross-compiles anywhere.

## Configure

Claude Code (`.mcp.json`):

```json
{ "mcpServers": { "vestigio": { "command": "vestigio", "args": ["mcp"] } } }
```

Codex CLI (`~/.codex/config.toml`):

```toml
[mcp_servers.vestigio]
command = "vestigio"
args = ["mcp"]
```

Every other MCP client takes the same `command` + `args` shape. Codex setup, project
detection and troubleshooting in full: **[docs/codex.md](docs/codex.md)**.

| Variable | Meaning |
|---|---|
| `VESTIGIO_DB` | Database path (default `~/.vestigio/vestigio.db`) |
| `VESTIGIO_PROJECT` | Override project detection |

Projects are detected from the git remote, falling back to the directory name. The resolved name is printed to stderr on startup — silent misdetection makes an empty recall look like data loss.

## CLI

Rule 1 says the operational surface belongs to humans. This is that surface — none of it is exposed over MCP, so none of it costs an agent context.

```
vestigio projects               Inventory: memories and tokens per project
vestigio list [--all] [--kind=K] [--limit=N] [--project=NAME]
vestigio show <id>              Print one memory in full
vestigio edit <id> [--fix]      Rewrite a memory, recomputing hash and tokens
vestigio rm <id> [--yes]        Delete a memory
vestigio verify                 Report rows whose derived columns drifted
vestigio import <export.json>   Migrate an Engram export
```

**Never edit the database with a GUI browser or raw SQL.** Triggers keep the FTS index in sync, but `hash` and `tokens` are computed on write and no trigger touches them — a stale hash silently stops `remember` from deduplicating, so it inserts near-copies instead of updating. SQLite has no native `sha256()`, which makes maintaining the hash from SQL impossible; that is why `edit` exists. `verify` finds rows where it already happened, `edit --fix` repairs them.

Full reference with flags, exit codes and recipes: **[docs/cli.md](docs/cli.md)**.

## Retrieval

v1 is BM25 over SQLite FTS5 with Porter stemming.

Vectors are **not** implemented, and the schema is built for them anyway: `embedding` and `embedding_model` columns sit empty, bodies are stored whole so they can be backfilled without re-collecting anything, and ranking already runs through a `Scorer` interface with a Reciprocal Rank Fusion step — currently fusing exactly one list.

That is deliberate. Turning on embeddings later means registering a second scorer and running a backfill, not migrating a schema. Meanwhile the binary stays static and CGO-free, which is why Go was chosen at all.

Whether vectors are needed is a question for the M2 evaluation set, not for taste. A controlled M0 test already showed FTS5 missing a pure paraphrase — though part of that failure was lexical (`Go` vs `Golang`), which stemming and synonym expansion close without a model.

## Status

M1 — skeleton, verified against the built binary. Working MCP server, storage, BM25 recall, exact-content dedupe. 10/10 tests green; `tools/list` measured at 1,077 bytes on the wire, 8.2x smaller than Engram's `agent` profile.

Two things landed ahead of their milestone, both because a real corpus needed them:

- **Engram import** (`vestigio import`) — 179 memories migrated, with project-name consolidation and type collapsing. Self-retrieval baseline on those 179: 100% found by title, 84% ranked first when the query is degraded.
- **Inspect and repair commands** (`projects`, `list`, `show`, `edit`, `rm`, `verify`) — a store you cannot look inside is a store you cannot trust, and there is no reason to make an agent pay for the ability to look.

Next: M2 real budget packing + recall evaluation set · M3 near-duplicate merge via simhash · M4 decay-based eviction · M5 multi-platform releases.

## Contributing

Pull requests are welcome — [CONTRIBUTING.md](CONTRIBUTING.md) is what review will hold you to. Everything reaches `main` through a pull request with CI green, the maintainer included.

Security issues go through [private advisories](https://github.com/valzkat1/vestigio/security/advisories/new), not the issue tracker. The threat model is written down plainly in [SECURITY.md](SECURITY.md) — it is a local single-user tool, and the failure it takes most seriously is quiet data loss.

## License

[Apache-2.0](LICENSE).
