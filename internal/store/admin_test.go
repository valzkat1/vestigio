package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "admin.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mustRemember(t *testing.T, s *Store, project, kind, title, body string) int64 {
	t.Helper()
	id, _, err := s.Remember(project, kind, title, body)
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	return id
}

func TestProjects(t *testing.T) {
	s := newTestStore(t)
	mustRemember(t, s, "alpha", "decision", "one", "body one")
	mustRemember(t, s, "alpha", "bugfix", "two", "body two")
	mustRemember(t, s, "beta", "pattern", "three", "body three")

	got, err := s.Projects()
	if err != nil {
		t.Fatalf("projects: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d projects, want 2", len(got))
	}
	// Ordered by count descending.
	if got[0].Project != "alpha" || got[0].Count != 2 {
		t.Errorf("first = %+v, want alpha with 2", got[0])
	}
	if got[0].Tokens == 0 {
		t.Error("tokens should be summed, got 0")
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)
	mustRemember(t, s, "alpha", "decision", "a1", "body a1")
	mustRemember(t, s, "alpha", "bugfix", "a2", "body a2")
	mustRemember(t, s, "beta", "decision", "b1", "body b1")

	tests := []struct {
		name    string
		project string
		kind    string
		limit   int
		want    int
	}{
		{"all projects", "", "", 30, 3},
		{"by project", "alpha", "", 30, 2},
		{"by kind", "", "decision", 30, 2},
		{"project and kind", "alpha", "decision", 30, 1},
		{"limit applies", "", "", 2, 2},
		{"zero limit defaults", "", "", 0, 3},
		{"no match", "gamma", "", 30, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.List(tt.project, tt.kind, tt.limit)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("got %d rows, want %d", len(got), tt.want)
			}
		})
	}
}

func TestListNewestFirst(t *testing.T) {
	s := newTestStore(t)
	first := mustRemember(t, s, "alpha", "decision", "older", "body older")
	last := mustRemember(t, s, "alpha", "decision", "newer", "body newer")

	got, err := s.List("alpha", "", 30)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got[0].ID != last || got[1].ID != first {
		t.Errorf("order = [%d %d], want [%d %d]", got[0].ID, got[1].ID, last, first)
	}
}

func TestGet(t *testing.T) {
	s := newTestStore(t)
	id := mustRemember(t, s, "alpha", "decision", "title here", "body here")

	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want a memory")
	}
	if got.Title != "title here" || got.Project != "alpha" {
		t.Errorf("got %+v", got.Memory)
	}
	if got.Hash == "" || got.CreatedAt == 0 || got.UpdatedAt == 0 {
		t.Errorf("bookkeeping columns not populated: %+v", got)
	}

	// Missing id is not an error — it is an absence.
	missing, err := s.Get(9999)
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	if missing != nil {
		t.Errorf("got %+v, want nil", missing)
	}
}

// The point of Update: the derived columns must follow the content.
func TestUpdateRecomputesDerivedColumns(t *testing.T) {
	s := newTestStore(t)
	id := mustRemember(t, s, "alpha", "decision", "original", "short")
	before, _ := s.Get(id)

	after, err := s.Update(id, "", "", "a considerably longer body than the original one")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if after.Hash == before.Hash {
		t.Error("hash did not change after body edit — dedupe would break")
	}
	if after.Tokens <= before.Tokens {
		t.Errorf("tokens = %d, want more than %d", after.Tokens, before.Tokens)
	}
	if after.Title != "original" {
		t.Errorf("title = %q, want it preserved", after.Title)
	}
	if after.CreatedAt != before.CreatedAt {
		t.Error("created_at must not move on edit")
	}
}

// A repaired row must become findable again through Remember's dedupe path.
func TestUpdateKeepsDedupeWorking(t *testing.T) {
	s := newTestStore(t)
	id := mustRemember(t, s, "alpha", "decision", "title", "first body")

	if _, err := s.Update(id, "", "", "second body"); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Remembering the edited content must UPDATE the same row, not insert a copy.
	gotID, created, err := s.Remember("alpha", "decision", "title", "second body")
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	if created {
		t.Error("created a duplicate — the hash was not recomputed")
	}
	if gotID != id {
		t.Errorf("id = %d, want %d", gotID, id)
	}
}

func TestUpdateFTSStaysSearchable(t *testing.T) {
	s := newTestStore(t)
	id := mustRemember(t, s, "alpha", "decision", "title", "kubernetes deployment")

	if _, err := s.Update(id, "", "", "postgres replication lag"); err != nil {
		t.Fatalf("update: %v", err)
	}

	hits, err := s.Search("alpha", "replication", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != id {
		t.Errorf("new content not searchable: %+v", hits)
	}

	if stale, _ := s.Search("alpha", "kubernetes", 10); len(stale) != 0 {
		t.Errorf("old content still indexed: %+v", stale)
	}
}

func TestUpdateRejectsCollision(t *testing.T) {
	s := newTestStore(t)
	keep := mustRemember(t, s, "alpha", "decision", "same title", "same body")
	other := mustRemember(t, s, "alpha", "decision", "other title", "other body")

	_, err := s.Update(other, "same title", "", "same body")
	var collision *ErrHashCollision
	if !errors.As(err, &collision) {
		t.Fatalf("err = %v, want ErrHashCollision", err)
	}
	if collision.ExistingID != keep {
		t.Errorf("ExistingID = %d, want %d", collision.ExistingID, keep)
	}
}

// The same content in a DIFFERENT project is not a collision — dedupe is scoped.
func TestUpdateAllowsSameContentAcrossProjects(t *testing.T) {
	s := newTestStore(t)
	mustRemember(t, s, "alpha", "decision", "shared", "shared body")
	id := mustRemember(t, s, "beta", "decision", "other", "other body")

	if _, err := s.Update(id, "shared", "", "shared body"); err != nil {
		t.Errorf("update across projects: %v", err)
	}
}

func TestUpdateValidation(t *testing.T) {
	s := newTestStore(t)
	id := mustRemember(t, s, "alpha", "decision", "title", "body")

	if _, err := s.Update(id, "", "nonsense", ""); err == nil {
		t.Error("invalid kind accepted")
	}
	if _, err := s.Update(9999, "x", "", ""); err == nil {
		t.Error("missing id accepted")
	}
}

func TestVerify(t *testing.T) {
	s := newTestStore(t)
	id := mustRemember(t, s, "alpha", "decision", "title", "body")

	if drift, err := s.Verify(); err != nil || len(drift) != 0 {
		t.Fatalf("clean store reported drift: %v %v", drift, err)
	}

	// Simulate what a raw SQL prompt or a GUI browser does: change the content
	// and leave the derived columns behind.
	if _, err := s.db.Exec(`UPDATE memories SET body = ? WHERE id = ?`, "tampered", id); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	drift, err := s.Verify()
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(drift) != 1 || drift[0].ID != id || !drift[0].HashStale {
		t.Fatalf("drift = %+v, want stale hash on #%d", drift, id)
	}

	// A no-op Update is the repair path.
	if _, err := s.Update(id, "", "", ""); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if after, _ := s.Verify(); len(after) != 0 {
		t.Errorf("still drifting after repair: %+v", after)
	}
}
