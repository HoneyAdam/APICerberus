package raft

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// SQLiteStorage implements the Storage interface using SQLite.
type SQLiteStorage struct {
	db *sql.DB
}

// NewSQLiteStorage creates a new SQLite-backed Raft storage.
func NewSQLiteStorage(db *sql.DB) (*SQLiteStorage, error) {
	s := &SQLiteStorage{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("raft storage migration: %w", err)
	}
	return s, nil
}

func (s *SQLiteStorage) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS raft_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			current_term INTEGER NOT NULL DEFAULT 0,
			voted_for TEXT NOT NULL DEFAULT ''
		)`,
		`INSERT OR IGNORE INTO raft_state (id, current_term, voted_for) VALUES (1, 0, '')`,
		`CREATE TABLE IF NOT EXISTS raft_log (
			idx INTEGER PRIMARY KEY,
			term INTEGER NOT NULL,
			command BLOB
		)`,
		`CREATE TABLE IF NOT EXISTS raft_snapshot (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			last_index INTEGER NOT NULL DEFAULT 0,
			last_term INTEGER NOT NULL DEFAULT 0,
			data BLOB
		)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// SaveState persists the current term and voted-for to stable storage.
func (s *SQLiteStorage) SaveState(term uint64, votedFor string) error {
	_, err := s.db.Exec(
		`UPDATE raft_state SET current_term = ?, voted_for = ? WHERE id = 1`,
		term, votedFor,
	)
	return err
}

// LoadState restores the current term and voted-for from stable storage.
func (s *SQLiteStorage) LoadState() (uint64, string, error) {
	var term uint64
	var votedFor string
	err := s.db.QueryRow(`SELECT current_term, voted_for FROM raft_state WHERE id = 1`).
		Scan(&term, &votedFor)
	if err != nil {
		return 0, "", err
	}
	return term, votedFor, nil
}

// SaveLog persists log entries to stable storage.
func (s *SQLiteStorage) SaveLog(entries []LogEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO raft_log (idx, term, command) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		data, err := json.Marshal(e.Command)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(e.Index, e.Term, data); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// LoadLog restores all log entries from stable storage.
func (s *SQLiteStorage) LoadLog() ([]LogEntry, error) {
	rows, err := s.db.Query(`SELECT idx, term, command FROM raft_log ORDER BY idx ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []LogEntry
	for rows.Next() {
		var e LogEntry
		var data []byte
		if err := rows.Scan(&e.Index, &e.Term, &data); err != nil {
			return nil, err
		}
		e.Command = data
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// SaveSnapshot persists a snapshot to stable storage.
func (s *SQLiteStorage) SaveSnapshot(index, term uint64, data []byte) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO raft_snapshot (id, last_index, last_term, data) VALUES (1, ?, ?, ?)`,
		index, term, data,
	)
	return err
}

// LoadSnapshot restores the latest snapshot from stable storage.
func (s *SQLiteStorage) LoadSnapshot() (uint64, uint64, []byte, error) {
	var index, term uint64
	var data []byte
	err := s.db.QueryRow(`SELECT last_index, last_term, data FROM raft_snapshot WHERE id = 1`).
		Scan(&index, &term, &data)
	if err == sql.ErrNoRows {
		return 0, 0, nil, nil
	}
	if err != nil {
		return 0, 0, nil, err
	}
	return index, term, data, nil
}

// InmemStorage is an in-memory implementation for testing.
type InmemStorage struct {
	term     uint64
	votedFor string
	log      []LogEntry
	snapIdx  uint64
	snapTerm uint64
	snapData []byte
}

// NewInmemStorage creates a new in-memory storage.
func NewInmemStorage() *InmemStorage {
	return &InmemStorage{}
}

func (s *InmemStorage) SaveState(term uint64, votedFor string) error {
	s.term = term
	s.votedFor = votedFor
	return nil
}

func (s *InmemStorage) LoadState() (uint64, string, error) {
	return s.term, s.votedFor, nil
}

func (s *InmemStorage) SaveLog(entries []LogEntry) error {
	s.log = append(s.log, entries...)
	return nil
}

func (s *InmemStorage) LoadLog() ([]LogEntry, error) {
	return s.log, nil
}

func (s *InmemStorage) SaveSnapshot(index, term uint64, data []byte) error {
	s.snapIdx = index
	s.snapTerm = term
	s.snapData = data
	return nil
}

func (s *InmemStorage) LoadSnapshot() (uint64, uint64, []byte, error) {
	return s.snapIdx, s.snapTerm, s.snapData, nil
}
