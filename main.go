package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

type Event struct {
	Type string `json:"type"`
}

func main() {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, "Library/Application Support/BrowserTrace/events.log")
	os.MkdirAll(filepath.Dir(dbPath), 0o755)
	f, _ := os.OpenFile(dbPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	defer f.Close()

	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		var events []Event
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		for _, e := range events {
			log.SetOutput(f)
			log.Println(e.Type)
		}
		w.WriteHeader(204)
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:51425", nil))
}
