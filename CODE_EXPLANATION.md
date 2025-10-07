# BrowserTrace Agent - Code Explanation

A comprehensive guide to understanding the `main.go` implementation for Go beginners.

---

## Overview

This is a **browser activity tracking agent** that:

1. **Sets up storage:** Creates a SQLite database at `~/Library/Application Support/BrowserTrace/events.db`
2. **Creates schema:** Defines a table to store browsing events (clicks, navigation, text input, etc.)
3. **Starts HTTP server:** Listens on port 51425 (configurable via environment variable)
4. **Exposes two endpoints:**
   - `GET /healthz` - Health check (returns "ok")
   - `POST /events` - Receives batches of events and stores them in the database
5. **Handles shutdown gracefully:** On Ctrl+C, finishes processing requests, closes database cleanly, then exits

---

## Part 1: Package Declaration and Imports (Lines 1-18)

### Package Declaration
```go
package main
```
**What it does:** Declares this file belongs to the `main` package. In Go, `package main` is special - it tells Go this is an executable program (not a library). Every executable Go program must have exactly one `package main` and one `main()` function as the entry point.

### Imports
```go
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
```

**Each package's purpose:**
- `context` - Used for managing cancellation and timeouts (you'll see this for graceful shutdown)
- `database/sql` - Go's standard database interface for SQL databases
- `encoding/json` - Handles JSON encoding/decoding
- `fmt` - Formatted I/O (printing, string formatting)
- `log` - Logging errors and info messages
- `net/http` - HTTP server and client functionality
- `os` - Operating system functions (file system, environment variables)
- `os/signal` - Handles OS signals (like Ctrl+C)
- `path/filepath` - Cross-platform file path manipulation
- `syscall` - Low-level OS system calls
- `time` - Time-related functions

### Special Import: SQLite Driver
```go
_ "modernc.org/sqlite"
```
The underscore `_` is a **blank identifier**. This imports the package for its *side effects* only (initializing the SQLite driver), not to use any exported functions directly.

**Why?** The `database/sql` package works with drivers. When you import a driver package, it registers itself with `database/sql` during initialization. The comment "CGO-free SQLite" means this implementation doesn't require CGO (C bindings), making it easier to compile and distribute.

---

## Part 2: Data Structures (Lines 20-31)

### Event Struct
```go
type Event struct {
	TSUTC int64          `json:"ts_utc"`
	TSISO string         `json:"ts_iso"`
	URL   string         `json:"url"`
	Title *string        `json:"title"` // nullable
	Type  string         `json:"type"`  // navigate|visible_text|click|input|scroll|focus
	Data  map[string]any `json:"data"`  // arbitrary JSON
}
```

**What it does:** Defines a custom type called `Event` using a **struct** (Go's way of defining data structures with named fields).

**Each field explained:**
- `TSUTC int64` - Timestamp in UTC as an integer (Unix timestamp in milliseconds)
- `TSISO string` - Timestamp in ISO format (human-readable string like "2025-01-01T12:00:00Z")
- `URL string` - The URL where the event occurred
- `Title *string` - Notice the asterisk `*`? This makes it a **pointer to string**, meaning it can be `nil` (null). Why? Some events might not have a title.
- `Type string` - Event type (navigate, click, etc.)
- `Data map[string]any` - A map (like a dictionary/object) where keys are strings and values can be anything (`any` is an alias for `interface{}`, Go's way of saying "any type")

### Struct Tags
```go
`json:"ts_utc"`
```
These are **struct tags** - metadata about the field. The `json` tag tells the `encoding/json` package what JSON key name to use when converting between Go structs and JSON. So the Go field `TSUTC` becomes `"ts_utc"` in JSON.

**Design choice:** Why separate `TSUTC` and `TSISO`? Having both integer and string timestamps allows flexibility - the integer is efficient for sorting/filtering, while the ISO string is human-readable.

### Batch Struct
```go
type Batch struct {
	Events []Event `json:"events"`
}
```

**What it does:** Defines a `Batch` struct that contains a slice (Go's dynamic array) of `Event` structs.

**Design choice:** Why wrap events in a Batch? This allows the API to receive multiple events in a single HTTP request, which is more efficient than sending them one at a time.

---

## Part 3: Main Function Start - Setting Up Directories (Lines 33-43)

```go
func main() {
```
**What it does:** Declares the `main` function - the entry point where program execution begins. Every executable Go program must have exactly one `main()` function in the `main` package.

### Getting Home Directory
```go
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Failed to get user home directory:", err)
	}
```

**What it does:** Gets the user's home directory path (e.g., `/Users/baihaocheng`).

**Go pattern - error handling:**
- `os.UserHomeDir()` returns **two values**: the directory path AND an error
- This `:=` operator declares new variables and assigns values in one line
- Immediately check if `err != nil` (if there was an error)
- `log.Fatal()` prints the error message and exits the program with status code 1

**Design choice:** Go uses explicit error handling (no exceptions). Functions that can fail return an error as their last return value. You must check every error.

### Building Directory Path
```go
	applicationDirectory := filepath.Join(homeDirectory, "Library", "Application Support", "BrowserTrace")
```

**What it does:** `filepath.Join()` combines path segments using the correct separator for your OS (forward slash on Unix/Mac, backslash on Windows).

**Result:** Creates the path `~/Library/Application Support/BrowserTrace`

**Design choice:** Using `filepath.Join` instead of string concatenation makes code portable across operating systems.

### Creating Directory
```go
	if err := os.MkdirAll(applicationDirectory, 0o755); err != nil {
		log.Fatal("Failed to create application directory:", err)
	}
```

**What it does:**
- `os.MkdirAll()` creates the directory and any parent directories if they don't exist (like `mkdir -p`)
- `0o755` is the Unix file permissions in **octal** (the `0o` prefix means octal): owner can read/write/execute, group and others can read/execute
- Uses a compact if statement: `:=` declares `err`, then immediately checks it

**Design choice:** Using `MkdirAll` instead of `Mkdir` means we don't need to check if parent directories exist - it handles the entire path.

### Database Path
```go
	databasePath := filepath.Join(applicationDirectory, "events.db")
```

**What it does:** Creates the full path to the SQLite database file: `~/Library/Application Support/BrowserTrace/events.db`

---

## Part 4: Opening Database and Creating Tables (Lines 45-68)

### Opening Database Connection
```go
	// WAL + busy timeout to avoid "database is locked"
	db, err := sql.Open("sqlite", databasePath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()
```

**What it does:** Opens a connection to the SQLite database.

**Breaking down `sql.Open()`:**
- First argument: `"sqlite"` - the driver name (registered by the imported `modernc.org/sqlite` package)
- Second argument: connection string with URL parameters
  - `?_journal_mode=WAL` - Use **Write-Ahead Logging** mode
  - `&_busy_timeout=5000` - Wait up to 5000ms (5 seconds) if database is locked

**What is WAL?** Write-Ahead Logging allows multiple readers while one writer is active, preventing "database is locked" errors. Without WAL, SQLite locks the entire database during writes.

### The `defer` Keyword
```go
defer db.Close()
```
`defer` schedules a function call to execute when the surrounding function (`main`) returns, no matter how it returns (normal exit, panic, etc.). This ensures the database connection closes cleanly.

**Design choice:** Using `defer` right after opening resources is idiomatic Go - it keeps the cleanup code close to the acquisition code, preventing resource leaks.

### Creating Tables and Indexes
```go
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
```

**What it does:** `db.Exec()` executes SQL statements that don't return rows.

**The underscore `_`:**
```go
_, err = db.Exec(...)
```
`db.Exec()` returns two values: a result object and an error. We don't need the result, so `_` (blank identifier) discards it. We only care about the error.

**The SQL explained:**

**Table creation:**
- `CREATE TABLE IF NOT EXISTS` - Only creates if table doesn't exist (safe to run multiple times)
- `id INTEGER PRIMARY KEY` - Auto-incrementing unique identifier
- `INTEGER NOT NULL` / `TEXT NOT NULL` - These fields are required (can't be NULL)
- `title TEXT` - No `NOT NULL`, so this CAN be null (matches our `*string` pointer in Go)
- `CHECK (type IN (...))` - Database-level validation ensuring type is one of the allowed values
- `CHECK (json_valid(data_json))` - SQLite validates the JSON is well-formed

**The indexes:**
```go
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts_utc);
```
Indexes speed up queries. These three indexes optimize searches by:
- `ts_utc` - Finding events by time
- `type` - Filtering by event type
- `url` - Searching events by URL

**Design choice:** Having both Go-level validation AND database constraints provides defense-in-depth. Even if code bugs bypass Go validation, the database rejects invalid data.

---

## Part 5: Validation Setup (Lines 70-94)

### Valid Event Types Map
```go
	validEventTypes := map[string]bool{
		"navigate":     true,
		"visible_text": true,
		"click":        true,
		"input":        true,
		"scroll":       true,
		"focus":        true,
	}
```

**What it does:** Creates a **map** (Go's hash table/dictionary) where keys are strings and values are booleans.

**Design choice - using a map as a set:** This is a common Go idiom for implementing a "set" data structure. Go doesn't have a built-in set type, so we use `map[string]bool` where the keys are the set members. The boolean values don't really matter (they're all `true`), what matters is whether a key exists in the map.

**Why?** Fast O(1) lookup to check if an event type is valid. We could use a slice and loop through it, but map lookup is much faster.

### Validation Function
```go
	validateEvent := func(event Event) error {
```

**What it does:** Declares a **function variable** (also called an anonymous function or closure). This function is stored in the variable `validateEvent` and can be called like any function.

**Why use a function variable instead of a regular function?** It's defined inside `main()`, which keeps related code together and allows it to access variables from `main()`'s scope (like `validEventTypes`).

### Validation Checks
```go
		if event.URL == "" {
			return fmt.Errorf("URL cannot be empty")
		}
		if event.Type == "" {
			return fmt.Errorf("Type cannot be empty")
		}
```

**What it does:** Checks if required fields are empty strings.

**`fmt.Errorf()`**: Creates a new error with a formatted message (similar to `sprintf` but returns an error type).

### Type Validation
```go
		if !validEventTypes[event.Type] {
			return fmt.Errorf("invalid event type: %s", event.Type)
		}
```

**What it does:** Checks if the event type exists in our valid types map.

**Map lookup behavior:** In Go, when you access a map with a key that doesn't exist, it returns the **zero value** for that type (for `bool`, that's `false`). So:
- `validEventTypes["click"]` returns `true` (exists in map)
- `validEventTypes["invalid"]` returns `false` (doesn't exist)

The `!` negates it, so this reads as "if the event type is NOT valid".

**The `%s` placeholder:** In `fmt.Errorf()`, `%s` is replaced with the string value of `event.Type` (like printf formatting).

### Timestamp Validation
```go
		if event.TSUTC <= 0 {
			return fmt.Errorf("timestamp must be positive")
		}
		return nil
	}
```

**What it does:** Validates the timestamp is positive. Unix timestamps should be positive numbers.

**`return nil`:** In Go, returning `nil` for an error means "no error occurred" - the validation passed. This is a common pattern: functions return `error` type, and `nil` means success.

---

## Part 6: Insert Events Function (Lines 96-128)

### Function Declaration
```go
	insertEvents := func(events []Event) error {
```

**What it does:** Another function variable that takes a slice of events and returns an error (or `nil` on success).

### Starting a Transaction
```go
		transaction, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
```

**What it does:** `db.Begin()` starts a database **transaction**.

**What's a transaction?** A transaction groups multiple database operations into a single atomic unit - either ALL operations succeed, or ALL are rolled back (undone). This prevents partial data corruption.

**The `%w` verb:** Unlike `%s`, the `%w` verb in `fmt.Errorf()` **wraps** the original error. This preserves the error chain, allowing callers to check what the underlying error was using `errors.Is()` or `errors.As()`. It's the modern Go way (Go 1.13+) to add context to errors.

### Preparing Statement
```go
		statement, err := transaction.Prepare(`INSERT INTO events(ts_utc, ts_iso, url, title, type, data_json) VALUES(?,?,?,?,?,json(?))`)
		if err != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer statement.Close()
```

**What it does:** `Prepare()` creates a **prepared statement** - a pre-compiled SQL query.

**Why prepare?** When inserting many rows:
- Without prepare: SQL is parsed and compiled for each insert (slow)
- With prepare: SQL is parsed once, then executed many times with different values (fast and prevents SQL injection)

**The `?` placeholders:** These are parameter placeholders that will be filled with actual values during `Exec()`. This safely escapes values, preventing SQL injection attacks.

**The `json(?)` function:** SQLite's `json()` function validates and stores JSON. It ensures the data is valid JSON.

**`_ = transaction.Rollback()`:** If preparation fails, roll back the transaction. The underscore discards the rollback's error (we're already handling an error and just want cleanup).

**`defer statement.Close()`:** Ensures the prepared statement resources are freed when the function exits.

### Looping Through Events
```go
		for _, event := range events {
```

**What it does:** Loops through each event in the slice.

**The `range` keyword:** Iterates over a slice. It returns two values per iteration:
- Index (which we discard with `_` since we don't need it)
- The actual event value

### Validating Each Event
```go
			if err := validateEvent(event); err != nil {
				_ = transaction.Rollback()
				return fmt.Errorf("invalid event: %w", err)
			}
```

**What it does:** Validates each event. If any event is invalid, rollback the transaction and return the error.

**Design choice:** Validate BEFORE inserting. If one event is bad, we don't insert any events - maintaining data consistency.

### Marshaling JSON Data
```go
			jsonData, err := json.Marshal(event.Data)
			if err != nil {
				_ = transaction.Rollback()
				return fmt.Errorf("failed to marshal event data: %w", err)
			}
```

**What it does:** `json.Marshal()` converts the Go `map[string]any` into a JSON string (byte slice).

**Example:**
- Input: `map[string]any{"x": 100, "y": 200}`
- Output: `[]byte("{\"x\":100,\"y\":200}")`

### Executing Statement
```go
			if _, err := statement.Exec(event.TSUTC, event.TSISO, event.URL, event.Title, event.Type, string(jsonData)); err != nil {
				_ = transaction.Rollback()
				return fmt.Errorf("failed to execute statement: %w", err)
			}
		}
```

**What it does:** `statement.Exec()` executes the prepared statement, replacing each `?` with the corresponding argument in order.

**`string(jsonData)`:** Converts the byte slice to a string for the SQL parameter.

**Notice the pattern:** Every error causes a rollback and early return. This ensures atomicity.

### Committing Transaction
```go
		if err := transaction.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
		return nil
	}
```

**What it does:** `transaction.Commit()` saves all changes to the database permanently.

**Design choice - Transaction for batch inserts:** Using a transaction makes bulk inserts much faster (one disk write instead of many) and ensures either all events are saved or none are.

---

## Part 7: Server Configuration (Lines 130-167)

### Server Address Configuration
```go
	serverAddress := os.Getenv("BROWSETRACE_ADDRESS")
	if serverAddress == "" {
		serverAddress = "127.0.0.1:51425"
	}
```

**What it does:** `os.Getenv()` reads an environment variable.

**Design choice - Configuration via environment variables:** This is a common pattern (especially for 12-factor apps). It allows changing the server address without modifying code. If `BROWSETRACE_ADDRESS` isn't set, it defaults to `127.0.0.1:51425`.

**`127.0.0.1`** is localhost (only accessible from this machine), and **51425** is the port number.

### Creating HTTP Multiplexer
```go
	mux := http.NewServeMux()
```

**What it does:** `http.NewServeMux()` creates a new **HTTP request multiplexer** (router).

**What's a mux?** It's a router that matches incoming HTTP requests to handlers based on URL paths. Think of it as a switchboard directing requests to the right handler function.

### Health Check Endpoint
```go
	mux.HandleFunc("/healthz", func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.Write([]byte("ok"))
	})
```

**What it does:** Registers a handler for the `/healthz` endpoint.

**Breaking it down:**
- `mux.HandleFunc("/healthz", ...)` - When a request comes to `/healthz`, call this function
- `func(responseWriter http.ResponseWriter, _ *http.Request)` - This is an **anonymous function** (inline function definition)
  - `responseWriter` - Used to write the HTTP response back to the client
  - `_ *http.Request` - The incoming request (discarded with `_` since we don't need it)
- `responseWriter.Write([]byte("ok"))` - Sends "ok" as the response body
  - `[]byte("ok")` converts the string "ok" to a byte slice (what `Write()` expects)

**Design choice - Health check endpoint:** The `/healthz` endpoint is a common pattern for monitoring. External tools can ping this to verify the server is running.

### Events Endpoint - Method Checking
```go
	mux.HandleFunc("/events", func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(responseWriter, "POST only", http.StatusMethodNotAllowed)
			return
		}
```

**What it does:** Registers the main `/events` endpoint handler.

**Method checking:**
- `request.Method` contains the HTTP method (GET, POST, etc.)
- `http.MethodPost` is a constant equal to `"POST"`
- If it's not a POST request, `http.Error()` sends an error response with status code 405 (Method Not Allowed)
- `return` exits the handler function early

**Design choice:** Explicitly rejecting non-POST methods is good API design - it clearly communicates how the endpoint should be used.

### Parsing JSON Request
```go
		var batch Batch
		if err := json.NewDecoder(request.Body).Decode(&batch); err != nil {
			http.Error(responseWriter, "Invalid JSON format", http.StatusBadRequest)
			return
		}
```

**What it does:** Parses JSON from the request body into a `Batch` struct.

**Breaking it down:**
- `var batch Batch` - Declares an empty `Batch` variable
- `json.NewDecoder(request.Body)` - Creates a JSON decoder that reads from the request body
- `.Decode(&batch)` - Parses JSON and fills the `batch` variable
  - The `&` (ampersand) passes the **address** of `batch`, allowing `Decode()` to modify it
  - This is called "passing by reference" - without `&`, Go would pass a copy and changes wouldn't persist

**Why use `NewDecoder` instead of `json.Unmarshal`?** `NewDecoder` streams from the request body directly without loading everything into memory first - more efficient for potentially large payloads.

### Handling Empty Batch
```go
		if len(batch.Events) == 0 {
			responseWriter.WriteHeader(http.StatusNoContent)
			return
		}
```

**What it does:** If the batch is empty, return status 204 (No Content) - success but nothing to do.

**`len()` built-in function:** Returns the length of a slice, map, string, array, or channel.

### Inserting Events
```go
		if err := insertEvents(batch.Events); err != nil {
			log.Printf("Database error: %v", err)
			http.Error(responseWriter, "Failed to store events", http.StatusInternalServerError)
			return
		}
		responseWriter.WriteHeader(http.StatusNoContent) // success, no body
	})
```

**What it does:** Calls our `insertEvents` function. If it fails, log the error and return 500 (Internal Server Error). If successful, return 204 (No Content).

**`log.Printf()`:** Logs to stderr with formatting. The `%v` verb prints the error's default format.

**Design choice - Error responses:** Notice we log the detailed error but send a generic message to the client. This prevents leaking internal implementation details while keeping detailed logs for debugging.

### Creating HTTP Server
```go
	server := &http.Server{
		Addr:         serverAddress,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
```

**What it does:** Creates an HTTP server configuration.

**The `&` operator:** Creates a pointer to a new `http.Server` struct.

**Fields explained:**
- `Addr` - The address to listen on
- `Handler` - Our mux router that handles requests
- `ReadTimeout` / `WriteTimeout` - Prevents slow clients from holding connections open forever (protects against slowloris attacks)

**`5 * time.Second`:** Go's `time` package uses constants for duration. This multiplies to create a 5-second duration.

---

## Part 8: Graceful Shutdown (Lines 169-191)

### Signal Handling Setup
```go
	// Graceful shutdown
	shutdownChannel := make(chan os.Signal, 1)
	signal.Notify(shutdownChannel, syscall.SIGINT, syscall.SIGTERM)
```

**What it does:** Sets up signal handling for graceful shutdown.

**`make(chan os.Signal, 1)`:** Creates a **channel** - Go's way of communicating between goroutines (concurrent functions).
- `chan os.Signal` - A channel that carries OS signal values
- The `1` is the **buffer size** - it can hold 1 signal without blocking

**What's a channel?** Think of it as a typed pipe. One goroutine can send values into it, another can receive values from it. It's how Go handles concurrency safely.

**`signal.Notify(shutdownChannel, syscall.SIGINT, syscall.SIGTERM)`:** Tells Go to send signals to our channel.
- `syscall.SIGINT` - Signal sent when you press Ctrl+C
- `syscall.SIGTERM` - Signal sent by `kill` command (graceful termination request)

**Design choice - Graceful shutdown:** Instead of abruptly killing the server, we catch these signals to clean up properly (finish handling current requests, close database, etc.).

### Starting the Server in a Goroutine
```go
	go func() {
		log.Printf("BrowserTrace agent listening on %s", serverAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start:", err)
		}
	}()
```

**What it does:** Starts the HTTP server in a separate goroutine.

**The `go` keyword:** Launches a **goroutine** - a lightweight thread managed by Go. The function runs concurrently with the rest of `main()`.

**Why use a goroutine here?** `server.ListenAndServe()` blocks (waits forever) while serving requests. By running it in a goroutine, the main function can continue executing to set up the shutdown logic.

**`server.ListenAndServe()`:** Starts the HTTP server and blocks until it stops.

**Error handling:**
- `err != nil` - An error occurred
- `&& err != http.ErrServerClosed` - BUT ignore this specific error
- Why? `http.ErrServerClosed` is returned when we shut down the server intentionally (not an actual error)

### Waiting for Shutdown Signal
```go
	<-shutdownChannel
	log.Println("Shutting down server...")
```

**What it does:** Waits for a signal to arrive.

**The `<-` operator:** Receives from a channel. This **blocks** (waits) until a value is available.

So the program flow is:
1. Server starts in background goroutine
2. Main goroutine waits here at `<-shutdownChannel`
3. When user presses Ctrl+C, signal is sent to channel
4. This line "receives" the signal and continues

**Design choice:** This is elegant - the entire shutdown logic is written linearly, but it only executes when needed.

### Creating Shutdown Context
```go
	shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
```

**What it does:** Creates a **context** with a 30-second timeout.

**What's a context?** It's Go's standard way to handle cancellation, deadlines, and request-scoped values across API boundaries.

**`context.Background()`:** Creates an empty base context.

**`context.WithTimeout(..., 30*time.Second)`:** Wraps it with a 30-second deadline. After 30 seconds, the context automatically cancels.

**Returns two values:**
- `shutdownContext` - The context with timeout
- `cancel` - A function to cancel the context early (cleanup function)

**`defer cancel()`:** Ensures we call `cancel()` when `main()` exits, releasing context resources.

**Design choice:** The 30-second timeout means "try to shut down gracefully, but force shutdown after 30 seconds if needed."

### Graceful Shutdown
```go
	if err := server.Shutdown(shutdownContext); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
```

**What it does:** `server.Shutdown()` gracefully stops the server.

**What does "graceful" mean here?**
1. Stops accepting new connections
2. Waits for active requests to complete
3. Returns when done OR when the context times out (30 seconds)

If shutdown fails or times out, `log.Fatal()` exits the program.

### Final Log
```go
	log.Println("Server exited")
}
```

**What it does:** Logs a final message and the program ends.

---

## Key Go Patterns Demonstrated

### 1. Explicit Error Handling
Go doesn't use exceptions. Functions that can fail return an error as their last return value:
```go
result, err := someFunction()
if err != nil {
    // handle error
}
```

### 2. Defer for Cleanup
`defer` ensures cleanup code runs when the function exits:
```go
file, err := os.Open("file.txt")
if err != nil {
    return err
}
defer file.Close() // Always closes, even if panic occurs
```

### 3. Channels and Goroutines
Channels enable safe communication between concurrent goroutines:
```go
ch := make(chan int, 1)
go func() {
    ch <- 42  // Send value
}()
value := <-ch  // Receive value
```

### 4. Context for Cancellation
Context carries deadlines and cancellation signals:
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
```

### 5. Struct Tags
Tags provide metadata for reflection-based libraries:
```go
type Person struct {
    Name string `json:"name" xml:"name"`
}
```

### 6. Prepared Statements
Prevent SQL injection and improve performance:
```go
stmt, _ := db.Prepare("INSERT INTO users(name) VALUES(?)")
stmt.Exec("Alice")
stmt.Exec("Bob")
```

### 7. Function Variables (Closures)
Functions can be stored in variables and capture surrounding scope:
```go
multiplier := 2
multiply := func(x int) int {
    return x * multiplier  // Captures 'multiplier'
}
```

### 8. Blank Identifier
Use `_` to discard values you don't need:
```go
_, err := someFunction()  // Ignore first return value
```

### 9. Map as Set
Implement sets using maps with boolean values:
```go
set := map[string]bool{
    "apple": true,
    "banana": true,
}
if set["apple"] {  // O(1) lookup
    // "apple" is in set
}
```

### 10. Graceful Shutdown Pattern
Listen for OS signals and shut down cleanly:
```go
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
<-sigChan  // Wait for signal
// Cleanup code here
```

---

## Common Go Concepts Glossary

- **Goroutine**: Lightweight thread managed by Go runtime
- **Channel**: Typed conduit for communication between goroutines
- **Defer**: Postpones function execution until surrounding function returns
- **Slice**: Dynamic array that can grow/shrink
- **Map**: Hash table (key-value store)
- **Struct**: Composite data type with named fields
- **Pointer**: Variable that holds memory address of another variable
- **Interface**: Set of method signatures (like a contract)
- **Context**: Carries deadlines, cancellation signals, and request-scoped values
- **Error**: Built-in interface type for error handling
- **Package**: Unit of code organization and reuse

---

## HTTP Status Codes Used

- **204 No Content**: Success, but no response body
- **400 Bad Request**: Client sent invalid data
- **405 Method Not Allowed**: Wrong HTTP method (e.g., GET instead of POST)
- **500 Internal Server Error**: Server error (database failure, etc.)

---

## SQL Concepts Used

- **Transaction**: Atomic unit of database operations (all-or-nothing)
- **Prepared Statement**: Pre-compiled SQL query for efficiency and security
- **Index**: Data structure that speeds up queries
- **WAL (Write-Ahead Logging)**: SQLite journaling mode that allows concurrent reads during writes
- **CHECK constraint**: Database validation rule
- **PRIMARY KEY**: Unique identifier for table rows

---

## File System Locations

- **Database**: `~/Library/Application Support/BrowserTrace/events.db`
- **Platform**: macOS (uses `Library/Application Support` convention)
- **Permissions**: `0o755` (rwxr-xr-x) - Owner has full access, others can read/execute

---

## Environment Variables

- **BROWSETRACE_ADDRESS**: Optional. Sets the server listen address (default: `127.0.0.1:51425`)

## API Endpoints

### GET /healthz
Simple health check endpoint.

**Response**: `200 OK` with body `"ok"`

### POST /events
Accepts a batch of browser events.

**Request Body**:
```json
{
  "events": [
    {
      "ts_utc": 1609459200000,
      "ts_iso": "2021-01-01T00:00:00Z",
      "url": "https://example.com",
      "title": "Example Page",
      "type": "navigate",
      "data": {}
    }
  ]
}
```

**Valid Event Types**:
- `navigate` - Page navigation
- `visible_text` - Text became visible
- `click` - User clicked
- `input` - User typed input
- `scroll` - User scrolled
- `focus` - Element received focus

**Responses**:
- `204 No Content` - Success
- `400 Bad Request` - Invalid JSON or validation failed
- `405 Method Not Allowed` - Non-POST request
- `500 Internal Server Error` - Database error

---

## Running the Program

```bash
# Run with default settings
go run main.go

# Run with custom address
BROWSETRACE_ADDRESS="0.0.0.0:8080" go run main.go

# Graceful shutdown
# Press Ctrl+C (sends SIGINT)
```

---

## Testing the API

```bash
# Health check
curl http://127.0.0.1:51425/healthz

# Send events
curl -X POST http://127.0.0.1:51425/events \
  -H "Content-Type: application/json" \
  -d '{
    "events": [{
      "ts_utc": 1609459200000,
      "ts_iso": "2021-01-01T00:00:00Z",
      "url": "https://example.com",
      "title": "Example",
      "type": "navigate",
      "data": {}
    }]
  }'
```
