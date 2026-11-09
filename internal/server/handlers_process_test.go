package server

import (
	"net/http"
	"testing"
)

func TestMapsList(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	got := e.arr(http.MethodGet, "/api/maps", "", http.StatusOK)
	if len(got) != 1 {
		t.Fatalf("maps = %d, want 1", len(got))
	}
	m := object(t, got[0])
	if start := str(t, m, "start"); start != "0x1000" {
		t.Errorf("start = %q, want %q", start, "0x1000")
	}
	if end := str(t, m, "end"); end != "0x1400" {
		t.Errorf("end = %q, want %q", end, "0x1400")
	}
	if size := num(t, m, "size"); size != heapSize {
		t.Errorf("size = %v, want %d", size, heapSize)
	}
	if name := str(t, m, "name"); name != "[heap]" {
		t.Errorf("name = %q, want %q", name, "[heap]")
	}
}

func TestProcessAttachSwitchDetach(t *testing.T) {
	e := newEnv(t, Config{})

	list := e.arr(http.MethodGet, "/api/process/list", "", http.StatusOK)
	if len(list) != 1 {
		t.Fatalf("processes = %d, want 1", len(list))
	}
	p := object(t, list[0])
	if pid := num(t, p, "pid"); pid != 1 {
		t.Errorf("pid = %v, want 1", pid)
	}
	if name := str(t, p, "name"); name != "com.example.fake" {
		t.Errorf("name = %q, want %q", name, "com.example.fake")
	}

	if msg := e.errMsg(http.MethodPost, "/api/process/attach", `{"pid":0}`, http.StatusBadRequest); msg != "invalid pid" {
		t.Errorf("error = %q, want %q", msg, "invalid pid")
	}
	if msg := e.errMsg(http.MethodPost, "/api/process/attach", `{"pid":-3}`, http.StatusBadRequest); msg != "invalid pid" {
		t.Errorf("error = %q, want %q", msg, "invalid pid")
	}

	e.attach()
	if !e.fake.Attached(1) {
		t.Fatal("fake driver reports pid 1 as not attached")
	}

	attached := e.arr(http.MethodGet, "/api/process/attached", "", http.StatusOK)
	if len(attached) != 1 {
		t.Fatalf("attached = %d, want 1", len(attached))
	}
	a := object(t, attached[0])
	if pid := num(t, a, "pid"); pid != 1 {
		t.Errorf("pid = %v, want 1", pid)
	}
	if active, ok := a["active"].(bool); !ok || !active {
		t.Errorf("active = %v, want true", a["active"])
	}

	if msg := e.errMsg(http.MethodPost, "/api/process/switch", `{"pid":99}`, http.StatusBadRequest); msg != "pid is not attached" {
		t.Errorf("error = %q, want %q", msg, "pid is not attached")
	}
	e.obj(http.MethodPost, "/api/process/switch", `{"pid":1}`, http.StatusOK)

	got := e.obj(http.MethodPost, "/api/process/detach", "", http.StatusOK)
	if d := num(t, got, "detached"); d != 1 {
		t.Errorf("detached = %v, want 1", d)
	}
	if e.fake.Attached(1) {
		t.Error("fake driver still reports pid 1 as attached")
	}
	if boolean(t, e.obj(http.MethodGet, "/api/status", "", http.StatusOK), "attached") {
		t.Error("status still reports attached")
	}
}

func TestProcessStopContinue(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	for _, path := range []string{"/api/process/stop", "/api/process/continue"} {
		if !boolean(t, e.obj(http.MethodPost, path, "", http.StatusOK), "ok") {
			t.Errorf("%s: ok = false", path)
		}
	}
}
