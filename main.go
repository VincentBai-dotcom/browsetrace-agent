package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // CGO-free SQLite
)

type Event struct {
	TSUTC int64          `json:"ts_utc" :"tsutc"`
	TSISO string         `json:"ts_iso" :"tsiso"`
	URL   string         `json:"url" :"url"`
	Title *string        `json:"title" :"title"` // nullable
	Type  string         `json:"type" :"type"`   // navigate|visible_text|click|input|scroll|focus
	Data  map[string]any `json:"data" :"data"`   // arbitrary JSON
}

type Batch struct {
	Events []Event `json:"events"`
}

func main() {
	// app data dir: ~/Library/Application Support/BrowserTrace/events.db
	home, _ := os.UserHomeDir()
	appDir := filepath.Join(home, "Library", "Application Support", "BrowserTrace")
	_ = os.MkdirAll(appDir, 0o755)
	dbPath := filepath.Join(appDir, "events.db")

	// WAL + busy timeout to avoid “database is locked”
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Schema from your design doc
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
		log.Fatal(err)
	}

	ins := func(evts []Event) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		stmt, err := tx.Prepare(`INSERT INTO events(ts_utc, ts_iso, url, title, type, data_json) VALUES(?,?,?,?,?,json(?))`)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		defer stmt.Close()

		for _, e := range evts {
			j, _ := json.Marshal(e.Data)
			if _, err := stmt.Exec(e.TSUTC, e.TSISO, e.URL, e.Title, e.Type, string(j)); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		return tx.Commit()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var b Batch
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if len(b.Events) == 0 {
			w.WriteHeader(204)
			return
		}
		if err := ins(b.Events); err != nil {
			http.Error(w, "db error", 500)
			return
		}
		w.WriteHeader(204) // success, no body
	})

	s := &http.Server{
		Addr:         "127.0.0.1:51425",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	log.Println("agent listening on 127.0.0.1:51425")
	log.Fatal(s.ListenAndServe())
}
