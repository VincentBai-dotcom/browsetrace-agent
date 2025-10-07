package database

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/vincentbai/browsetrace-agent/internal/models"
	_ "modernc.org/sqlite" // CGO-free SQLite
)

type Database struct {
	db              *sql.DB
	validEventTypes map[string]bool
}

func NewDatabase(databasePath string) (*Database, error) {
	// WAL + busy timeout to avoid "database is locked"
	db, err := sql.Open("sqlite", databasePath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := createTables(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Database{
		db: db,
		validEventTypes: map[string]bool{
			"navigate":     true,
			"visible_text": true,
			"click":        true,
			"input":        true,
			"scroll":       true,
			"focus":        true,
		},
	}, nil
}

func createTables(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS events(
	  id        INTEGER PRIMARY KEY,
	  ts_utc    INTEGER NOT NULL,
	  ts_iso    TEXT    NOT NULL,
	  url       TEXT    NOT NULL,
	  title     TEXT,
	  type      TEXT    NOT NULL CHECK (type IN ('navigate','visible_text','click','input','scroll','focus')),
	  data_json TEXT    NOT NULL CHECK (json_valid(data_json))
	);
	CREATE INDEX IF NOT EXISTS idx_events_ts   ON events(ts_utc);
	CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
	CREATE INDEX IF NOT EXISTS idx_events_url  ON events(url);
	`)
	if err != nil {
		return fmt.Errorf("failed to create database tables: %w", err)
	}
	return nil
}

func (d *Database) Close() error {
	return d.db.Close()
}

func (d *Database) ValidateEvent(event models.Event) error {
	if event.URL == "" {
		return fmt.Errorf("URL cannot be empty")
	}
	if event.Type == "" {
		return fmt.Errorf("Type cannot be empty")
	}
	if !d.validEventTypes[event.Type] {
		return fmt.Errorf("invalid event type: %s", event.Type)
	}
	if event.TSUTC <= 0 {
		return fmt.Errorf("timestamp must be positive")
	}
	return nil
}

func (d *Database) InsertEvents(events []models.Event) error {
	transaction, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	statement, err := transaction.Prepare(`INSERT INTO events(ts_utc, ts_iso, url, title, type, data_json) VALUES(?,?,?,?,?,json(?))`)
	if err != nil {
		_ = transaction.Rollback()
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer statement.Close()

	for _, event := range events {
		if err := d.ValidateEvent(event); err != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("invalid event: %w", err)
		}

		jsonData, err := json.Marshal(event.Data)
		if err != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("failed to marshal event data: %w", err)
		}
		if _, err := statement.Exec(event.TSUTC, event.TSISO, event.URL, event.Title, event.Type, string(jsonData)); err != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("failed to execute statement: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}
