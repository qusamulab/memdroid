# Development Guide

## Requirements

- Go 1.26+
- Node.js 22+ to run the browser-helper tests (not needed to build or run memdroid)
- macOS, Linux, or Windows (host PC)
- [pre-commit](https://pre-commit.com/) for local hooks

## Setup

```bash
git clone https://github.com/qusamulab/memdroid.git
cd memdroid
go mod download

# Install pre-commit hooks
brew install pre-commit   # or: pip install pre-commit
pre-commit install
```

## Build

```bash
go build -o memdroid ./cmd/memdroid
```

## Pre-commit Hooks

Configured in [.pre-commit-config.yaml](https://github.com/qusamulab/memdroid/blob/main/.pre-commit-config.yaml):

| Hook           | What it does                              |
|----------------|-------------------------------------------|
| `go-fmt`       | Formats all `.go` files with `gofmt`      |
| `go-vet`       | Runs `go vet ./...`                       |
| `go-imports`   | Cleans up import blocks with `goimports`  |
| `golangci-lint`| Runs linters defined in `.golangci.yml`   |

Run manually at any time:

```bash
pre-commit run --all-files
```

Linter config: [.golangci.yml](https://github.com/qusamulab/memdroid/blob/main/.golangci.yml)  
Enabled: `gofmt`, `goimports`, `govet`, `errcheck`, `staticcheck`

## Verification

```bash
go test -race -count=1 ./...
go vet ./...
go build ./...
node --test internal/webui/tests/*.test.mjs
```

Go tests use an in-memory driver, so an Android device and `adb` are not
required. UI asset tests walk the embedded module graph and verify that every
referenced file is served with browser-compatible MIME types. The Node tests
cover API errors and escaping of dynamic action arguments.

For visual checks, run `go run ./cmd/memdroid` and open the displayed URL.
Check device selection, attach, search/filter, candidate pagination, editing
and undo at desktop and narrow widths. An absent `adb` or device should display
an actionable error in the device list.

## Adding a New Feature

1. Put reusable behavior in the appropriate package:
   - `internal/app/` — operations shared by CLI and HTTP, process lifecycle
   - `internal/memory/search/` — scanning and filtering
   - `internal/memory/modify/` — writes, undo, freeze, snapshots and dumps
   - `internal/memory/watch/` — background monitoring
   - `internal/memory/store/` — bookmarks, imports and file formats
   - `internal/driver/adb/` — ADB transport

2. Add CLI prompts/output to the matching file in `internal/cli/` and register
   its key, label and action together in `menu.go`.

3. Add HTTP handlers to `internal/server/handlers_<resource>.go` and register
   their paths in `routes.go`. Keep resource tests beside the handlers.

4. Add browser actions to a feature module under `internal/webui/static/js/`.
   Register new modules in `app.js`, use declarative action attributes in the
   markup, and keep styling in `styles.css`. Assets are embedded automatically;
   restart the Go server and reload the browser after editing them.

5. Update [Usage](usage.md), [API Reference](api.md) and
   [Architecture](architecture.md) when behavior or structure changes.

## Commit Convention

Write descriptive commit messages. Format:

```
<Package>: <short summary>

- detail 1
- detail 2
```

Example:
```
search/filter.go: add fuzzy float comparison

- Treat float32/64 values within 1e-6 as equal to handle
  floating-point drift in game engines.
```
