<!--
This mirrors the receipt format in .rdd/ledger.md. It is short on purpose.
The "does not prove" field is the one that earns its place — see CONTRIBUTING.md.
-->

## Intent

<!-- What was wrong, or missing, before this. Not what the diff does. -->

## Changed

<!-- Files and what happened in each. -->

## Ran

<!-- Real commands with their real output. "go test ./... -> 44 passed, 0 failed"
     beats "tests pass". If you did not run something CI will run, say so. -->

- [ ] `gofmt -l .` prints nothing
- [ ] `go vet ./...`
- [ ] `go test ./...`
- [ ] `go mod tidy` leaves `go.mod` and `go.sum` unchanged

## What this does not prove

<!-- Untested paths, platforms you did not try, assumptions you made.
     Leaving this empty reads as "nothing" and will be taken literally. -->

---

- [ ] New behaviour has a test; a fix has the test that would have caught it
- [ ] No new MCP tool — or the description argues why an agent must *call* it (CONTRIBUTING.md)
- [ ] Failure modes are loud: no unrecognised input silently falling through
