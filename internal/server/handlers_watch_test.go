package server

import (
	"net/http"
	"testing"
)

func TestWatchAddListRemove(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	// A long interval keeps the background poller from doing real work during
	// the test; the endpoint contract is what is under test here.
	got := e.obj(http.MethodPost, "/api/watch/add", `{"addr":"0x1010","interval_ms":3600000}`, http.StatusOK)
	if addr := str(t, got, "addr"); addr != "0x1010" {
		t.Errorf("addr = %q, want %q", addr, "0x1010")
	}
	if vt := str(t, got, "value_type"); vt != "int32" {
		t.Errorf("value_type = %q, want %q", vt, "int32")
	}
	if ms := num(t, got, "interval_ms"); ms != 3600000 {
		t.Errorf("interval_ms = %v, want 3600000", ms)
	}

	list := e.arr(http.MethodGet, "/api/watch/list", "", http.StatusOK)
	if len(list) != 1 || list[0] != "0x1010" {
		t.Fatalf("watch list = %v, want [0x1010]", list)
	}
	if watched := slice(t, e.obj(http.MethodGet, "/api/status", "", http.StatusOK), "watched"); len(watched) != 1 {
		t.Errorf("status watched = %v, want one entry", watched)
	}

	e.obj(http.MethodPost, "/api/watch/remove", `{"addr":"0x1010"}`, http.StatusOK)
	if list := e.arr(http.MethodGet, "/api/watch/list", "", http.StatusOK); len(list) != 0 {
		t.Errorf("watch list after remove = %v, want empty", list)
	}
	// Removing an address that is not watched is a client error.
	if msg := e.errMsg(http.MethodPost, "/api/watch/remove", `{"addr":"0x1010"}`, http.StatusBadRequest); msg == "" {
		t.Error("removing an unwatched address returned no error")
	}
}

func TestWatchAddRejectsNegativeInterval(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	msg := e.errMsg(http.MethodPost, "/api/watch/add", `{"addr":"0x1010","interval_ms":-5}`, http.StatusBadRequest)
	if msg != "interval_ms must be positive" {
		t.Errorf("error = %q, want %q", msg, "interval_ms must be positive")
	}
}

func TestAlertAddListRemove(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	body := `{"addr":"0x1010","condition":"above","threshold":"9000","action":"notify","interval_ms":3600000}`
	got := e.obj(http.MethodPost, "/api/alert/add", body, http.StatusOK)
	if addr := str(t, got, "addr"); addr != "0x1010" {
		t.Errorf("addr = %q, want %q", addr, "0x1010")
	}

	if list := e.arr(http.MethodGet, "/api/alert/list", "", http.StatusOK); len(list) != 1 {
		t.Fatalf("alert list = %v, want one entry", list)
	}

	e.obj(http.MethodPost, "/api/alert/remove", `{"addr":"0x1010"}`, http.StatusOK)
	if list := e.arr(http.MethodGet, "/api/alert/list", "", http.StatusOK); len(list) != 0 {
		t.Errorf("alert list after remove = %v, want empty", list)
	}
}

func TestAlertAddRejectsBadInput(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	tests := []struct {
		name string
		body string
	}{
		{"unknown condition", `{"addr":"0x1010","condition":"sideways","action":"notify"}`},
		{"unknown action", `{"addr":"0x1010","condition":"changed","action":"explode"}`},
		{"threshold not parseable", `{"addr":"0x1010","condition":"above","threshold":"abc","action":"notify"}`},
		{"malformed addr", `{"addr":"nope","condition":"changed","action":"notify"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if msg := e.errMsg(http.MethodPost, "/api/alert/add", tc.body, http.StatusBadRequest); msg == "" {
				t.Error("no error message")
			}
		})
	}
}
