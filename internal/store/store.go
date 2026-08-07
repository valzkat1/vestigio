package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Memory is one durable fact. Small on purpose: a memory that needs more than a
// few hundred tokens to state is two memories.
type Memory struct {
	ID      int64  `json:"id"`
	Project string `json:"project,omitempty"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Tokens  int    `json:"tokens"`
}

// Kinds is the closed set of memory types. Five, not fifteen — every extra enum
// value is schema text paid for in every session of every agent.
var Kinds = map[string]bool{
	"decision":   true,
	"bugfix":     true,
	"pattern":    true,
	"constraint": true,
	"reference":  true,
}

type Store struct{ db *sql.DB }

// DefaultPath resolves the database location, honouring VESTIGIO_DB.
func DefaultPath() string {
	if p := os.Getenv("VESTIGIO_DB"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "vestigio.db"
	}
	return filepath.Join(home, ".vestigio", "vestigio.db")
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// SQLite writes are serialized anyway; more connections only buys lock contention.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close checkpoints the WAL before closing. Skipping this is how Engram ended up
// with a WAL twice the size of its database.
func (s *Store) Close() error {
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return s.db.Close()
}

// Remember stores a fact. An exact content match within the same project updates
// the existing row instead of inserting a near-copy.
func (s *Store) Remember(project, kind, title, body string) (id int64, created bool, err error) {
	if !Kinds[kind] {
		kind = "reference"
	}
	now := time.Now().Unix()
	h := contentHash(title, body)
	tok := EstimateTokens(title) + EstimateTokens(body)

	var existing int64
	err = s.db.QueryRow(
		`SELECT id FROM memories WHERE project = ? AND hash = ?`, project, h,
	).Scan(&existing)

	switch {
	case err == nil:
		_, err = s.db.Exec(
			`UPDATE memories SET kind = ?, title = ?, body = ?, tokens = ?, updated_at = ? WHERE id = ?`,
			kind, title, body, tok, now, existing,
		)
		return existing, false, err
	case err != sql.ErrNoRows:
		return 0, false, fmt.Errorf("dedupe lookup: %w", err)
	}

	res, err := s.db.Exec(
		`INSERT INTO memories (project, kind, title, body, tokens, hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		project, kind, title, body, tok, h, now, now,
	)
	if err != nil {
		return 0, false, fmt.Errorf("insert: %w", err)
	}
	id, err = res.LastInsertId()
	return id, true, err
}

// Search runs BM25 over the FTS index. Lower bm25() is better, so results come
// back already ordered best-first.
func (s *Store) Search(project, query string, limit int) ([]Memory, error) {
	return s.search(project, SanitizeFTS(query), limit)
}

// SearchAll requires every term to be present. Used by destructive operations,
// where a generous match means deleting something the caller never named.
func (s *Store) SearchAll(project, query string, limit int) ([]Memory, error) {
	return s.search(project, SanitizeFTSAll(query), limit)
}

func (s *Store) search(project, match string, limit int) ([]Memory, error) {
	if match == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(
		`SELECT m.id, m.kind, m.title, m.body, m.tokens
		   FROM memories_fts f
		   JOIN memories m ON m.id = f.rowid
		  WHERE memories_fts MATCH ? AND m.project = ?
		  ORDER BY bm25(memories_fts) LIMIT ?`,
		match, project, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var out []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.Kind, &m.Title, &m.Body, &m.Tokens); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarkRecalled feeds the decay/eviction pass planned for M4. Failures here must
// never break a read.
func (s *Store) MarkRecalled(ids []int64) {
	now := time.Now().Unix()
	for _, id := range ids {
		_, _ = s.db.Exec(
			`UPDATE memories SET recalled_at = ?, recall_count = recall_count + 1 WHERE id = ?`,
			now, id,
		)
	}
}

func (s *Store) ForgetID(project string, id int64) (int, error) {
	res, err := s.db.Exec(`DELETE FROM memories WHERE id = ? AND project = ?`, id, project)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ForgetQuery deletes memories matching ALL terms in the query.
//
// Reads are generous and deletes are strict, on purpose. With the OR semantics
// that recall uses, forget("FTS5 syntax") would also delete every memory that
// merely mentions "syntax" — the caller never asked for that, and there is no
// undo. Caught by the M1 end-to-end run, where a two-word delete removed two
// unrelated memories.
func (s *Store) ForgetQuery(project, query string) (int, error) {
	hits, err := s.SearchAll(project, query, 50)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, h := range hits {
		c, err := s.ForgetID(project, h.ID)
		if err != nil {
			return n, err
		}
		n += c
	}
	return n, nil
}

// SanitizeFTS turns free text into a safe FTS5 MATCH expression.
//
// FTS5 MATCH is a query language, not a LIKE pattern: raw input containing "-",
// "*", ":" or quotes is a syntax error, not a miss. Every term is quoted, and
// terms are OR-ed rather than AND-ed — BM25 ranking sorts out precision, while
// AND would silently drop anything that isn't a full phrase match.
func SanitizeFTS(q string) string { return sanitize(q, " OR ") }

// SanitizeFTSAll is the strict variant: every term must be present.
func SanitizeFTSAll(q string) string { return sanitize(q, " AND ") }

func sanitize(q, join string) string {
	fields := strings.Fields(q)
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ReplaceAll(f, `"`, "")
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		terms = append(terms, `"`+f+`"`)
	}
	return strings.Join(terms, join)
}

// EstimateTokens approximates token count at 4 characters per token.
//
// This is the same ratio used for the M0 baseline, kept deliberately so the
// before/after comparison stays apples-to-apples. It is an estimate, not a
// measurement — swap in a real tokenizer before quoting these numbers publicly.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len([]rune(s)) + 3) / 4
}

func contentHash(title, body string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(title) + "\x00" + strings.TrimSpace(body)))
	return hex.EncodeToString(sum[:16])
}
