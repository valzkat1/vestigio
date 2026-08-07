package store

import (
	"path/filepath"
	"testing"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// FTS5 MATCH is a query language, not a LIKE pattern. Unquoted user input
// containing an operator character is a syntax error, so a query like
// "fix auth-bug" would fail rather than simply miss.
func TestSanitizeFTSQuotesTermsAndOrsThem(t *testing.T) {
	cases := []struct{ in, want string }{
		{"go bm25", `"go" OR "bm25"`},
		{"fix auth-bug", `"fix" OR "auth-bug"`},
		{`say "hello"`, `"say" OR "hello"`},
		{"  spaced   out  ", `"spaced" OR "out"`},
		{"", ""},
	}
	for _, c := range cases {
		if got := SanitizeFTS(c.in); got != c.want {
			t.Errorf("SanitizeFTS(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRememberThenRecall(t *testing.T) {
	s := open(t)

	if _, created, err := s.Remember("proj", "decision", "Chose Go for the binary",
		"Static binary, no CGO, fast cold start."); err != nil || !created {
		t.Fatalf("remember: created=%v err=%v", created, err)
	}

	hits, err := s.Search("proj", "static binary", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Tokens == 0 {
		t.Error("tokens must be precomputed at write time — the M2 packer cannot count at query time")
	}
}

// Identical content must update in place. Without this, re-saving the same fact
// across sessions grows the store and splits recall across near-copies.
func TestRememberDedupesExactContent(t *testing.T) {
	s := open(t)

	id1, created1, _ := s.Remember("proj", "pattern", "Same title", "Same body")
	id2, created2, _ := s.Remember("proj", "pattern", "Same title", "Same body")

	if !created1 || created2 {
		t.Errorf("first save must create and second must merge; got created1=%v created2=%v", created1, created2)
	}
	if id1 != id2 {
		t.Errorf("dedupe must reuse the row: %d vs %d", id1, id2)
	}
}

// Memory is scoped per project — leaking across projects is worse than missing.
func TestSearchIsProjectScoped(t *testing.T) {
	s := open(t)
	s.Remember("alpha", "reference", "Alpha note", "shared keyword here")
	s.Remember("beta", "reference", "Beta note", "shared keyword here")

	hits, _ := s.Search("alpha", "shared keyword", 10)
	if len(hits) != 1 {
		t.Fatalf("expected only alpha's memory, got %d", len(hits))
	}
	if hits[0].Title != "Alpha note" {
		t.Errorf("wrong project's memory returned: %q", hits[0].Title)
	}
}

func TestForgetByID(t *testing.T) {
	s := open(t)
	id, _, _ := s.Remember("proj", "bugfix", "Temporary", "to be removed")

	n, err := s.ForgetID("proj", id)
	if err != nil || n != 1 {
		t.Fatalf("forget: n=%d err=%v", n, err)
	}
	if hits, _ := s.Search("proj", "temporary", 10); len(hits) != 0 {
		t.Error("deleted memory still present in the FTS index — check the delete trigger")
	}
}

// Regression: reads are generous, deletes are strict.
//
// With recall's OR semantics, forgetting "FTS5 syntax" also deleted a memory
// about Go that merely mentioned FTS5 in passing. There is no undo, so a delete
// must match every term the caller supplied.
func TestForgetQueryRequiresAllTerms(t *testing.T) {
	s := open(t)
	s.Remember("proj", "decision", "Chose Go", "Pure Go sqlite gives FTS5 with no CGO")
	s.Remember("proj", "bugfix", "FTS5 syntax trap", "Quote every term before MATCH")

	n, err := s.ForgetQuery("proj", "FTS5 syntax")
	if err != nil {
		t.Fatalf("forget: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 deletion, got %d — OR semantics on a delete is data loss", n)
	}
	if hits, _ := s.Search("proj", "Chose Go", 10); len(hits) != 1 {
		t.Error("the unrelated memory was deleted")
	}
}

func TestUnknownKindFallsBackToReference(t *testing.T) {
	s := open(t)
	s.Remember("proj", "not-a-kind", "Title", "Body")

	hits, _ := s.Search("proj", "title", 10)
	if len(hits) != 1 || hits[0].Kind != "reference" {
		t.Errorf("unknown kind should fall back to reference, got %+v", hits)
	}
}
