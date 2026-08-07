package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Detail is a Memory plus the bookkeeping columns. Memory stays small because it
// is what crosses the MCP boundary and every field there is context paid for in
// every session; Detail is for the CLI, where a human is reading and nobody is
// paying per token.
type Detail struct {
	Memory
	Hash        string `json:"hash"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
	RecalledAt  int64  `json:"recalled_at,omitempty"`
	RecallCount int    `json:"recall_count"`
}

// Stat is one row of the project inventory.
type Stat struct {
	Project string `json:"project"`
	Count   int    `json:"count"`
	Tokens  int    `json:"tokens"`
}

const detailCols = `id, project, kind, title, body, tokens, hash,
	created_at, updated_at, COALESCE(recalled_at, 0), recall_count`

func scanDetail(sc interface{ Scan(...any) error }) (Detail, error) {
	var d Detail
	err := sc.Scan(&d.ID, &d.Project, &d.Kind, &d.Title, &d.Body, &d.Tokens,
		&d.Hash, &d.CreatedAt, &d.UpdatedAt, &d.RecalledAt, &d.RecallCount)
	return d, err
}

// Projects returns the inventory, largest first.
//
// This exists because project detection is derived, not stored: it comes from the
// git remote, or the directory name, or VESTIGIO_PROJECT. Renaming a repository
// or adding a remote silently changes the key everything is filed under, and the
// only symptom is recall going quiet. Being able to see every key at once is how
// that gets caught.
func (s *Store) Projects() ([]Stat, error) {
	rows, err := s.db.Query(
		`SELECT project, COUNT(*), COALESCE(SUM(tokens), 0)
		   FROM memories GROUP BY project ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, fmt.Errorf("projects: %w", err)
	}
	defer rows.Close()

	var out []Stat
	for rows.Next() {
		var st Stat
		if err := rows.Scan(&st.Project, &st.Count, &st.Tokens); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// List returns memories newest first. An empty project means every project, and
// an empty kind means every kind.
func (s *Store) List(project, kind string, limit int) ([]Detail, error) {
	if limit <= 0 {
		limit = 30
	}
	q := `SELECT ` + detailCols + ` FROM memories`
	args := []any{}
	where := ""
	if project != "" {
		where, args = " WHERE project = ?", append(args, project)
	}
	if kind != "" {
		if where == "" {
			where = " WHERE kind = ?"
		} else {
			where += " AND kind = ?"
		}
		args = append(args, kind)
	}
	args = append(args, limit)

	rows, err := s.db.Query(q+where+` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()

	var out []Detail
	for rows.Next() {
		d, err := scanDetail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Get fetches one memory by id, across every project.
//
// Deliberately NOT project-scoped, unlike ForgetID. An id is a primary key and is
// already unique database-wide; scoping it would mean that running the CLI from
// the wrong directory reports "not found" for a row that plainly exists — the
// same silent-wrong-project failure the project detection comment warns about.
// The project is printed instead, so the operator sees what they are touching.
func (s *Store) Get(id int64) (*Detail, error) {
	d, err := scanDetail(s.db.QueryRow(`SELECT `+detailCols+` FROM memories WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	return &d, nil
}

// ErrHashCollision reports that the edited content already exists in the project.
type ErrHashCollision struct{ ExistingID int64 }

func (e *ErrHashCollision) Error() string {
	return fmt.Sprintf("content already stored as memory #%d in this project", e.ExistingID)
}

// Update rewrites a memory and recomputes the derived columns.
//
// This is the whole reason the CLI needs to own editing instead of handing people
// a SQL prompt. The FTS triggers keep the index in sync on their own, so a raw
// UPDATE looks like it worked — but `hash` and `tokens` are computed in Go at
// write time and no trigger touches them. A stale hash silently breaks dedupe:
// Remember() stops matching the row and inserts a near-copy instead of updating
// it. A stale token count makes the recall budget ceiling a lie.
//
// Empty title, kind or body mean "keep the current value". Passing none of them
// is therefore a no-op edit that still recomputes hash and tokens, which is how a
// row that drifted gets repaired.
func (s *Store) Update(id int64, title, kind, body string) (*Detail, error) {
	cur, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, fmt.Errorf("no memory with id %d", id)
	}

	if title == "" {
		title = cur.Title
	}
	if body == "" {
		body = cur.Body
	}
	if kind == "" {
		kind = cur.Kind
	}
	if !Kinds[kind] {
		return nil, fmt.Errorf("invalid kind %q", kind)
	}

	h := contentHash(title, body)
	tok := EstimateTokens(title) + EstimateTokens(body)

	// Two rows sharing (project, hash) makes Remember()'s dedupe lookup
	// non-deterministic: it takes whichever row the index returns first and the
	// other becomes unreachable through the normal write path.
	var other int64
	err = s.db.QueryRow(
		`SELECT id FROM memories WHERE project = ? AND hash = ? AND id != ?`,
		cur.Project, h, id,
	).Scan(&other)
	switch {
	case err == nil:
		return nil, &ErrHashCollision{ExistingID: other}
	case err != sql.ErrNoRows:
		return nil, fmt.Errorf("collision check: %w", err)
	}

	if _, err := s.db.Exec(
		`UPDATE memories SET title = ?, kind = ?, body = ?, tokens = ?, hash = ?, updated_at = ?
		  WHERE id = ?`,
		title, kind, body, tok, h, time.Now().Unix(), id,
	); err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	return s.Get(id)
}

// Drift is one row whose derived columns no longer match its content.
type Drift struct {
	ID         int64
	HashStale  bool
	TokensHave int
	TokensWant int
}

// Verify recomputes hash and tokens for every row and reports the mismatches.
//
// Anything that writes to the database without going through Remember/Import —
// a SQL prompt, a GUI browser, a restored backup from an older schema — leaves
// exactly this kind of damage, and it is invisible until dedupe misbehaves.
func (s *Store) Verify() ([]Drift, error) {
	rows, err := s.db.Query(`SELECT id, title, body, hash, tokens FROM memories`)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}
	defer rows.Close()

	var out []Drift
	for rows.Next() {
		var (
			id        int64
			ti, bo, h string
			tok       int
		)
		if err := rows.Scan(&id, &ti, &bo, &h, &tok); err != nil {
			return nil, err
		}
		want := EstimateTokens(ti) + EstimateTokens(bo)
		if hs, ts := contentHash(ti, bo) != h, want != tok; hs || ts {
			out = append(out, Drift{ID: id, HashStale: hs, TokensHave: tok, TokensWant: want})
		}
	}
	return out, rows.Err()
}
