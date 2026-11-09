# Architecture

## Overview

```
./memdroid
  ├── CLI (main goroutine)           — interactive menu
  ├── HTTP server (:8080)            — Web UI + REST API + WebSocket
  └── app.State (shared, mutex)
        └── driver.Driver            — ADB backend
              ├── ListProcesses()    — adb shell ps -A
              ├── Attach/Detach()    — kill -STOP / -CONT via su
              ├── Peek/Poke()        — /proc/<pid>/mem via dd + base64
              └── ReadMaps()         — adb shell cat /proc/<pid>/maps
```

## Package Structure

```text
cmd/memdroid/
└── main.go                 # Flags, ADB selection, CLI + HTTP startup
internal/
├── app/
│   ├── state.go            # Shared state and subsystem construction
│   ├── process.go          # Attach/switch/detach lifecycle shared by CLI/API
│   ├── session.go          # Search session and active value type
│   ├── persistence.go      # Save/load and live-driver rebinding
│   └── snapshots.go        # Bounded region snapshot storage
├── cli/
│   ├── run.go              # Interactive loop and shutdown
│   ├── command.go          # Action context and precondition guards
│   ├── menu.go             # One menu table for rendering and dispatch
│   ├── prompt.go           # Input parsing
│   ├── output.go           # Shared terminal output
│   └── *.go                # Device, process, search, memory, pointer,
│                           # alert, bookmark and session actions
├── driver/
│   ├── driver.go           # Device/process/memory interface and region types
│   ├── adb/                # Android Debug Bridge implementation
│   └── drivertest/         # In-memory driver for device-free tests
├── server/
│   ├── server.go           # HTTP lifecycle and watch event wiring
│   ├── routes.go           # API route table and method restrictions
│   ├── middleware.go       # Authentication, body limits and origin checks
│   ├── urls.go             # Listen address and browser URL helpers
│   ├── handler.go          # Request/response validation helpers
│   ├── paths.go            # API file-path confinement
│   ├── handlers_*.go       # One resource per file, with adjacent tests
│   └── wswatch/            # Per-server WebSocket broadcast hub
├── webui/
│   ├── webui.go            # Embedded HTTP assets; no frontend build step
│   ├── static/
│   │   ├── index.html      # Semantic markup and declarative actions
│   │   ├── styles.css      # Theme, components and responsive layout
│   │   └── js/
│   │       ├── app.js      # Action registration, navigation and startup
│   │       ├── api.js      # JSON transport and connection/auth errors
│   │       ├── ui.js       # Action dispatch, loading states and feedback
│   │       ├── status.js   # Shared status and non-overlapping polling
│   │       ├── watch.js    # Live event rendering and reconnection
│   │       └── *.js        # Feature modules: device, process, search,
│   │                       # candidates, memory, pointer, maps, bookmarks, session
│   └── tests/              # Browser-helper tests using Node's test runner
├── memory/
│   ├── search/             # Typed, pattern and string scans; candidate filtering
│   ├── pointer/            # Pointer-chain scan and resolution
│   ├── modify/             # Writes, undo, freeze, dump and snapshot diff
│   ├── watch/              # Change/alert polling and listener fan-out
│   └── store/              # Bookmarks, CheatEngine import and versioned JSON
└── poller/                 # Keyed background goroutine manager
```

## CLI and Application Boundaries

`cmd/memdroid/main.go` is the composition root. It configures the ADB driver,
shared state and HTTP server, then runs the CLI. The displayed server URL is
passed to the CLI explicitly instead of stored in a package variable.

The CLI's `menu.go` holds each key, label and action together. `command.go`
resolves fresh state for each action and applies attached/session guards.
Feature handlers live beside related commands rather than in the menu loop.

Both CLI and API call `app.State.Attach`, `SwitchProcess`, `Detach`, `Save` and
`Load`. Process transitions are serialized independently of the state read
lock, keeping slow device I/O from blocking status reads. A loaded session is
rebound to the live driver before either frontend can use it.

## Web UI Boundaries

`webui` owns the embedded browser assets; `server` owns HTTP routing and API
behavior. All assets still ship in a single Go binary. Native ES modules keep
feature state private without a JavaScript framework or bundler.

Markup uses `data-click`, `data-change`, `data-input` and `data-enter` actions.
`app.js` binds those names to module exports; `ui.js` handles event delegation,
loading states and notifications. Dynamic row arguments are escaped JSON data,
so process names, bookmark values and addresses never become executable code.

The setup strip links device selection, process attachment and searching.
Status controls operation availability, value types are persisted through the
API, and search results link to paginated candidates. Opening a data panel
refreshes it automatically. Search/typing operations share a busy group to
prevent overlapping session mutations. Status polling waits for each request
to finish, and the watch socket reconnects with bounded backoff.

## Dependency Graph

```
cmd/memdroid
 ├── app.State
 ├── cli
 ├── driver/adb              (no internal deps — exec.Command("adb", ...) only)
 ├── server                  → app, driver, driver/adb, memory/*, server/wswatch, webui
 ├── memory/search           → driver
 ├── memory/pointer          → driver
 ├── memory/modify           → driver, memory/search, poller
 ├── memory/watch            → driver, memory/search, poller
 └── memory/store            → driver, memory/search
```

No circular imports.

## Key Design Decisions

### Driver Interface

All device I/O is behind `driver.Driver`. Higher-level packages (search, modify, etc.)
never call `adb` directly. This decouples the logic from the transport and makes it
straightforward to add a local ptrace backend in the future.

```go
type Driver interface {
    ListProcesses() ([]ProcessInfo, error)
    Attach(pid int) error
    Detach(pid int)
    Peek(pid int, addr uintptr, size int) ([]byte, error)
    Poke(pid int, addr uintptr, data []byte) error
    ReadMaps(pid int) ([]Region, error)
    ReadMapsFiltered(pid int, filter RegionFilter, ...) ([]Region, error)
    ReadBytes(pid int, addr uintptr, n int) ([]byte, error)
    ReadRegion(pid int, addr uintptr, size int) ([]byte, error)
    ...
}
```

### ADB Memory Access

`/proc/<pid>/mem` gives direct access to a process's virtual address space.
ADB transmits data as text, so raw bytes are wrapped in base64:

```
Peek: su -c 'dd if=/proc/<pid>/mem ... | base64'  → base64.Decode()
Poke: echo <base64> | base64 -d | dd of=/proc/<pid>/mem ...
```

Attach/Detach use `kill -STOP` / `kill -CONT` because ptrace is not available
from the host side over ADB.

### Shared State (app.State)

`app.State` holds the single source of truth protected by `sync.RWMutex`.
Both the CLI goroutine and all HTTP handler goroutines read/write through it.

```
State.GetDriver()     → current ADB driver
State.GetSession()    → current search session (nil if not attached)
State.GetPID()        → attached PID (0 if none)
State.NewSession(pid) → replace the session with a fresh one
State.EnsureSession() → creates session if nil
State.Attach(pid, n)  → attach, activate and create a fresh session
State.SwitchProcess(p)→ validate and activate an attached process
State.Detach()        → unfreeze/unwatch, detach, promote the next process
State.Save/Load(path) → persist and restore with the live driver
```

`State.Detach()` exists because the CLI and the API both need exactly that
sequence; keeping it in one place stops the two paths from drifting.

### Value Type Ownership

The active value type has exactly one owner at a time. Before a session exists,
`State` holds it; once a session exists, the **session** owns it and
`State.GetValueType()` delegates. `State.SetValueType` propagates into the live
session, and `Session.SetType` discards existing candidates because they were
recorded at a different byte width.

This matters because a scan and the formatting of its results must agree on the
width. Storing the type in two places let them disagree: setting the type over
the API updated `State` while the session kept scanning at the old width.

### Search Session

`Session` holds `map[uintptr][]byte` (address → last-seen value bytes). All
fields are unexported and mutex-guarded; long scans do their I/O without
holding the lock and take it only to swap in the finished result.

```
Session.Search(target)               → full scan, reset candidate map
Session.SearchFiltered(target, ...)  → restricted to a region type
Session.Filter(mode, ...)            → re-read each candidate, drop non-matching
Session.Page(offset, limit)          → one sorted page, without copying the rest
Session.Snapshot()                   → deep copy for concurrent reads
Session.Reset()                      → clear all candidates
```

`Page` exists so the API can serve `/api/search/candidates` without deep-copying
a multi-million-entry candidate map on every request.

### Pointer Scan Algorithm

1. Read all rw memory regions via ReadMaps
2. Scan every pointer-aligned 8-byte value in every region → build `ptrMap[value] = []srcAddr`
3. Walk backwards from `targetAddr` up to `maxDepth` levels:
   - For each delta in `[0, maxOffset]` (step 8), look up `ptrMap[targetAddr - delta]`
   - For each source address found, check if it lies in a static region (named .so / module)
   - If yes, record the chain; if not, recurse deeper
4. Return all found chains as `[]Chain{BaseAddr, BaseLabel, Offsets, FinalAddr}`

### Watch Event Fan-out

`Watcher` and `AlertWatcher` expose `OnChange` / `OnAlert` as **registrations**
that return an unsubscribe func, backed by the generic `listeners[T]` registry:

```go
remove := state.Watcher.OnChange(func(ev watch.ChangeEvent) { ... })
```

Watch events have two consumers — the CLI prints them, and the server forwards
them to `wswatch.Hub` for the browser — and they are wired up from different
goroutines at startup. A single assignable callback field let whichever side
registered last silently win (the server's registration clobbered the CLI's),
and was raced on by the polling goroutines that invoked it.

`wswatch.Hub` is a value rather than package state, so a test — or a second
server — gets its own client set.

### Freeze / Watch Goroutines

Both use the ticker+stop-channel pattern:

```go
ticker := time.NewTicker(interval)
for {
    select {
    case <-ticker.C:  /* do work */
    case <-e.stop:    return
    }
}
```

`UnfreezeAll` / `UnwatchAll` close all stop channels. These are called on detach and exit.

### The Shared Scan Engine

Value, byte-sequence, pattern and string scans all run through one function,
`search.scanRegions`. It reads each region via `ReadRegion` — one `adb shell`
round-trip per 32 MiB rather than one per address — fans regions out across
`scanWorkers` goroutines, and merges the per-worker match maps at the end.

A full scan issues ~50 round-trips (one per rw region) instead of millions,
bringing scan time from hours to seconds.

Two details the engine handles so callers don't have to:

- **Chunking.** A region larger than `scanChunkBytes` (32 MiB) is read in
  pieces that overlap by `width-1` bytes, so a match straddling a chunk
  boundary is still found. Without the cap, one multi-gigabyte mapping times
  `scanWorkers` would exhaust RAM.
- **Alignment.** Fixed-width types are scanned on their natural alignment (as
  Cheat Engine does); `bytes` uses stride 1. The stride offset is computed from
  the *region* base, not the chunk base, so chunking cannot shift which offsets
  a scan visits.

`Filter` works the other way round: it builds a region → `[]byte` cache from
`ReadMaps`, binary-searches each candidate into its region, slices the current
bytes out in memory, and falls back to `Peek` only for addresses whose region
is missing (unmapped since the scan, or failed to read).

### Pattern & String Search

`SearchPattern` (byte pattern with `??` wildcards) runs on the same engine, and
`SearchString` compiles a UTF-8 or UTF-16LE string into a wildcard-free pattern.

Both cap results at `PatternMaxResults` and report `Truncated` rather than
silently returning a prefix. Results carry the matched **bytes** alongside each
address, so the API can seed the search session directly instead of re-reading
every hit.

## Testing

`internal/driver/drivertest` provides an in-memory `driver.Driver`. Everything
above the driver layer is pure logic over a byte-addressed process, so search,
filter, pointer scanning and the HTTP handlers are all tested without an
Android device or an `adb` binary:

```go
f := drivertest.New(drivertest.Region{Start: 0x1000, Name: "[heap]", Data: data})
st := app.NewState(f)
```

The fake records `Peeks` / `Pokes` / `RegionReads`, so a test can assert that a
change actually reduced transport round-trips. CI runs `go test -race`; the
scan engine, the poller pool and `app.State` are all concurrent, which is the
point of that flag.

## API Reference

See [docs/api.md](api.md) for the full REST + WebSocket endpoint list.
