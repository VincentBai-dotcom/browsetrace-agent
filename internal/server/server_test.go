package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vincentbai/browsetrace-agent/internal/database"
	"github.com/vincentbai/browsetrace-agent/internal/models"
)

func setupTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "browsetrace-server-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.NewDatabase(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create test database: %v", err)
	}

	server := NewServer(db, "127.0.0.1:0") // Port 0 for testing

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return server, cleanup
}

func TestNewServer(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	if server == nil {
		t.Fatal("Expected non-nil server")
	}
	if server.db == nil {
		t.Fatal("Expected non-nil database")
	}
	if server.address != "127.0.0.1:0" {
		t.Errorf("Expected address 127.0.0.1:0, got %s", server.address)
	}
}

func TestHandleHealthz(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	server.handleHealthz(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if body != "ok" {
		t.Errorf("Expected body 'ok', got %s", body)
	}
}

func TestHandleEventsSuccess(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	title := "Test Page"
	batch := models.Batch{
		Events: []models.Event{
			{
				TSUTC: 1234567890,
				TSISO: "2009-02-13T23:31:30Z",
				URL:   "https://example.com",
				Title: &title,
				Type:  "navigate",
				Data:  map[string]any{"referrer": "https://google.com"},
			},
		},
	}

	jsonData, _ := json.Marshal(batch)
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleEvents(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}
}

func TestHandleEventsMethodNotAllowed(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	w := httptest.NewRecorder()

	server.handleEvents(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestHandleEventsInvalidJSON(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	invalidJSON := []byte(`{"events": [invalid json]}`)
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(invalidJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleEvents(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestHandleEventsEmptyBatch(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	batch := models.Batch{Events: []models.Event{}}
	jsonData, _ := json.Marshal(batch)
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleEvents(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}
}

func TestHandleEventsInvalidEvent(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	batch := models.Batch{
		Events: []models.Event{
			{
				TSUTC: 1234567890,
				TSISO: "2009-02-13T23:31:30Z",
				URL:   "", // Invalid: empty URL
				Type:  "navigate",
				Data:  map[string]any{},
			},
		},
	}

	jsonData, _ := json.Marshal(batch)
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleEvents(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", resp.StatusCode)
	}
}

func TestHandleEventsMultipleEvents(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	title1 := "Page 1"
	title2 := "Page 2"
	batch := models.Batch{
		Events: []models.Event{
			{
				TSUTC: 1234567890,
				TSISO: "2009-02-13T23:31:30Z",
				URL:   "https://example.com",
				Title: &title1,
				Type:  "navigate",
				Data:  map[string]any{},
			},
			{
				TSUTC: 1234567891,
				TSISO: "2009-02-13T23:31:31Z",
				URL:   "https://example.com/page2",
				Title: &title2,
				Type:  "click",
				Data:  map[string]any{"x": 100, "y": 200},
			},
			{
				TSUTC: 1234567892,
				TSISO: "2009-02-13T23:31:32Z",
				URL:   "https://example.com/page3",
				Title: nil,
				Type:  "scroll",
				Data:  map[string]any{"position": 500},
			},
		},
	}

	jsonData, _ := json.Marshal(batch)
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleEvents(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}
}

func TestSetupRoutes(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	mux := server.setupRoutes()
	if mux == nil {
		t.Fatal("Expected non-nil ServeMux")
	}

	// Test that routes are registered
	tests := []struct {
		path   string
		method string
		status int
	}{
		{"/healthz", http.MethodGet, http.StatusOK},
		{"/events", http.MethodGet, http.StatusMethodNotAllowed}, // Only POST allowed
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != tt.status {
				t.Errorf("Expected status %d for %s %s, got %d", tt.status, tt.method, tt.path, w.Code)
			}
		})
	}
}

func TestHandleEventsContentType(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	title := "Test"
	batch := models.Batch{
		Events: []models.Event{
			{
				TSUTC: 1234567890,
				TSISO: "2009-02-13T23:31:30Z",
				URL:   "https://example.com",
				Title: &title,
				Type:  "navigate",
				Data:  map[string]any{},
			},
		},
	}

	jsonData, _ := json.Marshal(batch)
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(jsonData))
	// Not setting Content-Type header to test robustness
	w := httptest.NewRecorder()

	server.handleEvents(w, req)

	resp := w.Result()
	// Should still work without Content-Type
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}
}
