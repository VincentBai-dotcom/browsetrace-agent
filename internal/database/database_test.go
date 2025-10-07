package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vincentbai/browsetrace-agent/internal/models"
)

func setupTestDB(t *testing.T) (*Database, func()) {
	t.Helper()

	// Create temporary directory for test database
	tmpDir, err := os.MkdirTemp("", "browsetrace-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDatabase(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Return cleanup function
	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

func TestNewDatabase(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	if db == nil {
		t.Fatal("Expected non-nil database")
	}
	if db.db == nil {
		t.Fatal("Expected non-nil sql.DB")
	}
}

func TestValidateEvent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	tests := []struct {
		name      string
		event     models.Event
		wantError bool
	}{
		{
			name: "valid navigate event",
			event: models.Event{
				TSUTC: 1234567890,
				TSISO: "2009-02-13T23:31:30Z",
				URL:   "https://example.com",
				Type:  "navigate",
				Data:  map[string]any{},
			},
			wantError: false,
		},
		{
			name: "empty URL",
			event: models.Event{
				TSUTC: 1234567890,
				TSISO: "2009-02-13T23:31:30Z",
				URL:   "",
				Type:  "navigate",
				Data:  map[string]any{},
			},
			wantError: true,
		},
		{
			name: "empty type",
			event: models.Event{
				TSUTC: 1234567890,
				TSISO: "2009-02-13T23:31:30Z",
				URL:   "https://example.com",
				Type:  "",
				Data:  map[string]any{},
			},
			wantError: true,
		},
		{
			name: "invalid event type",
			event: models.Event{
				TSUTC: 1234567890,
				TSISO: "2009-02-13T23:31:30Z",
				URL:   "https://example.com",
				Type:  "invalid_type",
				Data:  map[string]any{},
			},
			wantError: true,
		},
		{
			name: "zero timestamp",
			event: models.Event{
				TSUTC: 0,
				TSISO: "2009-02-13T23:31:30Z",
				URL:   "https://example.com",
				Type:  "navigate",
				Data:  map[string]any{},
			},
			wantError: true,
		},
		{
			name: "negative timestamp",
			event: models.Event{
				TSUTC: -1,
				TSISO: "2009-02-13T23:31:30Z",
				URL:   "https://example.com",
				Type:  "navigate",
				Data:  map[string]any{},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.ValidateEvent(tt.event)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateEvent() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestInsertEvents(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	title := "Test Page"
	events := []models.Event{
		{
			TSUTC: 1234567890,
			TSISO: "2009-02-13T23:31:30Z",
			URL:   "https://example.com",
			Title: &title,
			Type:  "navigate",
			Data:  map[string]any{"referrer": "https://google.com"},
		},
		{
			TSUTC: 1234567891,
			TSISO: "2009-02-13T23:31:31Z",
			URL:   "https://example.com/page2",
			Title: nil,
			Type:  "click",
			Data:  map[string]any{"x": 100, "y": 200},
		},
	}

	err := db.InsertEvents(events)
	if err != nil {
		t.Fatalf("Failed to insert events: %v", err)
	}

	// Verify events were inserted
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query count: %v", err)
	}

	if count != len(events) {
		t.Errorf("Expected %d events, got %d", len(events), count)
	}
}

func TestInsertEventsInvalidEvent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	events := []models.Event{
		{
			TSUTC: 1234567890,
			TSISO: "2009-02-13T23:31:30Z",
			URL:   "",  // Invalid: empty URL
			Type:  "navigate",
			Data:  map[string]any{},
		},
	}

	err := db.InsertEvents(events)
	if err == nil {
		t.Fatal("Expected error for invalid event, got nil")
	}

	// Verify transaction was rolled back
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query count: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 events after rollback, got %d", count)
	}
}

func TestAllEventTypes(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	eventTypes := []string{"navigate", "visible_text", "click", "input", "scroll", "focus"}

	for _, eventType := range eventTypes {
		t.Run(eventType, func(t *testing.T) {
			events := []models.Event{
				{
					TSUTC: 1234567890,
					TSISO: "2009-02-13T23:31:30Z",
					URL:   "https://example.com",
					Type:  eventType,
					Data:  map[string]any{},
				},
			}

			err := db.InsertEvents(events)
			if err != nil {
				t.Errorf("Failed to insert %s event: %v", eventType, err)
			}
		})
	}

	// Verify all events were inserted
	var count int
	err := db.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query count: %v", err)
	}

	if count != len(eventTypes) {
		t.Errorf("Expected %d events, got %d", len(eventTypes), count)
	}
}

func TestInsertEventsWithComplexData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	events := []models.Event{
		{
			TSUTC: 1234567890,
			TSISO: "2009-02-13T23:31:30Z",
			URL:   "https://example.com",
			Type:  "input",
			Data: map[string]any{
				"field":  "email",
				"value":  "test@example.com",
				"nested": map[string]any{
					"foo": "bar",
					"baz": 123,
				},
			},
		},
	}

	err := db.InsertEvents(events)
	if err != nil {
		t.Fatalf("Failed to insert event with complex data: %v", err)
	}

	// Verify data was stored as valid JSON
	var dataJSON string
	err = db.db.QueryRow("SELECT data_json FROM events WHERE id = 1").Scan(&dataJSON)
	if err != nil {
		t.Fatalf("Failed to query data_json: %v", err)
	}

	if dataJSON == "" {
		t.Error("Expected non-empty data_json")
	}
}

func TestDatabaseClose(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := db.Close()
	if err != nil {
		t.Errorf("Failed to close database: %v", err)
	}
}
