// Package cost provides a lightweight SQLite-backed store for tracking
// LLM session costs as a sidecar to the main beads database.
package cost

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

const (
	dbFile        = "cost.db"
	schemaVersion = 1
)

// Session represents a single LLM interaction session with cost data.
type Session struct {
	ID            string
	BeadID        string // optional: associated bead/issue ID
	Model         string
	Provider      string // e.g. "anthropic", "openai"
	InputTokens   int64
	OutputTokens  int64
	CostUSD       float64 // total cost in USD
	DurationSec   float64 // wall-clock seconds
	Note          string  // free-form annotation
	CreatedAt     time.Time
	CacheRead     int64 // cache read tokens (optional)
	CacheCreation int64 // cache creation tokens (optional)
}

// Summary holds aggregated cost statistics.
type Summary struct {
	TotalSessions int
	TotalCostUSD  float64
	TotalInput    int64
	TotalOutput   int64
	TotalDuration float64
	FirstSession  time.Time
	LastSession   time.Time
}

// Store manages the cost.db SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the cost database inside the given beads directory.
// beadsDir is typically ".beads" relative to the repo root.
func Open(beadsDir string) (*Store, error) {
	dbPath := filepath.Join(beadsDir, dbFile)

	// Ensure parent directory exists.
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		return nil, fmt.Errorf("cost: mkdir %s: %w", beadsDir, err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("cost: open %s: %w", dbPath, err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	// Create version table if not exists.
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("cost: create schema_version: %w", err)
	}

	var ver int
	err := s.db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&ver)
	if err == sql.ErrNoRows {
		ver = 0
	} else if err != nil {
		return fmt.Errorf("cost: read schema version: %w", err)
	}

	if ver < 1 {
		schema := `
CREATE TABLE IF NOT EXISTS sessions (
    id             TEXT PRIMARY KEY,
    bead_id        TEXT NOT NULL DEFAULT '',
    model          TEXT NOT NULL DEFAULT '',
    provider       TEXT NOT NULL DEFAULT '',
    input_tokens   INTEGER NOT NULL DEFAULT 0,
    output_tokens  INTEGER NOT NULL DEFAULT 0,
    cost_usd       REAL NOT NULL DEFAULT 0.0,
    duration_sec   REAL NOT NULL DEFAULT 0.0,
    note           TEXT NOT NULL DEFAULT '',
    cache_read     INTEGER NOT NULL DEFAULT 0,
    cache_creation INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_sessions_bead ON sessions(bead_id);
CREATE INDEX IF NOT EXISTS idx_sessions_created ON sessions(created_at);
`
		if _, err := s.db.Exec(schema); err != nil {
			return fmt.Errorf("cost: create sessions table: %w", err)
		}
	}

	// Upsert version.
	if _, err := s.db.Exec(`DELETE FROM schema_version`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, schemaVersion); err != nil {
		return err
	}
	return nil
}

// Track inserts a new session record.
func (s *Store) Track(sess Session) error {
	if sess.ID == "" {
		return fmt.Errorf("cost: session ID is required")
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`
		INSERT INTO sessions (id, bead_id, model, provider, input_tokens, output_tokens,
		                      cost_usd, duration_sec, note, cache_read, cache_creation, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.BeadID, sess.Model, sess.Provider,
		sess.InputTokens, sess.OutputTokens,
		sess.CostUSD, sess.DurationSec, sess.Note,
		sess.CacheRead, sess.CacheCreation,
		sess.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

// GetSession retrieves a single session by ID.
func (s *Store) GetSession(id string) (*Session, error) {
	row := s.db.QueryRow(`SELECT id, bead_id, model, provider, input_tokens, output_tokens,
		cost_usd, duration_sec, note, cache_read, cache_creation, created_at
		FROM sessions WHERE id = ?`, id)
	return scanSession(row)
}

// ListSessions returns sessions ordered by creation time (newest first), with an optional limit.
func (s *Store) ListSessions(limit int) ([]Session, error) {
	q := `SELECT id, bead_id, model, provider, input_tokens, output_tokens,
		cost_usd, duration_sec, note, cache_read, cache_creation, created_at
		FROM sessions ORDER BY created_at DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		sess, err := scanSessionRows(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *sess)
	}
	return sessions, rows.Err()
}

// SessionsForBead returns sessions associated with a given bead ID.
func (s *Store) SessionsForBead(beadID string) ([]Session, error) {
	rows, err := s.db.Query(`SELECT id, bead_id, model, provider, input_tokens, output_tokens,
		cost_usd, duration_sec, note, cache_read, cache_creation, created_at
		FROM sessions WHERE bead_id = ? ORDER BY created_at DESC`, beadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		sess, err := scanSessionRows(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *sess)
	}
	return sessions, rows.Err()
}

// Status returns an aggregate summary of all tracked sessions.
func (s *Store) Status() (*Summary, error) {
	row := s.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0), COALESCE(SUM(duration_sec), 0),
		       COALESCE(MIN(created_at), ''), COALESCE(MAX(created_at), '')
		FROM sessions`)

	var sum Summary
	var first, last string
	if err := row.Scan(&sum.TotalSessions, &sum.TotalCostUSD, &sum.TotalInput,
		&sum.TotalOutput, &sum.TotalDuration, &first, &last); err != nil {
		return nil, err
	}
	if first != "" {
		sum.FirstSession, _ = time.Parse(time.RFC3339, first)
	}
	if last != "" {
		sum.LastSession, _ = time.Parse(time.RFC3339, last)
	}
	return &sum, nil
}

// scanner interface for both *sql.Row and *sql.Rows
type scanner interface {
	Scan(dest ...any) error
}

func scanSession(s scanner) (*Session, error) {
	var sess Session
	var createdAt string
	if err := s.Scan(&sess.ID, &sess.BeadID, &sess.Model, &sess.Provider,
		&sess.InputTokens, &sess.OutputTokens, &sess.CostUSD, &sess.DurationSec,
		&sess.Note, &sess.CacheRead, &sess.CacheCreation, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	sess.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &sess, nil
}

func scanSessionRows(rows *sql.Rows) (*Session, error) {
	return scanSession(rows)
}
