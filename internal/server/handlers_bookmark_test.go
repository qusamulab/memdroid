package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBookmarkAddListRemove(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	if got := e.arr(http.MethodGet, "/api/bookmark/list", "", http.StatusOK); len(got) != 0 {
		t.Fatalf("initial bookmarks = %v, want empty", got)
	}

	got := e.obj(http.MethodPost, "/api/bookmark/add", `{"addr":"0x1010","label":"hp"}`, http.StatusOK)
	if addr := str(t, got, "addr"); addr != "0x1010" {
		t.Errorf("addr = %q, want %q", addr, "0x1010")
	}
	e.obj(http.MethodPost, "/api/bookmark/add", `{"addr":"0x1020","label":"mp"}`, http.StatusOK)

	list := e.arr(http.MethodGet, "/api/bookmark/list", "", http.StatusOK)
	if len(list) != 2 {
		t.Fatalf("bookmarks = %d, want 2", len(list))
	}
	first := object(t, list[0])
	if idx := num(t, first, "index"); idx != 0 {
		t.Errorf("index = %v, want 0", idx)
	}
	if addr := str(t, first, "addr"); addr != "0x1010" {
		t.Errorf("addr = %q, want %q", addr, "0x1010")
	}
	if label := str(t, first, "label"); label != "hp" {
		t.Errorf("label = %q, want %q", label, "hp")
	}
	if vt := str(t, first, "type"); vt != "int32" {
		t.Errorf("type = %q, want %q", vt, "int32")
	}
	// The live value is read back through the driver.
	if v := str(t, first, "value"); v != "1337" {
		t.Errorf("value = %q, want %q", v, "1337")
	}

	e.obj(http.MethodPost, "/api/bookmark/remove", `{"index":0}`, http.StatusOK)

	list = e.arr(http.MethodGet, "/api/bookmark/list", "", http.StatusOK)
	if len(list) != 1 {
		t.Fatalf("bookmarks after remove = %d, want 1", len(list))
	}
	if addr := str(t, object(t, list[0]), "addr"); addr != "0x1020" {
		t.Errorf("remaining addr = %q, want %q", addr, "0x1020")
	}
}

func TestBookmarkRejectsBadInput(t *testing.T) {
	e := newEnv(t, Config{})

	tests := []struct {
		name    string
		path    string
		body    string
		wantSub string
	}{
		{"missing addr", "/api/bookmark/add", `{"label":"hp"}`, "addr required"},
		{"malformed addr", "/api/bookmark/add", `{"addr":"0xzz"}`, "invalid addr"},
		{"invalid JSON", "/api/bookmark/add", `{"addr":}`, "invalid JSON body"},
		{"remove out of range", "/api/bookmark/remove", `{"index":7}`, "index"},
		{"remove negative index", "/api/bookmark/remove", `{"index":-1}`, "index"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := e.errMsg(http.MethodPost, tc.path, tc.body, http.StatusBadRequest)
			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("error = %q, want it to contain %q", msg, tc.wantSub)
			}
		})
	}
}

func TestBookmarkModifyAll(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()
	e.obj(http.MethodPost, "/api/bookmark/add", `{"addr":"0x1010","label":"a"}`, http.StatusOK)
	e.obj(http.MethodPost, "/api/bookmark/add", `{"addr":"0x1020","label":"b"}`, http.StatusOK)

	got := e.obj(http.MethodPost, "/api/bookmark/modify-all", `{"value":"7"}`, http.StatusOK)
	if c := num(t, got, "count"); c != 2 {
		t.Fatalf("count = %v, want 2", c)
	}
	for _, addr := range []uintptr{0x1010, 0x1020} {
		if v := e.heapInt32(addr); v != 7 {
			t.Errorf("memory at 0x%x = %d, want 7", addr, v)
		}
	}
}

const ctFixture = `<?xml version="1.0" encoding="utf-8"?>
<CheatTable>
  <CheatEntries>
    <CheatEntry>
      <Description>"Health"</Description>
      <VariableType>4 Bytes</VariableType>
      <Address>1010</Address>
    </CheatEntry>
    <CheatEntry>
      <Description>"Mana"</Description>
      <VariableType>Float</VariableType>
      <Address>1020</Address>
    </CheatEntry>
  </CheatEntries>
</CheatTable>`

func TestImportCT(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "table.CT"), []byte(ctFixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	e := newEnv(t, Config{FileRoot: root})

	got := e.obj(http.MethodPost, "/api/import/ct", `{"path":"table.CT"}`, http.StatusOK)
	if n := num(t, got, "imported"); n != 2 {
		t.Fatalf("imported = %v, want 2", n)
	}

	list := e.arr(http.MethodGet, "/api/bookmark/list", "", http.StatusOK)
	if len(list) != 2 {
		t.Fatalf("bookmarks = %d, want 2", len(list))
	}
	first := object(t, list[0])
	if addr := str(t, first, "addr"); addr != "0x1010" {
		t.Errorf("addr = %q, want %q", addr, "0x1010")
	}
	if label := str(t, first, "label"); label != "Health" {
		t.Errorf("label = %q, want %q", label, "Health")
	}
}

func TestImportCTRejectsBadInput(t *testing.T) {
	root := t.TempDir()
	e := newEnv(t, Config{FileRoot: root})

	if msg := e.errMsg(http.MethodPost, "/api/import/ct", `{}`, http.StatusBadRequest); msg != "path required" {
		t.Errorf("error = %q, want %q", msg, "path required")
	}
	if msg := e.errMsg(http.MethodPost, "/api/import/ct", `{"path":"/etc/passwd"}`, http.StatusBadRequest); !strings.Contains(msg, "absolute paths are not allowed") {
		t.Errorf("error = %q, want the path to be rejected", msg)
	}
	if msg := e.errMsg(http.MethodPost, "/api/import/ct", `{"path":"absent.CT"}`, http.StatusInternalServerError); !strings.Contains(msg, "import CT") {
		t.Errorf("error = %q, want it to name the operation", msg)
	}
}
