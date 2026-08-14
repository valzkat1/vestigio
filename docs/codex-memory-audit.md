# Codex memory audit

Written against the code as it stands, with every number measured rather than
estimated. Where something could not be measured, it says so.

Date: 2026-08-13 · Commit base: `9600d1d` · Go 1.26.5 windows/amd64

---

## Summary

Three findings decide what the next milestones should be.

**1. Codex already works. The protocol needed nothing.** A harness replaying the
exact frame sequence a Codex client sends passes end to end — handshake, tool
listing, save, retrieve, delete. No Codex-specific MCP implementation is
required, which is what a proposal should hope to conclude and rarely does.

**2. The retrieval engine is not in the request path.** `internal/retrieve`
holds a `Scorer` interface, Reciprocal Rank Fusion and a rank type. It has zero
importers. Real ranking is `ORDER BY bm25(memories_fts)` inside
`store.search` — SQLite does it. The scaffolding is well designed and
disconnected, so any scoring work starts by wiring it in, not by designing it.

**3. The compatibility surface is the instruction layer, not the config.** A
sweep of a live Codex installation found **225 references to the previous memory
system across 21 files**, of which the MCP server entry was four. Three of the
remaining hits are marked MANDATORY with unconditional calls, which means they
fail silently rather than loudly when the server behind them changes.

---

## Current architecture

```
  Codex / Claude Code / any MCP client
                 │  newline-delimited JSON-RPC 2.0 over stdio
                 ▼
  cmd/vestigio/main.go        runMCP: parse args, detect project, open store
                 │
                 ▼
  internal/mcp/server.go      Serve: scan frames, dispatch, encode replies
                 │            recall packs results under the token budget
                 ▼
  internal/store/store.go     Remember / Search / ForgetID / ForgetQuery
                 │            BM25 ordering happens HERE, in SQL
                 ▼
  SQLite (modernc.org/sqlite v1.56.0, pure Go, no CGO)
                 memories + memories_fts (external-content FTS5) + 3 triggers

  internal/retrieve/          Scorer, Hit, Fuse (RRF) — NOT CALLED BY ANYTHING
```

That last box is drawn detached on purpose. It is the honest picture.

### Data flow, one recall

1. `Serve` reads a line, unmarshals it, dispatches on `method`.
2. `tools/call` → `callTool` → `recall(query, budget)`.
3. `store.Search(project, query, 25)` → `SanitizeFTS` quotes each term and joins
   with ` OR ` → `SELECT … WHERE memories_fts MATCH ? AND m.project = ? ORDER BY
   bm25(memories_fts) LIMIT 25`.
4. Results are appended greedily while `used + tokens + 8 <= budget`.
5. `MarkRecalled` bumps `recalled_at` and `recall_count`; failures are swallowed
   so a read never breaks on a write.
6. One text block goes back. No second fetch step exists.

---

## Current MCP surface

Measured on the wire, not counted by hand:

| Metric | Value |
|---|---|
| `tools/list` payload | **1,077 bytes** (~269 tokens at 4 chars/token) |
| Budget enforced in CI | 1,500 bytes (`budgetChars`, `tools_test.go`) |
| Headroom | 423 bytes |
| Tools exposed | 3 — `recall`, `remember`, `forget` |
| Session-start dump | none; recall is on demand |

For contrast, the M0 baseline of the system this replaces: 8,835 chars
(~2,209 tokens) for 11 tools in its `agent` profile, plus a 7,929-char
session-start dump. 51% of that cost was prose in descriptions rather than JSON
Schema.

### Methods handled

| Method | Behaviour |
|---|---|
| `initialize` | Replies with a pinned `protocolVersion`, `tools` capability, serverInfo |
| `notifications/initialized`, `notifications/cancelled` | Accepted, no reply |
| `ping` | Empty result |
| `tools/list` | The three tools; params ignored, present or absent |
| `tools/call` | `recall` / `remember` / `forget` |
| anything else | `-32601`, transport stays alive |

### Codex compatibility verdict

`internal/mcp/codex_test.go` drives the real `Serve` loop with Codex-shaped
frames. All seven tests pass. What each one settles:

| Test | Settles |
|---|---|
| `TestCodexHandshakeAcrossProtocolVersions` | Handshake succeeds for `2024-11-05`, `2025-03-26`, `2025-06-18` |
| `TestCodexOpeningSequence` | The notification draws no reply — a reply here desynchronises the client |
| `TestCodexToolsListWithoutParams` | Works whether `params` is `{}` or omitted |
| `TestCodexUnsupportedMethodsDoNotKillTheTransport` | `resources/list`, `prompts/list` return `-32601` and the session survives |
| `TestCodexFullRoundTrip` | remember → recall → forget, with the body returned in one call |
| `TestCodexHandlesCRLFFraming` | Windows CRLF frames parse |
| `TestCodexProjectScopingIsolatesMemories` | One project cannot read another's memories |

**One measured caveat.** The server answers `2024-11-05` regardless of what the
client requests:

```
client asked 2024-11-05, server answered 2024-11-05
client asked 2025-03-26, server answered 2024-11-05
client asked 2025-06-18, server answered 2024-11-05
```

This is spec-legal — a server may answer with any version it supports, and the
client decides whether to proceed. It was deliberately **left unchanged**: the
harness proves the handshake works today, and changing behaviour that works
without evidence of breakage is how regressions get introduced. The test is the
tripwire for the day a client stops accepting the pinned version.

---

## Current data model

```sql
memories(
  id, project, kind, title, body,
  tokens,                      -- precomputed at write time
  hash,                        -- sha256(title \0 body), first 16 bytes hex
  embedding, embedding_model,  -- present, deliberately empty
  created_at, updated_at, recalled_at, recall_count
)
memories_fts USING fts5(title, body, content='memories', tokenize='porter unicode61')
```

Three triggers keep the FTS index in sync on insert, update and delete.

Observations that matter for the roadmap:

- **`kind` is a closed set of five**: `decision`, `bugfix`, `pattern`,
  `constraint`, `reference`. An unknown value silently becomes `reference`.
- **`embedding` / `embedding_model` are vector-ready and unused.** Bodies are
  stored whole, so enabling vectors later is a backfill, not a migration.
- **`recalled_at` and `recall_count` are written and never read.** `MarkRecalled`
  maintains them for a decay pass that does not exist yet.
- **`hash` and `tokens` are computed in Go, not by triggers.** SQLite has no
  native `sha256()`. Editing the database by hand leaves them stale, which
  silently stops deduplication — hence `vestigio verify` and `edit --fix`.
- **Scope is a single `project` column.** There is no global, module, file or
  session scope, and `search` filters `m.project = ?` with no fallback.

---

## Current retrieval pipeline

```
query → SanitizeFTS (quote each term, join with OR) → FTS5 MATCH
      → ORDER BY bm25() → LIMIT 25 → greedy budget pack → text
```

Design decisions worth recording because they are easy to mistake for bugs:

- **Terms are OR-ed, not AND-ed.** BM25 sorts out precision; AND would silently
  drop anything short of a full phrase match. `TestRecallIsGenerousBecauseTermsAreORed`
  pins this so the trade stays visible.
- **Deletes invert it.** `ForgetQuery` uses `SanitizeFTSAll` (AND). Reads are
  generous, deletes are strict — a real bug found in end-to-end testing, where a
  two-word delete removed two unrelated memories.
- **Porter stemming is on.** It closes lexical gaps like plural forms for free,
  but not cross-language ones.

### Measured retrieval quality

`internal/store/eval_test.go` is an evaluation set, not a unit test: 12 realistic
memories, 15 paraphrased queries that deliberately avoid the memory's own wording.

| Metric | Value |
|---|---|
| recall@1 | **10/15 (67%)** |
| recall@3 | **12/15 (80%)** |
| never retrieved | 1/15 |

The floors are a ratchet: the build fails if recall drops **and** if it improves
without raising the floor. Stable across five consecutive runs.

The three known failures share one cause — no semantic bridge:

- `"dónde quedan registradas las corridas"` → the ledger memory. Zero shared terms.
- `"sistema de recuerdos persistente en Golang"` → `Go`, not `Golang`.
- `"por qué no imprimir logs por salida estándar"` → the stderr memory.

This is the concrete evidence for or against embeddings, and it says the gap is
partly lexical (synonyms, cross-language) rather than purely semantic.

---

## Current token accounting

`EstimateTokens` returns `(runeCount + 3) / 4`.

It is an **estimate, not a measurement**, kept at 4 chars/token deliberately so
the before/after comparison against the M0 baseline stays apples-to-apples. It
runs at write time and is stored in the `tokens` column, because the budget
packer cannot afford to tokenise while serving a query.

The recall packer adds a flat **8 tokens per memory** for the `#id [kind] title`
header line. `budget_tokens` defaults to 800 when omitted, zero or negative.

Two consequences to be honest about:

1. For non-ASCII text the 4 chars/token ratio drifts — Spanish and accented
   content tokenises worse than the estimate suggests, so real usage runs
   slightly over the stated budget.
2. Any published token figure derived from this is an approximation. Character
   counts are exact; token counts are not.

---

## Strengths

- **The tool surface is defended by the build.** `TestToolSchemaFitsBudget` and
  `TestToolCountIsThree` turn the product thesis into a failing test.
- **`budget_tokens` is a real ceiling.** Not a suggestion — verified by mutation
  check: disabling the ceiling condition fails four tests.
- **One round trip.** `recall` returns bodies. There is no search-then-fetch.
- **Project isolation holds** at the SQL level, proved across both suites.
- **The evaluation set catches what coverage cannot.** Flipping `SanitizeFTS` to
  `SanitizeFTSAll` drops recall from 67% to 13% while the entire pre-existing
  suite stays green. Only the eval tests catch it.
- **Operational surface costs agents nothing.** Ten CLI commands, zero MCP tools.
- **Argument parsing rejects what it does not recognise**, after a class of bug
  where `rm 1 2 --yes` reported success and deleted one of two.

## Weaknesses

| # | Weakness | Evidence |
|---|---|---|
| W1 | `internal/retrieve` is dead code; scoring has no seam in the live path | zero importers |
| W2 | Only `project` scope exists; nothing global, and no fallback | `store.go:172` |
| W3 | Budget packing is greedy — no truncation, no swapping a long low-ranked memory for two short better ones | `server.go:187-199` |
| W4 | Lifecycle columns are written and never read | `MarkRecalled` has no consumer |
| W5 | Near-duplicate detection absent; only exact-content hashing | `contentHash` |
| W6 | Token counting is an estimate that drifts on non-ASCII | `EstimateTokens` |
| W7 | No `stats` or `recall-debug` command for retrieval debugging | CLI has `projects` only |
| W8 | No memory-policy or multi-agent documentation | `docs/` has `cli.md` only |
| W9 | 13 memories in the real corpus exceed an 800-token budget alone | M0 import receipt |
| W10 | Eval set is 15 queries written by the corpus author — known, unmeasured bias | `eval_test.go` header |

## Risks

- **Silent project misdetection.** Codex launches the server as a subprocess, so
  the working directory is Codex's choice. A wrong project returns empty and
  reads like data loss. Mitigated by printing the resolved project to stderr and
  by `VESTIGIO_PROJECT`; documented in `docs/codex.md`.
- **Pinned protocol version.** Legal today, a break the day a client drops
  `2024-11-05`. The harness is the tripwire.
- **Recalled memories are untrusted input.** A memory can contain anything an
  agent wrote, including text shaped like instructions. Retrieved memories are
  contextual knowledge, **not** instructions carrying authority over the agent's
  current directives. This belongs in the agent-facing policy, not only here.
- **Coverage is uneven**: `internal/mcp` 98.8%, `internal/retrieve` 100% (of code
  nothing calls), `internal/store` 72.9%, `cmd/vestigio` 64.8%,
  `internal/importer` 48.7%.

---

## Recommended architecture

The target from the proposal, with the one correction the audit found — the
engine box has to be *connected*, not built:

```
recall(query, budget_tokens)
        │
        ▼
  candidate retrieval      store.Search → FTS5 BM25            EXISTS
        │
        ▼
  metadata filtering       project, kind, age                  PARTIAL (project only)
        │
        ▼
  scoring                  Scorer interface + RRF               EXISTS, NOT WIRED
        │
        ▼
  deduplication            exact hash, then SimHash             PARTIAL (exact only)
        │
        ▼
  budget packing           greedy → real packing                PARTIAL (ceiling honoured)
        │
        ▼
  response
```

Embeddings stay out until the eval set shows FTS5 is the limit. Two of the three
known failures are lexical, so synonym expansion should be tried first — it is
cheaper, needs no model, and keeps the binary CGO-free.

### On the proposed memory model

The proposal suggests nine kinds. **Recommendation: keep five.** Every enum value
is schema text paid in every session of every agent, and the proposed
`architecture`, `convention`, `solution`, `dependency`, `workflow` and `context`
collapse cleanly onto the existing set (`architecture` → `decision`,
`convention` → `pattern`, `dependency`/`workflow` → `reference`). If a kind
cannot be inferred from the body, it is metadata that earns its bytes; if it can,
it is duplication.

### On negative memories

**Recommendation: no `polarity` column.** `kind=constraint` already carries "do
not use X". Adding a field to express what an existing field expresses is the
complexity the proposal itself warns against.

### On scope — the one real gap

`scope: personal` from the previous system has **nowhere to go**. `search`
filters by project with no fallback, so a user-level preference either gets
duplicated into every project or is lost. This is the strongest concrete
argument for the scoping work, and it is the only capability the migration
cannot carry across honestly.

Recommended minimum: a `global` sentinel project searched as a fallback, ranked
below project matches, never above. `module`, `file` and `session` scopes are not
justified by evidence yet.

---

## Incremental plan

| Milestone | Scope | Complexity | Risk |
|---|---|---|---|
| **M1** (this) | Audit, Codex harness, `docs/codex.md`, instruction-layer migration | Low | None to the engine — no production code changed |
| **M1a** | `vestigio seed`: bootstrap a store from documents the user already wrote | Low | CLI-only, zero MCP surface; two content-loss bugs found by running it |
| **M2** | `global` scope + project-aware ranking. Schema add, fallback in `search`, ranking weight | **Medium** | First schema change; needs migration test and eval re-baseline |
| **M3** | Real budget packing: truncation with a floor, swap long low-rank for short high-rank | **Medium** | Contract already promises the ceiling, so callers see no break |
| **M4** | Wire `internal/retrieve` into `search`; register BM25 as the first `Scorer`; add freshness/kind weights | **Medium-high** | Touches the ranking that eval measures — the ratchet is the safety net |
| **M5** | SimHash near-duplicate merge on write, reusing the `hash` column | **Medium** | Merging is destructive; needs a dry-run path |
| **M6** | Decay/eviction reading `recalled_at` / `recall_count`; never evict `constraint`/`decision` | **Medium** | Deletes data — must be opt-in and reversible |
| **M7** | `vestigio stats`, `vestigio recall-debug` — CLI only, never MCP | **Low** | None; zero agent context cost |
| **M8** | Multi-agent and handoff guidance, `docs/memory-policy.md` | **Low** | Documentation |

Sequencing note: **M4 should follow M2 and M3**, not lead. Wiring the scorer
before scope and packing exist means rewriting it twice.

### What must not regress

1. `tools/list` stays under 1,500 bytes — currently 1,077.
2. Three tools. Operational surface stays in the CLI.
3. `budget_tokens` stays a hard ceiling.
4. recall@1 ≥ 10/15 and recall@3 ≥ 12/15, raised when improved.
5. Project isolation.
6. Existing CLI, database format, and Engram import keep working.
7. CGO-free single binary.
