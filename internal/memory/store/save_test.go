package store

import (
	"os"
	"path/filepath"
	"testing"

	"memdroid/internal/memory/search"
)

func TestInvalidSavedSessionDoesNotReplaceBookmarks(t *testing.T) {
	for _, session := range []string{
		`{"pid":1,"value_type":999,"candidates":{}}`,
		`{"pid":0,"value_type":0,"candidates":{}}`,
		`{"pid":1,"value_type":0,"candidates":{"invalid":"AQAAAA=="}}`,
		`{"pid":1,"value_type":0,"candidates":{"0x1000":"AQ=="}}`,
	} {
		t.Run(session, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			data := `{"version":2,"bookmarks":[{"addr":4096,"label":"new","vtype":0}],"session":` + session + `}`
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			bl := NewBookmarkList()
			bl.Add(0x2000, "current", search.TypeInt32)
			if _, err := LoadState(path, bl); err == nil {
				t.Fatal("invalid session accepted")
			}
			if bl.Entries()[0].Label != "current" {
				t.Fatal("failed load changed bookmarks")
			}
		})
	}
}
func TestLegacyBookmarksRemainReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"bookmarks":[{"addr":4096,"label":"saved","vtype":0}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	bl := NewBookmarkList()
	if _, err := LoadState(path, bl); err != nil || bl.Len() != 1 {
		t.Fatalf("Load=%v, bookmarks=%d", err, bl.Len())
	}
}
