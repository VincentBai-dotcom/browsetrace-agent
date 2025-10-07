package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "modernc.org/sqlite" // CGO-free SQLite
)

type Event struct {
	TSUTC int64          `json:"ts_utc"`
	TSISO string         `json:"ts_iso"`
	URL   string         `json:"url"`
	Title *string        `json:"title"` // nullable
	Type  string         `json:"type"`  // navigate|visible_text|click|input|scroll|focus
	Data  map[string]any `json:"data"`  // arbitrary JSON
}

type Batch struct {
	Events []Event `json:"events"`
}

func main() {
	// app data dir: ~/Library/Application Support/BrowserTrace/events.db
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Failed to get user home directory:", err)
	}
	applicationDirectory := filepath.Join(homeDirectory, "Library", "Application Support", "BrowserTrace")
	if err := os.MkdirAll(applicationDirectory, 0o755); err != nil {
		log.Fatal("Failed to create application directory:", err)
	}
	databasePath := filepath.Join(applicationDirectory, "events.db")

	// WAL + busy timeout to avoid "database is locked"
	db, err := sql.Open("sqlite", databasePath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()

	_, err = db.Exec(`
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
		log.Fatal("Failed to create database tables:", err)
	}

	// Input validation helpers
	validEventTypes := map[string]bool{
		"navigate":     true,
		"visible_text": true,
		"click":        true,
		"input":        true,
		"scroll":       true,
		"focus":        true,
	}

	validateEvent := func(event Event) error {
		if event.URL == "" {
			return fmt.Errorf("URL cannot be empty")
		}
		if event.Type == "" {
			return fmt.Errorf("Type cannot be empty")
		}
		if !validEventTypes[event.Type] {
			return fmt.Errorf("invalid event type: %s", event.Type)
		}
		if event.TSUTC <= 0 {
			return fmt.Errorf("timestamp must be positive")
		}
		return nil
	}

	insertEvents := func(events []Event) error {
		transaction, err := db.Begin()
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
			if err := validateEvent(event); err != nil {
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

	// Get server address from environment or use default
	serverAddress := os.Getenv("BROWSETRACE_ADDRESS")
	if serverAddress == "" {
		serverAddress = "127.0.0.1:51425"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.Write([]byte("ok"))
	})
	mux.HandleFunc("/events", func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(responseWriter, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var batch Batch
		if err := json.NewDecoder(request.Body).Decode(&batch); err != nil {
			http.Error(responseWriter, "Invalid JSON format", http.StatusBadRequest)
			return
		}
		if len(batch.Events) == 0 {
			responseWriter.WriteHeader(http.StatusNoContent)
			return
		}
		if err := insertEvents(batch.Events); err != nil {
			log.Printf("Database error: %v", err)
			http.Error(responseWriter, "Failed to store events", http.StatusInternalServerError)
			return
		}
		responseWriter.WriteHeader(http.StatusNoContent) // success, no body
	})

	server := &http.Server{
		Addr:         serverAddress,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	// Graceful shutdown
	shutdownChannel := make(chan os.Signal, 1)
	signal.Notify(shutdownChannel, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("BrowserTrace agent listening on %s", serverAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start:", err)
		}
	}()

	<-shutdownChannel
	log.Println("Shutting down server...")

	shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownContext); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}
