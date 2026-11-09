package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	e := newEnv(t, Config{FileRoot: root})
	e.attach()

	e.obj(http.MethodPost, "/api/bookmark/add", `{"addr":"0x1010","label":"hp"}`, http.StatusOK)
	if c := num(t, e.obj(http.MethodPost, "/api/search/value", `{"value":"1337"}`, http.StatusOK), "candidates"); c != 3 {
		t.Fatalf("candidates = %v, want 3", c)
	}

	saved := e.obj(http.MethodPost, "/api/session/save", `{"path":"state.json"}`, http.StatusOK)
	wantPath := filepath.Join(mustAbs(t, root), "state.json")
	if got := str(t, saved, "path"); got != wantPath {
		t.Fatalf("saved path = %q, want %q", got, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("state file: %v", err)
	}

	// Throw away everything that was saved.
	e.obj(http.MethodPost, "/api/bookmark/remove", `{"index":0}`, http.StatusOK)
	e.obj(http.MethodPost, "/api/search/reset", "", http.StatusOK)
	if c := num(t, e.obj(http.MethodGet, "/api/status", "", http.StatusOK), "candidates"); c != 0 {
		t.Fatal("candidates were not cleared before the load")
	}

	loaded := e.obj(http.MethodPost, "/api/session/load", `{"path":"state.json"}`, http.StatusOK)
	if got := str(t, loaded, "path"); got != wantPath {
		t.Errorf("loaded path = %q, want %q", got, wantPath)
	}
	if c := num(t, loaded, "candidates"); c != 3 {
		t.Fatalf("loaded candidates = %v, want 3", c)
	}

	if c := num(t, e.obj(http.MethodGet, "/api/status", "", http.StatusOK), "candidates"); c != 3 {
		t.Errorf("status candidates after load = %v, want 3", c)
	}
	list := e.arr(http.MethodGet, "/api/bookmark/list", "", http.StatusOK)
	if len(list) != 1 {
		t.Fatalf("bookmarks after load = %d, want 1", len(list))
	}
	if label := str(t, object(t, list[0]), "label"); label != "hp" {
		t.Errorf("label = %q, want %q", label, "hp")
	}

	// The restored session is rebound to the live driver, so it can still filter.
	if c := num(t, e.obj(http.MethodPost, "/api/search/filter", `{"mode":"unchanged"}`, http.StatusOK), "candidates"); c != 3 {
		t.Errorf("candidates after filtering a restored session = %v, want 3", c)
	}
}

func TestSessionSaveDefaultsToTheStateFileName(t *testing.T) {
	root := t.TempDir()
	e := newEnv(t, Config{FileRoot: root})

	// An empty body is allowed: every field is optional.
	got := e.obj(http.MethodPost, "/api/session/save", "", http.StatusOK)
	wantPath := filepath.Join(mustAbs(t, root), "memdroid.json")
	if p := str(t, got, "path"); p != wantPath {
		t.Errorf("path = %q, want %q", p, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("state file: %v", err)
	}
}

func TestSessionPathsAreConfined(t *testing.T) {
	root := t.TempDir()
	e := newEnv(t, Config{FileRoot: root})

	for _, path := range []string{"/api/session/save", "/api/session/load"} {
		t.Run(path, func(t *testing.T) {
			msg := e.errMsg(http.MethodPost, path, `{"path":"/etc/passwd"}`, http.StatusBadRequest)
			if !strings.Contains(msg, "absolute paths are not allowed") {
				t.Errorf("error = %q, want it to reject the absolute path", msg)
			}
		})
	}

	// Traversal is rejected outright, and nothing is written either inside or
	// outside the root.
	msg := e.errMsg(http.MethodPost, "/api/session/save", `{"path":"../escape.json"}`, http.StatusBadRequest)
	if !strings.Contains(msg, "escapes") {
		t.Errorf("error = %q, want it to report the escape", msg)
	}
	for _, dir := range []string{mustAbs(t, root), filepath.Dir(mustAbs(t, root))} {
		if _, err := os.Stat(filepath.Join(dir, "escape.json")); err == nil {
			t.Errorf("escape.json was written to %s", dir)
		}
	}
}

// TestSessionPathsRejectSymlinkEscape covers the case a pure lexical check
// misses: a link that lives inside the root but points out of it.
func TestSessionPathsRejectSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	e := newEnv(t, Config{FileRoot: root})

	msg := e.errMsg(http.MethodPost, "/api/session/save", `{"path":"link/state.json"}`, http.StatusBadRequest)
	if !strings.Contains(msg, "escapes") {
		t.Errorf("error = %q, want it to report the escape", msg)
	}
	if _, err := os.Stat(filepath.Join(outside, "state.json")); err == nil {
		t.Error("a file escaped the file root through a symlink")
	}
}

func TestSessionLoadMissingFile(t *testing.T) {
	e := newEnv(t, Config{FileRoot: t.TempDir()})
	msg := e.errMsg(http.MethodPost, "/api/session/load", `{"path":"absent.json"}`, http.StatusInternalServerError)
	if !strings.Contains(msg, "load state") {
		t.Errorf("error = %q, want it to name the operation", msg)
	}
}
