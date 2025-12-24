package agg

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/victhorio/opa/agg/core"
	_ "modernc.org/sqlite"
)

// SQLiteStore implements the Store interface with SQLite persistence.
// It uses an embedded EphemeralStore as a cache to reduce database reads
// during active conversations.
type SQLiteStore struct {
	db        *sql.DB
	ephemeral *EphemeralStore
	mu        sync.RWMutex
}

// NewSQLiteStore creates a new SQLite-backed store.
// The path parameter can be a file path or ":memory:" for an in-memory database.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	// Create parent directories if needed for file-based databases
	if path != ":memory:" {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Open database connection
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrent access
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Initialize schema
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	eph := NewEphemeralStore()
	return &SQLiteStore{
		db:        db,
		ephemeral: &eph,
	}, nil
}

// initSchema creates the necessary tables if they don't exist.
func initSchema(db *sql.DB) error {
	schema := `
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			payload BLOB NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_messages_session_id_id
			ON messages(session_id, id);

		CREATE TABLE IF NOT EXISTS usage (
			session_id TEXT PRIMARY KEY,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			cached_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			reasoning_tokens INTEGER NOT NULL DEFAULT 0,
			cost INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Messages returns all messages for a given session.
// It uses the ephemeral cache if the session has already been loaded.
func (s *SQLiteStore) Messages(sessionID string) []*core.Msg {
	s.mu.RLock()
	msgs := s.ephemeral.Messages(sessionID)
	if len(msgs) > 0 {
		defer s.mu.RUnlock()
		return msgs
	}
	s.mu.RUnlock()

	// Load from database
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	msgs = s.ephemeral.Messages(sessionID)
	if len(msgs) > 0 {
		return msgs
	}

	msgs, err := s.loadMessages(sessionID)
	if err != nil {
		// Log error but return empty slice to maintain interface contract
		fmt.Fprintf(os.Stderr, "failed to load messages for session %s: %v\n", sessionID, err)
		return []*core.Msg{}
	}

	// Let's also load the usage so that we can populate the ephemeral session with all relevant
	// data.
	usage, err := s.loadUsage(sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load usage for session %s: %v\n", sessionID, err)
		// This is concerning, since we've managed to load the messages. Even though we managed to
		// load the messages, since we've failed to load the usage, we'd be populating the
		// ephemeral cache with a session that has no usage data. This would lead to incorrect
		// accounting, which we cannot allow.
		return []*core.Msg{}
	}

	// Populate ephemeral cache
	if err := s.ephemeral.Extend(sessionID, msgs, usage); err != nil {
		fmt.Fprintf(os.Stderr, "failed to populate ephemeral cache: %v\n", err)
	}

	return msgs
}

// Usage returns the accumulated usage for a given session.
// It uses the ephemeral cache if the session has already been loaded.
// Note: Unlike Messages(), this method does not populate the ephemeral cache when loading from DB.
// This is intentional to avoid partial cache states. The cache is only populated via Messages()
// or Extend(), which ensure both messages and usage are loaded together.
func (s *SQLiteStore) Usage(sessionID string) core.Usage {
	s.mu.RLock()
	usage := s.ephemeral.Usage(sessionID)
	// Check if usage has been loaded. Any valid usage will either include non-zero Input or Cost.
	if usage.Input != 0 || usage.Cost != 0 {
		defer s.mu.RUnlock()
		return usage
	}
	s.mu.RUnlock()

	// Load from database
	usage, err := s.loadUsage(sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load usage for session %s: %v\n", sessionID, err)
		return core.Usage{}
	}

	return usage
}

// Extend appends messages and accumulates usage for a session.
// It writes through to both the ephemeral cache and SQLite.
func (s *SQLiteStore) Extend(sessionID string, msgs []*core.Msg, usage core.Usage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Persist to SQLite first to ensure DB is the source of truth
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert messages
	stmt, err := tx.Prepare("INSERT INTO messages (session_id, payload) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, msg := range msgs {
		payload, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to serialize message: %w", err)
		}

		if _, err := stmt.Exec(sessionID, payload); err != nil {
			return fmt.Errorf("failed to insert message: %w", err)
		}
	}

	// Upsert usage with accumulation
	_, err = tx.Exec(`
		INSERT INTO usage (session_id, input_tokens, cached_tokens, output_tokens, reasoning_tokens, cost)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			input_tokens = usage.input_tokens + excluded.input_tokens,
			cached_tokens = usage.cached_tokens + excluded.cached_tokens,
			output_tokens = usage.output_tokens + excluded.output_tokens,
			reasoning_tokens = usage.reasoning_tokens + excluded.reasoning_tokens,
			cost = usage.cost + excluded.cost
	`, sessionID, usage.Input, usage.Cached, usage.Output, usage.Reasoning, usage.Cost)
	if err != nil {
		return fmt.Errorf("failed to upsert usage: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Only update ephemeral cache after successful persistence
	if err := s.ephemeral.Extend(sessionID, msgs, usage); err != nil {
		return fmt.Errorf("failed to update ephemeral cache: %w", err)
	}

	return nil
}

// loadMessages loads all messages for a session from the database.
func (s *SQLiteStore) loadMessages(sessionID string) ([]*core.Msg, error) {
	rows, err := s.db.Query(
		"SELECT payload FROM messages WHERE session_id = ? ORDER BY id ASC",
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var msgs []*core.Msg
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}

		var msg core.Msg
		if err := json.Unmarshal(payload, &msg); err != nil {
			return nil, fmt.Errorf("failed to deserialize message: %w", err)
		}

		msgs = append(msgs, &msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return msgs, nil
}

// loadUsage loads usage data for a session from the database.
func (s *SQLiteStore) loadUsage(sessionID string) (core.Usage, error) {
	var usage core.Usage
	err := s.db.QueryRow(`
		SELECT input_tokens, cached_tokens, output_tokens, reasoning_tokens, cost
		FROM usage
		WHERE session_id = ?
	`, sessionID).Scan(&usage.Input, &usage.Cached, &usage.Output, &usage.Reasoning, &usage.Cost)

	if err == sql.ErrNoRows {
		// No usage data found, return zero-valued Usage
		return core.Usage{}, nil
	}

	if err != nil {
		return core.Usage{}, fmt.Errorf("failed to query usage: %w", err)
	}

	// Recompute total
	usage.Total = usage.Input + usage.Cached + usage.Output

	return usage, nil
}
