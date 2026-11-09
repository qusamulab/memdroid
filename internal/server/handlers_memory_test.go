package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryHexdump(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	got := e.obj(http.MethodGet, "/api/memory/hexdump?addr=0x1000&size=32", "", http.StatusOK)
	if addr := str(t, got, "addr"); addr != "0x1000" {
		t.Errorf("addr = %q, want %q", addr, "0x1000")
	}
	if size := num(t, got, "size"); size != 32 {
		t.Errorf("size = %v, want 32", size)
	}

	lines := slice(t, got, "hex_lines")
	if len(lines) != 2 {
		t.Fatalf("hex_lines = %d, want 2", len(lines))
	}

	first := object(t, lines[0])
	if off := num(t, first, "offset"); off != 0 {
		t.Errorf("line 0 offset = %v, want 0", off)
	}
	if addr := str(t, first, "addr"); addr != "0x1000" {
		t.Errorf("line 0 addr = %q, want %q", addr, "0x1000")
	}
	if hex := str(t, first, "hex"); hex != strings.TrimSpace(strings.Repeat("00 ", 16)) {
		t.Errorf("line 0 hex = %q, want 16 zero bytes", hex)
	}
	if ascii := str(t, first, "ascii"); ascii != strings.Repeat(".", 16) {
		t.Errorf("line 0 ascii = %q, want 16 dots", ascii)
	}

	// The second line starts at 0x1010, where the first int32 1337 lives.
	second := object(t, lines[1])
	if off := num(t, second, "offset"); off != 16 {
		t.Errorf("line 1 offset = %v, want 16", off)
	}
	if addr := str(t, second, "addr"); addr != "0x1010" {
		t.Errorf("line 1 addr = %q, want %q", addr, "0x1010")
	}
	if hex := str(t, second, "hex"); !strings.HasPrefix(hex, "39 05 00 00") {
		t.Errorf("line 1 hex = %q, want it to start with the little-endian 1337", hex)
	}
}

func TestMemoryHexdumpBounds(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	tests := []struct {
		name    string
		query   string
		wantSub string
	}{
		{"missing addr", "?size=16", "addr required"},
		{"malformed addr", "?addr=0xzz&size=16", "invalid addr"},
		{"size zero", "?addr=0x1000&size=0", "size must be 1-4096"},
		{"size negative", "?addr=0x1000&size=-1", "size must be 1-4096"},
		{"size absent", "?addr=0x1000", "size must be 1-4096"},
		{"size above the cap", "?addr=0x1000&size=4097", "size must be 1-4096"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := e.errMsg(http.MethodGet, "/api/memory/hexdump"+tc.query, "", http.StatusBadRequest)
			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("error = %q, want it to contain %q", msg, tc.wantSub)
			}
		})
	}

	// The upper bound itself is accepted (the region short-reads, which is fine).
	if rec := e.rec(http.MethodGet, "/api/memory/hexdump?addr=0x1000&size=4096", ""); rec.Code != http.StatusOK {
		t.Errorf("size=4096: status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestMemoryWriteString(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	got := e.obj(http.MethodPost, "/api/memory/write-string", `{"addr":"0x1080","value":"ZZZZ"}`, http.StatusOK)
	if n := num(t, got, "bytes"); n != 4 {
		t.Errorf("bytes = %v, want 4", n)
	}
	if s := string(e.fake.Bytes(0x1080, 8)); s != "ZZZZROID" {
		t.Errorf("memory at 0x1080 = %q, want %q", s, "ZZZZROID")
	}

	if msg := e.errMsg(http.MethodPost, "/api/memory/write-string", `{"addr":"0x1080","value":""}`, http.StatusBadRequest); msg != "value required" {
		t.Errorf("error = %q, want %q", msg, "value required")
	}
}

func TestMemoryDump(t *testing.T) {
	root := t.TempDir()
	e := newEnv(t, Config{FileRoot: root})
	e.attach()

	got := e.obj(http.MethodPost, "/api/memory/dump", `{"addr":"0x1010","size":16,"path":"dump.hex"}`, http.StatusOK)
	wantPath := filepath.Join(mustAbs(t, root), "dump.hex")
	if p := str(t, got, "path"); p != wantPath {
		t.Fatalf("path = %q, want %q", p, wantPath)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	if !strings.Contains(string(data), "0000000000001010") {
		t.Errorf("dump = %q, want it to contain the base address", data)
	}
	if !strings.Contains(string(data), "39 05 00 00") {
		t.Errorf("dump = %q, want it to contain the dumped bytes", data)
	}

	if msg := e.errMsg(http.MethodPost, "/api/memory/dump", `{"addr":"0x1010","size":0}`, http.StatusBadRequest); !strings.Contains(msg, "size must be 1-") {
		t.Errorf("error = %q, want a size complaint", msg)
	}
	if msg := e.errMsg(http.MethodPost, "/api/memory/dump", `{"addr":"0x1010","size":16,"path":"/tmp/escape.hex"}`, http.StatusBadRequest); !strings.Contains(msg, "absolute paths are not allowed") {
		t.Errorf("error = %q, want the path to be rejected", msg)
	}
}

func TestMemoryFreezeAll(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	// No session yet.
	if msg := e.errMsg(http.MethodPost, "/api/memory/freeze-all", "", http.StatusBadRequest); msg != "no active search session" {
		t.Errorf("error = %q, want %q", msg, "no active search session")
	}

	// A long interval keeps the freeze pollers idle for the duration of the test.
	e.obj(http.MethodPost, "/api/memory/freeze-interval", `{"interval_ms":3600000}`, http.StatusOK)
	e.obj(http.MethodPost, "/api/search/value", `{"value":"1337"}`, http.StatusOK)

	got := e.obj(http.MethodPost, "/api/memory/freeze-all", "", http.StatusOK)
	if c := num(t, got, "count"); c != 3 {
		t.Fatalf("count = %v, want 3", c)
	}
	if list := e.arr(http.MethodGet, "/api/memory/frozen", "", http.StatusOK); len(list) != 3 {
		t.Errorf("frozen = %v, want 3 entries", list)
	}

	// Detaching stops every freeze poller.
	e.obj(http.MethodPost, "/api/process/detach", "", http.StatusOK)
	if list := e.arr(http.MethodGet, "/api/memory/frozen", "", http.StatusOK); len(list) != 0 {
		t.Errorf("frozen after detach = %v, want empty", list)
	}
}
