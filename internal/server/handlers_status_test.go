package server

import (
	"net/http"
	"testing"
)

func TestStatusDetached(t *testing.T) {
	e := newEnv(t, Config{})

	got := e.obj(http.MethodGet, "/api/status", "", http.StatusOK)

	if pid := num(t, got, "pid"); pid != 0 {
		t.Errorf("pid = %v, want 0", pid)
	}
	if boolean(t, got, "attached") {
		t.Error("attached = true, want false")
	}
	if vt := str(t, got, "value_type"); vt != "int32" {
		t.Errorf("value_type = %q, want %q", vt, "int32")
	}
	if c := num(t, got, "candidates"); c != 0 {
		t.Errorf("candidates = %v, want 0", c)
	}
	if d := num(t, got, "undo_depth"); d != 0 {
		t.Errorf("undo_depth = %v, want 0", d)
	}
	for _, key := range []string{"frozen", "watched", "alerts"} {
		if l := slice(t, got, key); len(l) != 0 {
			t.Errorf("%s = %v, want empty", key, l)
		}
	}
	if dev := str(t, got, "device"); dev != "" {
		t.Errorf("device = %q, want empty", dev)
	}
}

func TestStatusAttached(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	if !e.fake.Attached(1) {
		t.Fatal("driver was not asked to attach")
	}

	got := e.obj(http.MethodGet, "/api/status", "", http.StatusOK)
	if pid := num(t, got, "pid"); pid != 1 {
		t.Errorf("pid = %v, want 1", pid)
	}
	if !boolean(t, got, "attached") {
		t.Error("attached = false, want true")
	}
	if c := num(t, got, "candidates"); c != 0 {
		t.Errorf("candidates = %v, want 0", c)
	}
}

// TestNotAttached checks the guard on every endpoint that needs a live process.
func TestNotAttached(t *testing.T) {
	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/maps", ""},
		{http.MethodGet, "/api/memory/hexdump?addr=0x1000&size=16", ""},
		{http.MethodPost, "/api/search/value", `{"value":"1337"}`},
		{http.MethodPost, "/api/search/pattern", `{"pattern":"39 05"}`},
		{http.MethodPost, "/api/search/string", `{"value":"hi"}`},
		{http.MethodPost, "/api/memory/modify", `{"addr":"0x1000","value":"1"}`},
		{http.MethodPost, "/api/memory/write-string", `{"addr":"0x1000","value":"hi"}`},
		{http.MethodPost, "/api/memory/freeze", `{"addr":"0x1000","value":"1"}`},
		{http.MethodPost, "/api/memory/dump", `{"addr":"0x1000","size":16}`},
		{http.MethodPost, "/api/snapshot/take", `{"addr":"0x1000","size":16}`},
		{http.MethodPost, "/api/snapshot/diff", `{"addr":"0x1000","size":16}`},
		{http.MethodPost, "/api/watch/add", `{"addr":"0x1000"}`},
		{http.MethodPost, "/api/alert/add", `{"addr":"0x1000","condition":"changed","action":"notify"}`},
		{http.MethodPost, "/api/pointer/scan", `{"addr":"0x1000"}`},
		{http.MethodPost, "/api/pointer/resolve", `{"label":"[heap]","offsets":[0]}`},
		{http.MethodPost, "/api/bookmark/modify-all", `{"value":"1"}`},
		{http.MethodPost, "/api/process/stop", ""},
		{http.MethodPost, "/api/process/continue", ""},
		{http.MethodPost, "/api/process/detach", ""},
	}

	e := newEnv(t, Config{})
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			if msg := e.errMsg(tc.method, tc.path, tc.body, http.StatusBadRequest); msg != "not attached" {
				t.Errorf("error = %q, want %q", msg, "not attached")
			}
		})
	}
}
