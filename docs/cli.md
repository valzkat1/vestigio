# CLI reference

Three tools reach the agent: `recall`, `remember`, `forget`. Everything else lives here.

That split is the product, not an accident of packaging. Every tool exposed over MCP is schema text
injected into every session of every agent, whether or not it is ever called — so a command a human
runs once a month has no business costing context forever. Browsing, repairing, auditing and
importing are human operations, and they are free down here.

```
vestigio <command> [arguments] [flags]
```

| Command | What it does |
|---|---|
| [`mcp`](#vestigio-mcp) | Start the MCP server over stdio |
| [`import`](#vestigio-import) | Migrate an Engram JSON export |
| [`projects`](#vestigio-projects) | Inventory of memories and tokens per project |
| [`list`](#vestigio-list) (`ls`) | List memories, newest first |
| [`show`](#vestigio-show) (`cat`) | Print one memory in full |
| [`edit`](#vestigio-edit) | Rewrite a memory, recomputing hash and tokens |
| [`rm`](#vestigio-rm) (`delete`) | Delete a memory |
| [`verify`](#vestigio-verify) | Report rows whose derived columns drifted |
| [`version`](#vestigio-version) | Print the version |
| [`help`](#vestigio-help) | Print usage |

Running `vestigio` with no arguments prints usage and exits `2`.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Runtime failure — or, for `verify`, drift was found |
| `2` | Usage error: missing command, unknown command, missing or invalid id |

`verify` returning `1` on drift is deliberate: it makes the command usable as a check in a
pre-commit hook or CI job without parsing its output.

## Environment

| Variable | Default | Meaning |
|---|---|---|
| `VESTIGIO_DB` | `~/.vestigio/vestigio.db` | Database path. Falls back to `./vestigio.db` if the home directory cannot be resolved. |
| `VESTIGIO_PROJECT` | — | Override project detection entirely. |

The parent directory of `VESTIGIO_DB` is created on open, so pointing it at a fresh path just works.

## How the project is resolved

Memories are scoped to a project, resolved in this order:

1. `VESTIGIO_PROJECT`, if set.
2. `git config --get remote.origin.url` — the remote is stripped of a trailing `.git` and `/`, cut
   at the last `/` or `:`, and lowercased. `git@github.com:valzkat1/AlCubo.git` → `alcubo`.
3. The current directory's base name, lowercased.
4. `default`.

The remote comes before the directory name so the same repository resolves identically from every
clone path. That ordering has a sharp edge worth knowing about: **adding a remote to a repository
changes the source of the answer**, and if the remote's repo name differs from the directory name,
every memory filed under the old key goes quiet. Nothing is lost, but nothing is found either, and
an empty `recall` reads exactly like data loss.

Two things guard against it. The resolved name is printed to stderr on every `mcp` startup, and
`vestigio projects` lists every key that exists in the database. If recall goes empty, compare them
before assuming anything is gone.

**Ids are global, not project-scoped.** `show`, `edit` and `rm` take an id and find the row wherever
it lives, and print the project they touched. Scoping ids to the detected project would mean the CLI
reports "not found" for a row that plainly exists, purely because of which directory you ran it from
— the same silent-wrong-project failure, in a second costume.

---

## `vestigio mcp`

```
vestigio mcp [--project=NAME]
```

Starts the MCP server, speaking JSON-RPC over stdin/stdout. Protocol version `2024-11-05`.

`--project=NAME` pins the project, bypassing detection. Equivalent to `VESTIGIO_PROJECT`.

**stdout is reserved for JSON-RPC frames.** The startup banner — `vestigio 0.1.0 — project "alcubo"`
— goes to stderr on purpose. Anything printed to stdout would corrupt the stream and the client
would drop the connection.

Client configuration, for Claude Code (`.mcp.json`) and identical in shape everywhere else:

```json
{ "mcpServers": { "vestigio": { "command": "vestigio", "args": ["mcp"] } } }
```

---

## `vestigio import`

```
vestigio import <export.json> [--dry-run] [--map=old=new,...] [--skip=type,...]
```

Migrates an Engram JSON export into the store. Import is a CLI command rather than an MCP tool for
the usual reason: an agent never needs to call it, so it should never be in an agent's context.

| Flag | Effect |
|---|---|
| `--dry-run` | Report what would happen, write nothing. Prints `DRY RUN — no se escribe nada` first. |
| `--map=old=new,...` | Rename projects on the way in. Comma-separated pairs; both sides lowercased and trimmed. |
| `--skip=type,...` | Drop observation types. Comma-separated. |

```bash
vestigio import ~/engram-export.json --dry-run
vestigio import ~/engram-export.json --map=myrepo-backend=myrepo,old-name=myrepo --skip=session,log
```

Exits `2` with a usage line if no arguments or no file path is given.

**Flag values are inline — the `=` is required.** A space-separated `--map old=new` is rejected
with `--map takes its value inline: --map=VALUE` and exit `2`. So is an unknown flag, and so is a
second positional argument:

```
$ vestigio import uno.json dos.json
vestigio: unexpected argument "dos.json" — the export path is already "uno.json"
```

That strictness is a fix, not a preference. The parser used to skip arguments it did not recognise,
so `--map old=new` split into a bare `--map` that matched nothing and an `old=new` that looked like
a positional — silently replacing the export path. Import then ran against a file named `old=new`
and reported a plain "not found", pointing nowhere near the flag that caused it.

---

## `vestigio projects`

```
vestigio projects
```

Inventory of the whole database. No flags — it is deliberately unscoped, because its job is to
answer "what keys exist?" when a scoped command comes back empty.

```
PROJECT                  MEMORIES  TOKENS
alcubo                   114       52106
bvc-a2censo-backend-acc  63        30463
personal                 7         2217
                         187       87719

database: C:\Users\victo\.vestigio\vestigio.db
```

The unlabelled final row is the total. The database path is printed last so you always know which
file you are looking at — useful when `VESTIGIO_DB` is set and you have forgotten.

The `TOKENS` column is the sum of the precomputed per-row token counts. It is what `budget_tokens`
spends against, which makes this the fastest read on how expensive a project's memory has become.

---

## `vestigio list`

```
vestigio list [--project=NAME] [--all] [--kind=KIND] [--limit=N]
vestigio ls   [...]
```

Lists memories, newest first.

| Flag | Default | Effect |
|---|---|---|
| `--project=NAME` | detected project | Scope to one project. |
| `--all` | off | Every project. Overrides `--project`. |
| `--kind=KIND` | all kinds | One of `decision`, `bugfix`, `pattern`, `constraint`, `reference`. |
| `--limit=N` | `30` | Maximum rows. A value that is not an integer is ignored and the default stands. |

```
ID   KIND        TOKENS  UPDATED           TITLE
187  constraint  519     2026-08-07 02:29  Working Agreement — ambos evolucionamos, y las lec…
186  decision    574     2026-08-07 02:18  Subcomandos nativos de inspeccion y edicion en el C…

3 memories — alcubo
```

Titles are truncated to 60 characters, cut on runes rather than bytes — titles here are routinely
non-ASCII and byte-slicing would produce mojibake. Timestamps are local time, `YYYY-MM-DD HH:MM`.

When a scoped list comes back empty, the command says so and points at `vestigio projects` rather
than leaving you to guess whether the store is empty or the project key is wrong.

---

## `vestigio show`

```
vestigio show <id>
vestigio cat  <id>
```

Prints one memory in full — the complete body, never truncated.

```
#186  [decision]  project alcubo
title    Subcomandos nativos de inspeccion y edicion en el CLI de vestigio
tokens   574
hash     a1b2c3d4e5f6...
created  2026-08-07 02:18
updated  2026-08-07 02:18
recalled 3 time(s), last 2026-08-07 09:41
------------------------------------------------------------------------
<body>
```

`recalled` is the retrieval counter and the timestamp of the last hit. It is the raw material for
decay-based eviction later: a memory nothing has recalled in months is a candidate for removal in a
way that a memory's age alone never is.

Exits `2` if the id is missing or not a number, `1` if no such row exists.

---

## `vestigio edit`

```
vestigio edit <id> [--title=TEXT] [--kind=KIND] [--body-file=FILE] [--fix]
```

Rewrites a memory **through the store**, so `hash` and `tokens` are recomputed. This is the only
supported way to change a memory by hand — see [Never edit the database directly](#never-edit-the-database-directly).

Behaviour depends on which flags are present:

| Invocation | What happens |
|---|---|
| `edit <id>` | Opens `$VISUAL` (else `$EDITOR`) on the current body. Saving an unchanged body prints `no changes` and writes nothing. |
| `edit <id> --body-file=FILE` | Takes the new body from a file. No editor involved — the form to use in scripts, or when no editor is configured. |
| `edit <id> --title=... --kind=...` | Changes metadata only. The editor is **not** opened and the body is untouched. |
| `edit <id> --fix` | Recomputes the derived columns without changing a character of content. |

With no `$VISUAL` or `$EDITOR` set, the bare form fails with a message pointing at `--body-file`
rather than dropping you into an editor you did not choose.

The result reports what actually moved:

```
#186 updated
  tokens 574 -> 581
  hash   a1b2c3d4e5f6… -> 7f8e9d0c1b2a…
  FTS index resynced by the memories_au trigger
```

An unchanged hash is reported as `hash unchanged — content is identical`, which is the expected
outcome of `--fix` on a row whose only problem was a stale token count.

---

## `vestigio rm`

```
vestigio rm     <id> [--yes]
vestigio delete <id> [--yes]
```

Deletes one memory by id. Without `--yes` it prints what it is about to remove and waits for
confirmation; only `y` or `yes` (any case) proceed, anything else prints `cancelled` and exits `0`.

```
#186 [decision] Subcomandos nativos de inspeccion y edicion en el CLI de vestigio
  project alcubo, 574 tokens
delete? [y/N]
```

**There is no undo.** Take a copy of the database first if you are deleting in bulk.

Note the asymmetry with the `forget` MCP tool, which deletes by query. That path had a real bug
found in end-to-end testing: it reused recall's search, which joins terms with `OR`, so
`forget("FTS5 syntax")` deleted everything containing *either* word. The fix generalises past this
one command — **reads are generous, deletes are strict**: recall unions terms, forget intersects
them. `rm` sidesteps the question entirely by taking an id.

---

## `vestigio verify`

```
vestigio verify
```

Recomputes `hash` and `tokens` for every row and reports the ones that no longer match their
content. Clean database:

```
ok — every hash and token count matches its content
```

Drift found — **exit code `1`**:

```
2 row(s) drifted:
  #42: hash
  #97: hash, tokens 300->318

repair with: vestigio edit <id> --fix
```

---

## `vestigio version`

```
vestigio version
vestigio --version
vestigio -v
```

Prints `vestigio <version>`. The same string goes out in the MCP `initialize` handshake.

## `vestigio help`

```
vestigio help
vestigio --help
vestigio -h
```

Prints usage. An unknown command prints `unknown command: <name>` to stderr, then usage, and exits
`2`.

---

## Never edit the database directly

It is SQLite, so a GUI browser or a raw `UPDATE` will happily let you change a memory. Do not.

The FTS5 index is not the problem — the `memories_ai` / `memories_ad` / `memories_au` triggers keep
it in sync through plain SQL writes. The problem is the two derived columns:

- **`hash`** is computed on write, in Go. A stale hash silently stops `remember` from recognising
  content it already holds, so instead of updating a memory it inserts a near-duplicate. Nothing
  errors. The store just quietly starts growing copies.
- **`tokens`** is precomputed because the budget packer cannot count at query time. Desynchronise it
  and `budget_tokens` stops being a real ceiling — which is the one guarantee vestigio makes.

SQLite has no native `sha256()`, so **maintaining the hash from pure SQL is impossible.** That is
not a missing feature, it is the reason `edit` exists.

If something already got in and edited rows behind the CLI's back:

```bash
vestigio verify                 # find the drifted rows
vestigio edit <id> --fix        # repair each one, content untouched
```

---

## Recipes

**Audit what memory is costing you**

```bash
vestigio projects                          # tokens per project, largest first
vestigio list --project=myrepo --limit=100 # per-memory token cost
```

**Health check, script-friendly**

```bash
vestigio verify || echo "drift found — run: vestigio edit <id> --fix"
```

**Back up before anything destructive**

```bash
cp ~/.vestigio/vestigio.db ~/.vestigio/vestigio.db.bak
```

**Work against a throwaway database**

```bash
VESTIGIO_DB=/tmp/scratch.db vestigio import ~/engram-export.json
VESTIGIO_DB=/tmp/scratch.db vestigio list --all
```

**Find what is filed under the wrong project after a rename**

```bash
vestigio projects                     # spot the orphaned key
vestigio list --project=old-name      # confirm what is in there
```

**Review a kind across every project**

```bash
vestigio list --all --kind=decision --limit=50
```
