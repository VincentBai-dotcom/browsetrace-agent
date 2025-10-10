package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/vincentbai/browsetrace-agent/internal/database"
	"github.com/vincentbai/browsetrace-agent/internal/server"
)

func main() {
	// app data dir: platform-specific
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Failed to get user home directory:", err)
	}

	var applicationDirectory string
	switch runtime.GOOS {
	case "darwin":
		applicationDirectory = filepath.Join(homeDirectory, "Library", "Application Support", "BrowserTrace")
	case "windows":
		applicationDirectory = filepath.Join(homeDirectory, "AppData", "Roaming", "BrowserTrace")
	default: // linux and others
		applicationDirectory = filepath.Join(homeDirectory, ".local", "share", "BrowserTrace")
	}
	if err := os.MkdirAll(applicationDirectory, 0o755); err != nil {
		log.Fatal("Failed to create application directory:", err)
	}
	databasePath := filepath.Join(applicationDirectory, "events.db")

	// Initialize database
	db, err := database.NewDatabase(databasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Get server address from environment or use default
	serverAddress := os.Getenv("BROWSETRACE_ADDRESS")
	if serverAddress == "" {
		serverAddress = "127.0.0.1:8123"
	}

	// Initialize and start server
	srv := server.NewServer(db, serverAddress)
	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
}
