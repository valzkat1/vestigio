package store

// schema is applied on every open. Every statement is idempotent.
//
// Two decisions here are direct consequences of measuring Engram in M0:
//
//   - wal_autocheckpoint: Engram's WAL had grown to 4.1 MB against a 2 MB database
//     because it never checkpointed. A memory store is read at the start of every
//     session, so that cost is paid constantly.
//   - porter stemming: M0's recall test missed a paraphrase partly on plain word
//     forms, not on meaning. Stemming closes that half for free.
const schema = `
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA wal_autocheckpoint = 512;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS memories (
  id              INTEGER PRIMARY KEY,
  project         TEXT    NOT NULL,
  kind            TEXT    NOT NULL DEFAULT 'reference',
  title           TEXT    NOT NULL,
  body            TEXT    NOT NULL,

  -- Precomputed at write time. The budget packer (M2) cannot afford to count
  -- tokens while serving a query.
  tokens          INTEGER NOT NULL DEFAULT 0,

  -- Exact-content hash in M1; becomes a simhash for near-duplicate upserts in M3.
  hash            TEXT    NOT NULL DEFAULT '',

  -- Vector-ready, deliberately unused in v1. Having the columns and the full body
  -- stored means enabling embeddings later is a backfill, not a migration.
  embedding       BLOB,
  embedding_model TEXT,

  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL,
  recalled_at     INTEGER,
  recall_count    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_memories_project ON memories(project);
CREATE INDEX IF NOT EXISTS idx_memories_hash    ON memories(project, hash);

CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
  title,
  body,
  content='memories',
  content_rowid='id',
  tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
  INSERT INTO memories_fts(rowid, title, body) VALUES (new.id, new.title, new.body);
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
  INSERT INTO memories_fts(memories_fts, rowid, title, body)
    VALUES ('delete', old.id, old.title, old.body);
END;

CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
  INSERT INTO memories_fts(memories_fts, rowid, title, body)
    VALUES ('delete', old.id, old.title, old.body);
  INSERT INTO memories_fts(rowid, title, body) VALUES (new.id, new.title, new.body);
END;
`
