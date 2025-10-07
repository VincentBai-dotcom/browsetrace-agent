package main

import (
	"log"
	"os"
	"path/filepath"
)

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

	// Initialize database
	db, err := NewDatabase(databasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Get server address from environment or use default
	serverAddress := os.Getenv("BROWSETRACE_ADDRESS")
	if serverAddress == "" {
		serverAddress = "127.0.0.1:51425"
	}

	// Initialize and start server
	server := NewServer(db, serverAddress)
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
