package importer

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/valzkat1/vestigio/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "import.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestKindMappingCollapsesElevenTypesIntoFive(t *testing.T) {
	cases := map[string]string{
		"decision":     "decision",
		"architecture": "decision",
		"bugfix":       "bugfix",
		"pattern":      "pattern",
		"preference":   "constraint",
		"feedback":     "constraint",
		"config":       "constraint",
		"discovery":    "reference",
		"manual":       "reference",
	}
	for in, want := range cases {
		if got := kindMap[in]; got != want {
			t.Errorf("kindMap[%q] = %q, want %q", in, got, want)
		}
	}
	// An unmapped type must not vanish — it lands on reference at import time.
	if _, exists := kindMap["session_summary"]; exists {
		t.Error("session_summary should be unmapped so it falls back to reference")
	}
}

// Project drift is the reason recall silently misses: the same repository stored
// under three names is three disconnected memories.
func TestCanonicalProjectAppliesRenameMap(t *testing.T) {
	m := map[string]string{"alcubo-backend": "alcubo", "reposa2censo": "alcubo"}
	cases := []struct{ in, want string }{
		{"alcubo-backend", "alcubo"},
		{"reposa2censo", "alcubo"},
		{"ALCUBO-BACKEND", "alcubo"}, // case must not defeat consolidation
		{"untouched", "untouched"},
		{"", "default"},
	}
	for _, c := range cases {
		got := canonicalProject(Observation{Project: c.in}, m)
		if got != c.want {
			t.Errorf("canonicalProject(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Personal memories follow the person, not whichever repository happened to be
// open when they were captured.
func TestPersonalScopeOverridesProject(t *testing.T) {
	got := canonicalProject(Observation{Project: "alcubo", Scope: "personal"}, nil)
	if got != "personal" {
		t.Errorf("personal scope should override project, got %q", got)
	}
}

func TestSkipTypesAndEmptyContent(t *testing.T) {
	e := &Export{Observations: []Observation{
		{Type: "decision", Title: "Kept", Content: "body", Project: "p"},
		{Type: "session_summary", Title: "Dropped", Content: "long narrative", Project: "p"},
		{Type: "discovery", Title: "Empty body", Content: "   ", Project: "p"},
	}}
	r := Run(newStore(t), e, Options{SkipTypes: map[string]bool{"session_summary": true}})

	if r.Imported != 1 {
		t.Errorf("expected 1 imported, got %d", r.Imported)
	}
	if r.Skipped["session_summary"] != 1 {
		t.Errorf("session_summary was not skipped: %+v", r.Skipped)
	}
	if r.Skipped["(vacío)"] != 1 {
		t.Errorf("empty content was not skipped: %+v", r.Skipped)
	}
}

// Re-running a migration must be safe. Without this, every retry doubles the corpus.
func TestImportIsIdempotent(t *testing.T) {
	st := newStore(t)
	e := &Export{Observations: []Observation{
		{Type: "decision", Title: "Same", Content: "same body", Project: "p"},
	}}

	first := Run(st, e, Options{})
	second := Run(st, e, Options{})

	if first.Imported != 1 || second.Imported != 0 || second.Duplicates != 1 {
		t.Errorf("re-import must dedupe: first=%d second=%d dups=%d",
			first.Imported, second.Imported, second.Duplicates)
	}
}

// Migration must not rewrite history: an April decision is evidence about April.
func TestOriginalTimestampsArePreserved(t *testing.T) {
	want := time.Date(2026, 4, 7, 18, 16, 0, 0, time.UTC)
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if got := parseTime(want.Format(layout)); got != want.Unix() {
			t.Errorf("parseTime(%s) = %d, want %d", layout, got, want.Unix())
		}
	}
	if parseTime("") == 0 {
		t.Error("a missing date must fall back to now, not zero")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	st := newStore(t)
	e := &Export{Observations: []Observation{
		{Type: "decision", Title: "Planned", Content: "body", Project: "p"},
	}}

	if r := Run(st, e, Options{DryRun: true}); r.Imported != 1 {
		t.Errorf("dry run should report 1 planned, got %d", r.Imported)
	}
	if hits, _ := st.Search("p", "Planned", 5); len(hits) != 0 {
		t.Error("dry run wrote to the database")
	}
}
