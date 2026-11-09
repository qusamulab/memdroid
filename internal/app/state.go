package app

import (
	"sync"

	"memdroid/internal/driver"
	"memdroid/internal/memory/modify"
	"memdroid/internal/memory/search"
	"memdroid/internal/memory/store"
	"memdroid/internal/memory/watch"
)

// State is the single shared mutable state of the tool.
// Both the CLI and HTTP server read/write through this struct.
type State struct {
	processMu sync.Mutex // serializes attach, switch and detach across CLI/API
	mu        sync.RWMutex
	drv       driver.Driver
	pid       int
	valueType search.ValueType
	session   *search.Session
	bookmarks *store.BookmarkList
	snapshots *snapshotStore
	attached  map[int]string // pid -> name, all currently attached processes

	Freezer      *modify.Freezer
	UndoStack    *modify.UndoStack
	Watcher      *watch.Watcher
	AlertWatcher *watch.AlertWatcher
}

func NewState(drv driver.Driver) *State {
	return &State{
		drv:          drv,
		valueType:    search.TypeInt32,
		bookmarks:    store.NewBookmarkList(),
		snapshots:    newSnapshotStore(maxSnapshotBytes),
		attached:     make(map[int]string),
		Freezer:      modify.NewFreezer(),
		UndoStack:    modify.NewUndoStack(),
		Watcher:      watch.NewWatcher(),
		AlertWatcher: watch.NewAlertWatcher(),
	}
}

// --- Driver ---

func (s *State) GetDriver() driver.Driver {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.drv
}

// --- Bookmarks ---

func (s *State) GetBookmarks() *store.BookmarkList {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bookmarks
}
