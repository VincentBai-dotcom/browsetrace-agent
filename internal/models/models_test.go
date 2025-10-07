package models

import (
	"encoding/json"
	"testing"
)

func TestEventJSONMarshaling(t *testing.T) {
	title := "Test Page"
	event := Event{
		TSUTC: 1234567890,
		TSISO: "2009-02-13T23:31:30Z",
		URL:   "https://example.com",
		Title: &title,
		Type:  "navigate",
		Data: map[string]any{
			"referrer": "https://google.com",
			"method":   "GET",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	// Unmarshal back
	var unmarshaled Event
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal event: %v", err)
	}

	// Verify fields
	if unmarshaled.TSUTC != event.TSUTC {
		t.Errorf("TSUTC mismatch: got %d, want %d", unmarshaled.TSUTC, event.TSUTC)
	}
	if unmarshaled.URL != event.URL {
		t.Errorf("URL mismatch: got %s, want %s", unmarshaled.URL, event.URL)
	}
	if unmarshaled.Type != event.Type {
		t.Errorf("Type mismatch: got %s, want %s", unmarshaled.Type, event.Type)
	}
}

func TestEventWithNullTitle(t *testing.T) {
	event := Event{
		TSUTC: 1234567890,
		TSISO: "2009-02-13T23:31:30Z",
		URL:   "https://example.com",
		Title: nil,
		Type:  "click",
		Data:  map[string]any{},
	}

	jsonData, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event with null title: %v", err)
	}

	var unmarshaled Event
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal event with null title: %v", err)
	}

	if unmarshaled.Title != nil {
		t.Errorf("Expected nil title, got %v", unmarshaled.Title)
	}
}

func TestBatchJSONMarshaling(t *testing.T) {
	title := "Test"
	batch := Batch{
		Events: []Event{
			{
				TSUTC: 1234567890,
				TSISO: "2009-02-13T23:31:30Z",
				URL:   "https://example.com",
				Title: &title,
				Type:  "navigate",
				Data:  map[string]any{"foo": "bar"},
			},
			{
				TSUTC: 1234567891,
				TSISO: "2009-02-13T23:31:31Z",
				URL:   "https://example.com/page2",
				Title: nil,
				Type:  "click",
				Data:  map[string]any{},
			},
		},
	}

	jsonData, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("Failed to marshal batch: %v", err)
	}

	var unmarshaled Batch
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal batch: %v", err)
	}

	if len(unmarshaled.Events) != len(batch.Events) {
		t.Errorf("Event count mismatch: got %d, want %d", len(unmarshaled.Events), len(batch.Events))
	}
}

func TestEmptyBatch(t *testing.T) {
	batch := Batch{Events: []Event{}}

	jsonData, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("Failed to marshal empty batch: %v", err)
	}

	var unmarshaled Batch
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal empty batch: %v", err)
	}

	if len(unmarshaled.Events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(unmarshaled.Events))
	}
}
