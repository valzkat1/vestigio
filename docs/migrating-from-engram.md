# Migrating from Engram

Two halves, and the small one is the one everybody plans for.

**The data** is a JSON export and one command. Half an hour, most of it spent deciding project names.

**The instruction layer** is the rest. In the migration this document was written from, a live
installation held **225 references to Engram across 21 files** — and the MCP server entry was four
of them. Config was 1.8% of the work. The other 98% was prompts, rules and skills telling an agent
to call tools that were about to stop existing.

That asymmetry is the whole point of this page. A migration that swaps the server and stops there
does not produce "off". It produces a system under standing orders to do the impossible, failing
quietly.

---

## Half one: the data

```bash
vestigio import ~/engram-export.json --dry-run
vestigio import ~/engram-export.json --map=myrepo-backend=myrepo --skip=session_summary
```

Always dry-run first. It prints the full plan — counts per project, per kind, total size, and how
many memories exceed a default budget on their own — and writes nothing.

Full flag reference: [cli.md](cli.md#vestigio-import).

### What the import fixes on the way in

Migration is not a copy. An export carries three problems a straight insert would preserve forever.

**Project names drift.** The same repository shows up as `alcubo`, `alcubo-backend` and
`reposa2censo` depending on how detection resolved that day. Memory split across name variants is
memory that never gets recalled. `--map=old=new,...` consolidates them; run `--dry-run` first and
read the per-project counts, because that list is where the drift becomes visible.

**Types do not line up.** Engram has eleven, vestigio has five, deliberately. The fold is applied
for you:

| Engram type | vestigio `kind` | Why |
|---|---|---|
| `decision` | `decision` | — |
| `architecture` | `decision` | An architecture note is a decision with more prose around it |
| `bugfix` | `bugfix` | — |
| `pattern` | `pattern` | — |
| `preference` | `constraint` | A preference is a constraint on how work gets done |
| `feedback` | `constraint` | Same |
| `config` | `constraint` | Same |
| `discovery` | `reference` | Worth keeping, no stronger claim |
| `analysis` | `reference` | Same |
| `manual` | `reference` | Same |
| anything else | `reference` | The honest default |

Authoritative source is `kindMap` in `internal/importer/engram.go` — if this table and that map ever
disagree, the map is right.

**Timestamps are preserved.** A decision made in April is evidence about April, and stamping it with
today's date destroys the ordering that makes an imported corpus worth having. `Import` writes the
original `created_at` and `updated_at`; only `remember` uses now.

### `scope: personal` lands in a project called `personal`

Engram scopes a memory `project` or `personal`. vestigio has **only** project scope — every query is
filtered to one project in SQL, with no fallback.

The importer resolves this by convention: a `personal`-scoped observation is pulled out of whatever
project it was captured in and filed under a project literally named `personal`. Personal knowledge
follows the person, not the repository.

**Be clear about what that does and does not buy you.** The data is not lost, and
`vestigio list --project=personal` will show it. But nothing recalls it automatically from another
project, and there is no way for an agent to *write* there. If you depended on personal-scope
memory surfacing everywhere, that behaviour does not exist yet — see the
[audit](codex-memory-audit.md) for why a `global` scope is the first item on the roadmap.

### What does not come across

| Engram concept | Status |
|---|---|
| `topic_key` | **Dropped.** vestigio deduplicates by content hash; there are no keys. Parsed from the export and ignored. |
| Session summaries | Kept only if you do not `--skip` them. They are transcripts, not facts — most are noise. |
| Passive capture | No equivalent. It was a server-side feature. |

### Two things to check after importing

```bash
vestigio projects                       # per-project counts and token totals
vestigio list --project=X --limit=100   # per-memory cost
```

An Engram corpus tends to be **large-grained**. In the corpus this was written against the median
memory was ~1,574 characters, and thirteen exceeded an 800-token budget on their own — meaning a
single one crowds out everything else in a recall. Bodies are imported whole on purpose (truncating
would be lossy and irreversible), so this is yours to clean up with `vestigio edit` and
`vestigio rm` when it bites.

---

## Half two: the instruction layer

This is the part that breaks quietly.

### Find the surface before you change anything

The question is never "where is the switch". It is "who depended on this". Sweep every alias, not
just the package name:

```bash
rg -i -c "engram|mem_save|mem_search|mem_context|mem_get_observation|\
mem_session_summary|mem_suggest_topic_key|mem_update|mem_capture_passive|topic_key" \
  ~/.codex/ ~/.claude/ ./ --glob '!node_modules'
```

Count the hits per file and **write the number down**. It is what you will check against when you
think you are finished.

Expect them in five places:

| Layer | Where it hides | If you miss it |
|---|---|---|
| Activation | `config.toml`, `settings.json`, plugin manifests | The server still loads |
| **Instructions** | `AGENTS.md`, `CLAUDE.md`, `rules/`, `skills/`, prompt files | **Orders to call tools that no longer exist** |
| Dependent subsystems | anything using memory as a *backend* or *artifact store* | Breaks or loses data silently |
| Permissions | allowlists, scopes | Dead entries; painful to revert |
| Callers | scripts, CI, cron | Runtime failure |

### The capability map

Do not find-and-replace tool names. Half of Engram's protocol has no counterpart because vestigio
solves the same problem with a different shape — and renaming a step that should be deleted leaves
an instruction that cannot succeed.

| Engram | vestigio | What to do |
|---|---|---|
| `mem_save` | `remember` | Replace, and remap the type enum |
| `mem_search` | `recall` | Replace — **and delete the second step** |
| `mem_get_observation` | — | **Delete.** `recall` returns the full text; there is no fetch step |
| `mem_context` | — | **Delete.** No session-history concept |
| `mem_session_summary` | `remember(kind: reference)` | Different shape, not a rename — see below |
| `mem_suggest_topic_key` | — | **Delete.** Dedupe is by content hash |
| `mem_update` | `remember` | **Delete.** Saving existing content updates it |
| `mem_capture_passive` | — | **Delete.** Server-side feature, no equivalent |
| — | `forget` | **New.** Engram's protocol had no delete |

Three tools, not eleven. If your instructions describe an eight-step memory protocol, most of those
steps are gone rather than renamed.

**Session close** is the one that needs rewriting rather than deleting. Engram had a dedicated call
that stored a whole transcript. vestigio stores facts — so the port is: make sure every durable
fact from the session is already saved, then add one closing entry with `kind: reference` titled
`Session YYYY-MM-DD — <topic>`, carrying what was accomplished, what is next, and which files
matter. Do not dump a log into it.

### Concepts with no 1:1

Before porting a noun, ask how the new system already handles that problem. If you port the noun,
you invent an artifact nothing reads or writes.

The clearest example from a real migration: an SDD pipeline kept an `apply-progress` artifact keyed
by `topic_key`, so batches could merge their progress. There is no such artifact on the other side —
progress **is** the `[x]` marks in `tasks.md`. Renaming it would have created a file that nothing
ever read, and the batches would have silently overwritten each other.

### Do not use vestigio as an artifact store

If a subsystem used Engram as a *backend* for documents — specs, proposals, plans addressed by
`topic_key` — do not point it at vestigio. Three reasons, each sufficient:

- **No deterministic keys.** vestigio dedupes by content similarity. There is no way to reliably
  fetch or overwrite one specific artifact.
- **Working-directory scoping.** Memory is scoped to the cwd, so a sub-agent running in a worktree
  reads and writes a different store than its orchestrator. Artifacts vanish without an error.
- **Wrong shape.** Memory holds small self-contained facts. Artifacts are large documents rewritten
  per phase.

Point those subsystems at files. Keep vestigio for what was *learned*.

For the same scoping reason: if sub-agents used to write memory themselves, **centralize writes in
the orchestrator**. Have sub-agents report what they learned and let the parent save it.

---

## Three traps, all of them observed

**1. A gate is not a migration.** Making a dependent conditional — "only if memory is enabled" —
schedules a second decommission that nobody has on a calendar. Dead branches keep costing prompt
tokens and keep offering a mode that cannot work. Migrate the dependent or write down that you
didn't.

**2. Classify per instruction, never per section.** Reading a section, concluding "this whole area
is conditional" and moving on is the failure that survives a first audit. A section can be 90%
gated and still contain a step marked **MANDATORY** with no condition on it — and that is the one
that runs. In the migration this was written from, three such blocks existed: an init guard, a
strict-TDD forwarder, and a batch-continuity check. All three did an unconditional `mem_search`, so
with the server gone they failed *into their fallback path* rather than erroring. Strict TDD simply
stopped activating.

**3. Check what points INTO a block before deleting it.** A self-contained-looking section is often
load-bearing for its neighbour. A real case: a skill file had two `{same table as above}`
references pointing at a table that lived inside the Engram-specific block queued for deletion.
Deleting cleanly would have orphaned both. Move the shared part out first, then delete.

---

## Procedure

1. **Export from Engram** and keep the file. It is your only copy of the old corpus.
2. **Back up the instruction directories.** `~/.codex`, `~/.claude` and friends are usually not git
   repos. There is no undo.
3. **Sweep and count.** Record hits per file before editing anything.
4. **Classify every hit**: replace · gate · inert · keep-for-revert. Line by line.
5. `vestigio import --dry-run`, read the plan, then import for real.
6. **Rewrite the instruction layer** against the capability map above.
7. **Re-sweep.** The count must drop to the number you predicted. Anything left should be inert on
   purpose — for example, sentences that *explain* that vestigio has no `topic_key`.
8. **Switch the server** in config. This step is last and it is the smallest.
9. **Restart the client.** MCP servers launch at startup; editing config does nothing to a running
   session.

## Verifying

```bash
vestigio projects                    # everything landed, under the names you expect
vestigio list --project=personal     # personal-scope memories, if you had any
vestigio verify                      # derived columns are consistent
```

Then ask the agent to recall something only the old store knew. A successful import that the agent
never queries is not a migration, it is a backup.

If recall comes back empty, check project detection before anything else — it is the most common
failure and it reads exactly like data loss. The resolved project name is printed to stderr on
startup for that reason. See [codex.md](codex.md#project-detection--read-this-one).

---

## Do you still need Engram installed?

No. Nothing in vestigio talks to it. The export is a plain JSON file, and once imported the data is
yours in `~/.vestigio/vestigio.db`.

Keep the export file anyway. It is cheap, and it is the only way back.
