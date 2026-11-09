package app

import (
	"path/filepath"
	"testing"

	"memdroid/internal/memory/search"
)

func TestLoadRebindsSessionToLiveDriver(t *testing.T) {
	state, fake := newTestState()
	if err := state.Attach(1, "app"); err != nil {
		t.Fatal(err)
	}
	state.GetBookmarks().Add(0x1000, "health", search.TypeInt32)
	state.GetSession().SetCandidatesAs(search.TypeInt32, map[uintptr][]byte{0x1000: {0, 0, 0, 0}})
	path := filepath.Join(t.TempDir(), "state.json")
	if err := state.Save(path); err != nil {
		t.Fatal(err)
	}
	restored := NewState(fake)
	count, err := restored.Load(path)
	if err != nil || count != 1 {
		t.Fatalf("Load = %d, %v; want 1, nil", count, err)
	}
	if restored.GetBookmarks().Len() != 1 {
		t.Fatal("bookmark was not restored")
	}
	if err := fake.Poke(1, 0x1000, []byte{42, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if err := restored.GetSession().Filter(search.FilterChanged, nil); err != nil {
		t.Fatalf("restored session cannot read the live driver: %v", err)
	}
	if restored.GetSession().CandidateCount() != 1 {
		t.Fatal("restored session did not observe the changed value")
	}
}

func TestLoadFailureKeepsExistingSession(t *testing.T) {
	state, _ := newTestState()
	previous := state.EnsureSession()
	if _, err := state.Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing file should fail")
	}
	if state.GetSession() != previous {
		t.Fatal("failed load replaced the active session")
	}
}
