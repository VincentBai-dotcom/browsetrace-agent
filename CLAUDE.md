# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

BrowserTrace Agent is a **browser activity tracking agent** written in Go. It:
- Stores browsing events (clicks, navigation, text input, scrolls, focus) in a SQLite database
- Exposes an HTTP API to receive batches of events from browser extensions
- Runs as a local server (default: `127.0.0.1:51425`)
- Supports graceful shutdown and cross-platform file paths

## Architecture

The codebase follows Go project layout conventions with clear separation of concerns:

### Package Structure

- **`cmd/browsetrace-agent/main.go`**: Entry point. Initializes database, creates server, handles platform-specific application directories
- **`internal/database/`**: Database layer handling SQLite connections, table creation, event validation, and transactional inserts
- **`internal/models/`**: Data models (`Event` and `Batch` structs) with JSON tags
- **`internal/server/`**: HTTP server with two endpoints (`/healthz` and `/events`), request handling, and graceful shutdown

### Key Design Patterns

1. **Platform-specific data storage**: Uses OS-appropriate paths for database storage:
   - macOS: `~/Library/Application Support/BrowserTrace/`
   - Windows: `~/AppData/Roaming/BrowserTrace/`
   - Linux: `~/.local/share/BrowserTrace/`

2. **Database layer (`internal/database/database.go`)**:
   - Uses CGO-free SQLite driver (`modernc.org/sqlite`)
   - Configured with WAL mode and busy timeout to prevent lock contention
   - Validates events before insertion (both Go-level and database constraints)
   - Uses transactions for atomic batch inserts
   - Stores arbitrary JSON data in `data_json` column with validation

3. **Server layer (`internal/server/server.go`)**:
   - Standard library HTTP server with timeouts (5s read/write)
   - Graceful shutdown via signal handling (SIGINT/SIGTERM)
   - POST-only `/events` endpoint accepts JSON batches
   - Returns appropriate HTTP status codes (204, 400, 405, 500)

4. **Event validation**: Two-layer validation (Go validation in database layer + SQLite CHECK constraints)

### Event Types

Valid event types are: `navigate`, `visible_text`, `click`, `input`, `scroll`, `focus`

## Common Development Commands

### Build
```bash
go build -o bin/browsetrace-agent ./cmd/browsetrace-agent
```

### Run
```bash
# Default (127.0.0.1:51425)
go run ./cmd/browsetrace-agent

# Custom address
BROWSETRACE_ADDRESS="0.0.0.0:8080" go run ./cmd/browsetrace-agent
```

### Test
```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# Verbose
go test -v ./...

# Single package
go test ./internal/database
go test ./internal/server
go test ./internal/models
```

### Linting
```bash
go fmt ./...
go vet ./...
```

## Testing the API

### Health check
```bash
curl http://127.0.0.1:51425/healthz
```

### Post events
```bash
curl -X POST http://127.0.0.1:51425/events \
  -H "Content-Type: application/json" \
  -d '{
    "events": [{
      "ts_utc": 1696704000000,
      "ts_iso": "2025-10-07T12:00:00Z",
      "url": "https://example.com",
      "title": "Example Page",
      "type": "navigate",
      "data": {"foo": "bar"}
    }]
  }'
```

## Database Schema

The SQLite database has a single `events` table:
- `id`: INTEGER PRIMARY KEY (auto-increment)
- `ts_utc`: INTEGER (Unix timestamp in milliseconds)
- `ts_iso`: TEXT (ISO 8601 timestamp)
- `url`: TEXT (page URL)
- `title`: TEXT (nullable page title)
- `type`: TEXT (constrained to valid event types)
- `data_json`: TEXT (validated JSON)

Indexes exist on `ts_utc`, `type`, and `url` for query performance.

## Error Handling

The codebase follows Go's explicit error handling pattern:
- All errors are wrapped with context using `fmt.Errorf(..., %w, err)`
- Database errors trigger transaction rollbacks
- HTTP handlers return appropriate status codes and log detailed errors internally
- Server uses timeouts to prevent slowloris attacks
