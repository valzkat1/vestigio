# Security

## Reporting a vulnerability

Report it privately through GitHub: **[Security → Report a vulnerability](https://github.com/valzkat1/vestigio/security/advisories/new)**.

Please do not open a public issue for a vulnerability. The advisory form is private until a fix ships, which is the point of it.

Expect a first response within 7 days. This is a single-maintainer project, so that is a realistic commitment rather than an optimistic one.

Include what you need to reproduce it: version (`vestigio version`), OS, and the sequence of calls or commands. A proof of concept helps more than a description.

## What is in scope

Only the latest commit on `main` is supported. There are no maintained release branches yet.

| In scope | Out of scope |
|---|---|
| Argument or query handling that lets input escape its intended effect — a `forget` that deletes more than it matched, a `recall` that returns another project's rows | Anything requiring an attacker who already has your user account or filesystem access |
| Path handling in `VESTIGIO_DB` or `--body-file` that escapes where it should write | The database being readable by your own user — see below, it is by design |
| Anything letting MCP tool input reach the shell, the filesystem, or the network | Denial of service by feeding it an enormous file locally |
| A dependency vulnerability that is actually reachable from this code | A `govulncheck` finding in code paths vestigio never calls |

## Threat model, stated plainly

vestigio is a **local, single-user tool**. It listens on no port, opens no socket, and speaks JSON-RPC over stdin and stdout to a process you started. There is no server, no auth, and no multi-tenancy, because there is nothing to authenticate against.

Two consequences worth being explicit about, because both are design decisions and neither is a bug:

**The database is not encrypted.** `~/.vestigio/vestigio.db` is plain SQLite, readable by your user, exactly like your shell history or your git objects. If your disk is not encrypted, neither is your memory.

**Memories can hold whatever an agent decided to write.** vestigio does not inspect content, and an agent that saw a credential is an agent that can write one down. `vestigio list` and `vestigio show` exist partly so you can look, and `vestigio rm` exists so you can act on what you find.

The failure this project takes most seriously is not remote compromise. It is **quiet data loss**: a project name resolved wrong so recall returns empty, or a delete that matched more than it was asked to. That second one was a real bug, found in end-to-end testing and fixed by a rule that now holds throughout — reads are generous, deletes are strict. Report anything in that family; it is treated as a security issue here even when nobody else would call it one.
