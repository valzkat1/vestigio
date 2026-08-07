package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valzkat1/vestigio/internal/store"
	_ "modernc.org/sqlite"
)

// These drive the real CLI commands, not helpers around them: a temp database
// via VESTIGIO_DB, the actual run* functions, the actual exit codes. Before this
// file cmd/vestigio sat at 8.3% — almost every command had never been executed
// by a test, which is exactly where a silent-wrong-answer hides.

// cli points every command at a throwaway database and seeds it.
func cli(t *testing.T) *store.Store {
	t.Helper()
	db := filepath.Join(t.TempDir(), "cli.db")
	t.Setenv("VESTIGIO_DB", db)
	t.Setenv("VESTIGIO_PROJECT", "cliproj")

	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// capture swaps os.Stdout for a pipe and returns whatever fn printed.
func capture(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	prev := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()

	code := fn()
	w.Close()
	os.Stdout = prev
	return <-done, code
}

func seedCLI(t *testing.T, st *store.Store) {
	t.Helper()
	for _, m := range []struct{ kind, title, body string }{
		{"decision", "Elegido Go para el binario", "Binario estático, sin CGO."},
		{"bugfix", "Corregido el crash de FTS5", "MATCH es un lenguaje de consulta."},
		{"pattern", "Tres tools en el servidor MCP", "El perfil se paga en cada sesión."},
	} {
		if _, _, err := st.Remember("cliproj", m.kind, m.title, m.body); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

// --- helpers -----------------------------------------------------------------

func TestTruncateCutsOnRunes(t *testing.T) {
	// Byte-slicing would split the multibyte runes and emit replacement chars.
	long := strings.Repeat("ñ", 100)
	got := truncate(long, 10)
	if r := []rune(got); len(r) != 10 {
		t.Errorf("truncate(…, 10) produced %d runes (%q), want 10", len(r), got)
	}
	if strings.Contains(got, "\uFFFD") {
		t.Errorf("truncate mangled a multibyte rune: %q", got)
	}
	if got := truncate("corto", 60); got != "corto" {
		t.Errorf("truncate(short) = %q, want it untouched", got)
	}
	if got := truncate("una\nlinea", 60); strings.Contains(got, "\n") {
		t.Errorf("truncate must flatten newlines for tabular output, got %q", got)
	}
}

func TestParseIDAndPositionals(t *testing.T) {
	if id, err := parseID([]string{"--fix", "42"}); err != nil || id != 42 {
		t.Errorf("parseID(--fix 42) = %d, %v; want 42, nil — flag order must not matter", id, err)
	}
	for _, args := range [][]string{{}, {"--fix"}, {"abc"}, {"1.5"}} {
		if _, err := parseID(args); err == nil {
			t.Errorf("parseID(%q) = nil error, want an error", args)
		}
	}
}

func TestFlagValueAndHasFlag(t *testing.T) {
	args := []string{"--kind=decision", "--all", "--limit="}
	if v, ok := flagValue(args, "kind"); !ok || v != "decision" {
		t.Errorf("flagValue(kind) = %q, %v", v, ok)
	}
	if v, ok := flagValue(args, "limit"); !ok || v != "" {
		t.Errorf("flagValue(limit) = %q, %v; an empty inline value is still present", v, ok)
	}
	if _, ok := flagValue(args, "project"); ok {
		t.Error("flagValue(project) reported present")
	}
	if !hasFlag(args, "all") || hasFlag(args, "yes") {
		t.Error("hasFlag disagrees with the args")
	}
}

// --- commands ----------------------------------------------------------------

func TestRunProjectsAndList(t *testing.T) {
	st := cli(t)
	seedCLI(t, st)

	out, code := capture(t, func() int { return runProjects(nil) })
	if code != 0 || !strings.Contains(out, "cliproj") {
		t.Errorf("runProjects = %d, output:\n%s", code, out)
	}

	out, code = capture(t, func() int { return runList(nil) })
	if code != 0 {
		t.Fatalf("runList = %d", code)
	}
	for _, want := range []string{"Elegido Go", "3 memories", "cliproj"} {
		if !strings.Contains(out, want) {
			t.Errorf("runList output missing %q:\n%s", want, out)
		}
	}

	out, _ = capture(t, func() int { return runList([]string{"--kind=bugfix"}) })
	if strings.Contains(out, "Elegido Go") || !strings.Contains(out, "crash de FTS5") {
		t.Errorf("--kind=bugfix did not filter:\n%s", out)
	}
}

func TestRunListEmptyProjectExplainsItself(t *testing.T) {
	cli(t)
	out, code := capture(t, func() int { return runList([]string{"--project=nope"}) })
	if code != 0 {
		t.Fatalf("runList = %d", code)
	}
	if !strings.Contains(out, "nothing filed under") {
		t.Errorf("an empty result must say where to look next:\n%s", out)
	}
}

// A bad --limit must not be swallowed. This is the same failure the import
// parser was fixed for in ff99658: an argument the CLI does not understand
// quietly becoming a different behaviour than the one asked for.
func TestRunListRejectsUnparseableLimit(t *testing.T) {
	st := cli(t)
	seedCLI(t, st)

	for _, bad := range []string{"--limit=abc", "--limit=", "--limit=-1", "--limit=0"} {
		t.Run(bad, func(t *testing.T) {
			_, code := capture(t, func() int { return runList([]string{bad}) })
			if code == 0 {
				t.Errorf("runList(%q) returned 0 — an unusable limit was silently replaced by the default", bad)
			}
		})
	}
}

// A misspelled kind returns zero rows and says "0 memories", which reads as
// "nothing was ever saved" instead of "that is not a kind".
func TestRunListRejectsUnknownKind(t *testing.T) {
	st := cli(t)
	seedCLI(t, st)

	out, code := capture(t, func() int { return runList([]string{"--kind=decisionn"}) })
	if code == 0 {
		t.Errorf("runList(--kind=decisionn) returned 0 with output:\n%s\nwant a non-zero code naming the valid kinds", out)
	}
}

func TestRunShow(t *testing.T) {
	st := cli(t)
	seedCLI(t, st)

	out, code := capture(t, func() int { return runShow([]string{"1"}) })
	if code != 0 || !strings.Contains(out, "Elegido Go") || !strings.Contains(out, "cliproj") {
		t.Errorf("runShow(1) = %d, output:\n%s", code, out)
	}
	if _, code := capture(t, func() int { return runShow([]string{"999"}) }); code != 1 {
		t.Errorf("runShow(999) = %d, want 1", code)
	}
	if _, code := capture(t, func() int { return runShow(nil) }); code != 2 {
		t.Errorf("runShow() = %d, want 2 (usage error)", code)
	}
}

func TestRunRmRequiresConfirmationOrYes(t *testing.T) {
	st := cli(t)
	seedCLI(t, st)

	out, code := capture(t, func() int { return runRm([]string{"1", "--yes"}) })
	if code != 0 || !strings.Contains(out, "1 memory deleted") {
		t.Errorf("runRm --yes = %d, output:\n%s", code, out)
	}
	if _, code := capture(t, func() int { return runRm([]string{"1", "--yes"}) }); code != 1 {
		t.Errorf("deleting the same id twice = %d, want 1", code)
	}
	if _, code := capture(t, func() int { return runRm(nil) }); code != 2 {
		t.Errorf("runRm() = %d, want 2", code)
	}
}

func TestRunVerifyCleanStore(t *testing.T) {
	st := cli(t)
	seedCLI(t, st)

	out, code := capture(t, func() int { return runVerify(nil) })
	if code != 0 || !strings.Contains(out, "ok —") {
		t.Errorf("runVerify on a clean store = %d, output:\n%s", code, out)
	}
}

// corrupt writes straight to SQLite, which is exactly the scenario verify and
// `edit --fix` exist for: a row damaged by a SQL prompt or a GUI browser.
func corrupt(t *testing.T, db, setClause string, id int64) {
	t.Helper()
	conn, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`UPDATE memories SET `+setClause+` WHERE id = ?`, id); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
}

func TestRunVerifyReportsDriftAndEditFixRepairsIt(t *testing.T) {
	st := cli(t)
	seedCLI(t, st)
	db := os.Getenv("VESTIGIO_DB")
	st.Close()

	corrupt(t, db, `hash = 'deadbeef', tokens = 99999`, 1)

	out, code := capture(t, func() int { return runVerify(nil) })
	if code != 1 || !strings.Contains(out, "#1") {
		t.Fatalf("runVerify on a drifted row = %d, output:\n%s", code, out)
	}

	out, code = capture(t, func() int { return runEdit([]string{"1", "--fix"}) })
	if code != 0 {
		t.Fatalf("edit --fix = %d, output:\n%s", code, out)
	}

	out, code = capture(t, func() int { return runVerify(nil) })
	if code != 0 || !strings.Contains(out, "ok —") {
		t.Errorf("verify after --fix = %d, output:\n%s\nthe repair path must actually repair", code, out)
	}
}

// The documented repair path must survive the worst row it will ever meet.
// A hash blanked by a SQL prompt is not exotic: it is the case `verify` was
// written to catch.
func TestEditFixSurvivesAShortHash(t *testing.T) {
	st := cli(t)
	seedCLI(t, st)
	db := os.Getenv("VESTIGIO_DB")
	st.Close()

	corrupt(t, db, `hash = ''`, 1)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("edit --fix panicked on a row with an empty hash: %v", r)
		}
	}()
	if _, code := capture(t, func() int { return runEdit([]string{"1", "--fix"}) }); code != 0 {
		t.Errorf("edit --fix = %d, want 0", code)
	}
}

func TestRunEditRejectsBadInput(t *testing.T) {
	st := cli(t)
	seedCLI(t, st)

	if _, code := capture(t, func() int { return runEdit(nil) }); code != 2 {
		t.Error("runEdit() must be a usage error")
	}
	if _, code := capture(t, func() int { return runEdit([]string{"999", "--fix"}) }); code != 1 {
		t.Error("runEdit on a missing id must fail")
	}
	if _, code := capture(t, func() int { return runEdit([]string{"1", "--kind=nope"}) }); code != 1 {
		t.Error("runEdit with an invalid kind must fail")
	}
	if _, code := capture(t, func() int { return runEdit([]string{"1", "--body-file=/nope/missing"}) }); code != 1 {
		t.Error("runEdit with a missing body file must fail")
	}
}

func TestRunEditTitleAndKind(t *testing.T) {
	st := cli(t)
	seedCLI(t, st)

	out, code := capture(t, func() int {
		return runEdit([]string{"1", "--title=Elegido Go para el binario estático"})
	})
	if code != 0 || !strings.Contains(out, "#1 updated") {
		t.Fatalf("runEdit = %d, output:\n%s", code, out)
	}

	d, err := st.Get(1)
	if err != nil || d == nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(d.Title, "estático") {
		t.Errorf("title = %q, want the edited one", d.Title)
	}
	if d.Body != "Binario estático, sin CGO." {
		t.Errorf("body = %q — editing the title must not touch the body", d.Body)
	}
}
