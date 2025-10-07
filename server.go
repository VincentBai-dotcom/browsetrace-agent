package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Server struct {
	db      *Database
	address string
	server  *http.Server
}

func NewServer(db *Database, address string) *Server {
	return &Server{
		db:      db,
		address: address,
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("ok"))
}

func (s *Server) handleEvents(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var batch Batch
	if err := json.NewDecoder(request.Body).Decode(&batch); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}
	if len(batch.Events) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.db.InsertEvents(batch.Events); err != nil {
		log.Printf("Database error: %v", err)
		http.Error(w, "Failed to store events", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent) // success, no body
}

func (s *Server) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/events", s.handleEvents)
	return mux
}

func (s *Server) Start() error {
	mux := s.setupRoutes()
	s.server = &http.Server{
		Addr:         s.address,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	// Graceful shutdown
	shutdownChannel := make(chan os.Signal, 1)
	signal.Notify(shutdownChannel, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("BrowserTrace agent listening on %s", s.address)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start:", err)
		}
	}()

	<-shutdownChannel
	log.Println("Shutting down server...")

	shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(shutdownContext); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
	return nil
}