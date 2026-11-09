package server

import (
	"net/http"
	"testing"
)

func TestFreezeUnfreeze(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	e.obj(http.MethodPost, "/api/memory/freeze-interval", `{"interval_ms":3600000}`, http.StatusOK)
	if msg := e.errMsg(http.MethodPost, "/api/memory/freeze-interval", `{"interval_ms":0}`, http.StatusBadRequest); msg != "interval_ms must be positive" {
		t.Errorf("error = %q, want %q", msg, "interval_ms must be positive")
	}

	e.obj(http.MethodPost, "/api/memory/freeze", `{"addr":"0x1010","value":"55"}`, http.StatusOK)
	if list := e.arr(http.MethodGet, "/api/memory/frozen", "", http.StatusOK); len(list) != 1 || list[0] != "0x1010" {
		t.Fatalf("frozen = %v, want [0x1010]", list)
	}

	e.obj(http.MethodPost, "/api/memory/unfreeze", `{"addr":"0x1010"}`, http.StatusOK)
	if list := e.arr(http.MethodGet, "/api/memory/frozen", "", http.StatusOK); len(list) != 0 {
		t.Errorf("frozen after unfreeze = %v, want empty", list)
	}
	if msg := e.errMsg(http.MethodPost, "/api/memory/unfreeze", `{"addr":"0x1010"}`, http.StatusBadRequest); msg == "" {
		t.Error("unfreezing an unfrozen address returned no error")
	}
}
