package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These drive runSeed against a throwaway database, the same way admin_test.go
// drives the browse commands. The point is not the parser — internal/seed tests
// that — it is the wiring: that a bad flag costs nothing, that --dry-run really
// writes nothing, and that a second run merges instead of duplicating.

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

const seedDoc = `# Decisions

## Elegimos Go para el binario

Binario estático, sin CGO. Descartamos Node por el peso de node_modules.

## [bugfix] El binario moría en macOS

Go 1.22 generaba binarios sin LC_UUID que el dyld actual rechaza.
`

func TestSeedWritesMemoriesAndIsIdempotent(t *testing.T) {
	cli(t)
	path := writeTemp(t, "decisions.md", seedDoc)

	out, code := capture(t, func() int { return runSeed([]string{path, "--project=seedtest"}) })
	if code != 0 {
		t.Fatalf("seed exited %d: %s", code, out)
	}
	if !strings.Contains(out, "2 created") {
		t.Errorf("expected 2 created, got:\n%s", out)
	}

	listed, _ := capture(t, func() int { return runList([]string{"--project=seedtest"}) })
	if !strings.Contains(listed, "Elegimos Go") {
		t.Errorf("memory not stored:\n%s", listed)
	}
	// The kind cascade must survive the round trip: one from the H1 section,
	// one from the inline marker.
	if !strings.Contains(listed, "decision") || !strings.Contains(listed, "bugfix") {
		t.Errorf("kinds were lost between parse and store:\n%s", listed)
	}

	// Re-seeding the same document must update, not duplicate. This is what
	// makes seed safe to re-run after editing one section of a long file.
	again, code := capture(t, func() int { return runSeed([]string{path, "--project=seedtest"}) })
	if code != 0 {
		t.Fatalf("second seed exited %d: %s", code, again)
	}
	if !strings.Contains(again, "0 created, 2 merged") {
		t.Errorf("re-seeding should merge all of them, got:\n%s", again)
	}
}

func TestSeedDryRunWritesNothing(t *testing.T) {
	cli(t)
	path := writeTemp(t, "decisions.md", seedDoc)

	out, code := capture(t, func() int { return runSeed([]string{path, "--project=dry", "--dry-run"}) })
	if code != 0 {
		t.Fatalf("dry run exited %d: %s", code, out)
	}
	if !strings.Contains(out, "DRY RUN") {
		t.Errorf("dry run did not announce itself:\n%s", out)
	}
	if !strings.Contains(out, "Elegimos Go") {
		t.Errorf("dry run should show the plan:\n%s", out)
	}

	listed, _ := capture(t, func() int { return runList([]string{"--project=dry"}) })
	if strings.Contains(listed, "Elegimos Go") {
		t.Fatalf("DRY RUN WROTE TO THE STORE:\n%s", listed)
	}
}

func TestSeedReportsTheCutItChose(t *testing.T) {
	cli(t)
	path := writeTemp(t, "decisions.md", seedDoc)

	out, _ := capture(t, func() int { return runSeed([]string{path, "--project=cut", "--dry-run"}) })
	if !strings.Contains(out, "cut at H2") {
		t.Errorf("the chosen cut must be reported — it is a guess about the user's document:\n%s", out)
	}
}

func TestSeedPlainTextUsesRules(t *testing.T) {
	cli(t)
	path := writeTemp(t, "notas.txt", "Primera nota\nCuerpo de la primera.\n---\nSegunda nota\nCuerpo de la segunda.\n")

	out, code := capture(t, func() int { return runSeed([]string{path, "--project=txt", "--kind=constraint"}) })
	if code != 0 {
		t.Fatalf("seed exited %d: %s", code, out)
	}
	if !strings.Contains(out, "cut on `---` rules") {
		t.Errorf("plain text should report the rule-based cut:\n%s", out)
	}
	if !strings.Contains(out, "2 created") {
		t.Errorf("expected 2 memories:\n%s", out)
	}
	if !strings.Contains(out, "constraint") {
		t.Errorf("--kind was not applied:\n%s", out)
	}
}

// Every bad flag must cost exit 2 and an untouched store. This is the class of
// bug the shared parser exists to close, so seed has to be held to it too.
func TestSeedRejectsBadArguments(t *testing.T) {
	cli(t)
	good := writeTemp(t, "d.md", seedDoc)

	cases := map[string][]string{
		"unknown flag":       {good, "--verbose"},
		"misspelled project": {good, "--porject=x"},
		"space-separated":    {good, "--kind", "decision"},
		"invalid kind":       {good, "--kind=decisionn"},
		"split too deep":     {good, "--split=9"},
		"split not a number": {good, "--split=abc"},
		"max-tokens zero":    {good, "--max-tokens=0"},
		"max-tokens garbage": {good, "--max-tokens=lots"},
		"two files":          {good, good},
		"no file":            {},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			out, code := capture(t, func() int { return runSeed(args) })
			if code != 2 {
				t.Fatalf("exit %d, want 2 — %s was accepted: %s", code, name, out)
			}
		})
	}
}

func TestSeedMissingFileFailsCleanly(t *testing.T) {
	cli(t)
	out, code := capture(t, func() int { return runSeed([]string{filepath.Join(t.TempDir(), "nope.md")}) })
	if code != 1 {
		t.Errorf("exit %d, want 1 (it ran and failed, not a usage error): %s", code, out)
	}
}

func TestSeedEmptyDocumentWritesNothing(t *testing.T) {
	cli(t)
	path := writeTemp(t, "empty.md", "\n\n   \n")

	out, code := capture(t, func() int { return runSeed([]string{path, "--project=empty"}) })
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !strings.Contains(out, "no memories") {
		t.Errorf("expected an explicit 'nothing to do', got:\n%s", out)
	}
}

// An oversized memory with nothing to split on is stored, and the operator is
// told. Silence here would hand back a store with a row recall can never use.
func TestSeedFlagsAnOversizedMemory(t *testing.T) {
	cli(t)
	wall := strings.Repeat("una parrafada larga sin subtítulos que no se puede partir. ", 40)
	path := writeTemp(t, "muro.md", "## Muro\n\n"+wall+"\n")

	out, code := capture(t, func() int {
		return runSeed([]string{path, "--project=big", "--max-tokens=50", "--dry-run"})
	})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !strings.Contains(out, "exceed the size limit") {
		t.Errorf("an oversized memory must be reported, got:\n%s", out)
	}
}
