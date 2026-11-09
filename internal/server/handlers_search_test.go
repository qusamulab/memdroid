package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestSearchFilterModifyUndoFlow(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	// 1. Initial scan finds the three planted int32 values.
	got := e.obj(http.MethodPost, "/api/search/value", `{"value":"1337"}`, http.StatusOK)
	if c := num(t, got, "candidates"); c != 3 {
		t.Fatalf("candidates = %v, want 3", c)
	}

	// 2. Paging returns addresses in ascending order across pages.
	page0 := e.obj(http.MethodGet, "/api/search/candidates?page_size=2&page=0", "", http.StatusOK)
	if total := num(t, page0, "total"); total != 3 {
		t.Errorf("page 0 total = %v, want 3", total)
	}
	if ps := num(t, page0, "page_size"); ps != 2 {
		t.Errorf("page 0 page_size = %v, want 2", ps)
	}
	items0 := slice(t, page0, "items")
	if len(items0) != 2 {
		t.Fatalf("page 0 items = %d, want 2", len(items0))
	}
	assertCandidate(t, items0[0], "0x1010", "1337")
	assertCandidate(t, items0[1], "0x1020", "1337")

	page1 := e.obj(http.MethodGet, "/api/search/candidates?page_size=2&page=1", "", http.StatusOK)
	items1 := slice(t, page1, "items")
	if len(items1) != 1 {
		t.Fatalf("page 1 items = %d, want 1", len(items1))
	}
	assertCandidate(t, items1[0], "0x1030", "1337")

	// A page past the end is empty but still reports the total.
	page9 := e.obj(http.MethodGet, "/api/search/candidates?page_size=2&page=9", "", http.StatusOK)
	if total := num(t, page9, "total"); total != 3 {
		t.Errorf("page 9 total = %v, want 3", total)
	}
	if items := page9["items"]; items != nil {
		if l, ok := items.([]any); ok && len(l) != 0 {
			t.Errorf("page 9 items = %v, want empty", l)
		}
	}

	// 3. Nothing has moved, so an "unchanged" filter keeps all three.
	got = e.obj(http.MethodPost, "/api/search/filter", `{"mode":"unchanged"}`, http.StatusOK)
	if c := num(t, got, "candidates"); c != 3 {
		t.Fatalf("candidates after unchanged filter = %v, want 3", c)
	}

	// 4. Modify one of them and confirm the fake process actually changed.
	got = e.obj(http.MethodPost, "/api/memory/modify", `{"addr":"0x1010","value":"4242"}`, http.StatusOK)
	if !boolean(t, got, "ok") {
		t.Error("ok = false")
	}
	if addr := str(t, got, "addr"); addr != "0x1010" {
		t.Errorf("addr = %q, want %q", addr, "0x1010")
	}
	if d := num(t, got, "undo_depth"); d != 1 {
		t.Errorf("undo_depth = %v, want 1", d)
	}
	if v := e.heapInt32(0x1010); v != 4242 {
		t.Fatalf("memory at 0x1010 = %d, want 4242", v)
	}
	if v := e.heapInt32(0x1020); v != 1337 {
		t.Errorf("memory at 0x1020 = %d, want it untouched at 1337", v)
	}

	// 5. A "changed" filter now narrows to exactly the modified address.
	got = e.obj(http.MethodPost, "/api/search/filter", `{"mode":"changed"}`, http.StatusOK)
	if c := num(t, got, "candidates"); c != 1 {
		t.Fatalf("candidates after changed filter = %v, want 1", c)
	}
	items := slice(t, e.obj(http.MethodGet, "/api/search/candidates", "", http.StatusOK), "items")
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	assertCandidate(t, items[0], "0x1010", "4242")

	// 6. Undo restores the original bytes and empties the stack.
	got = e.obj(http.MethodPost, "/api/memory/undo", "", http.StatusOK)
	if d := num(t, got, "undo_depth"); d != 0 {
		t.Errorf("undo_depth = %v, want 0", d)
	}
	if v := e.heapInt32(0x1010); v != 1337 {
		t.Fatalf("memory at 0x1010 after undo = %d, want 1337", v)
	}

	// Undoing an empty stack is a client error, not a crash.
	if msg := e.errMsg(http.MethodPost, "/api/memory/undo", "", http.StatusBadRequest); msg == "" {
		t.Error("undo on an empty stack returned no error message")
	}
}

// TestSearchValueRetypesAndRescans is a regression test.
//
// Naming a type in a /api/search/value request must switch the session's type
// *before* the scan runs. A previous version kept the session's original type,
// so a float32 request after an int32 search silently scanned and formatted at
// the stale type.
func TestSearchValueRetypesAndRescans(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	// Establish a session at the default int32 type.
	got := e.obj(http.MethodPost, "/api/search/value", `{"value":"1337"}`, http.StatusOK)
	if c := num(t, got, "candidates"); c != 3 {
		t.Fatalf("int32 candidates = %v, want 3", c)
	}
	status := e.obj(http.MethodGet, "/api/status", "", http.StatusOK)
	if vt := str(t, status, "value_type"); vt != "int32" {
		t.Fatalf("value_type = %q, want %q", vt, "int32")
	}

	// Now search at a different type in the same request.
	got = e.obj(http.MethodPost, "/api/search/value", `{"value":"2.5","type":"float32"}`, http.StatusOK)
	if c := num(t, got, "candidates"); c != 2 {
		t.Fatalf("float32 candidates = %v, want 2 (a stale int32 session would not re-scan)", c)
	}

	// The switch is visible in the status, and the candidate count proves the
	// scan re-ran rather than reusing the int32 result set.
	status = e.obj(http.MethodGet, "/api/status", "", http.StatusOK)
	if vt := str(t, status, "value_type"); vt != "float32" {
		t.Errorf("value_type = %q, want %q", vt, "float32")
	}
	if c := num(t, status, "candidates"); c != 2 {
		t.Errorf("status candidates = %v, want 2", c)
	}

	// Candidates are the float addresses, formatted with the new type.
	items := slice(t, e.obj(http.MethodGet, "/api/search/candidates", "", http.StatusOK), "items")
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	assertCandidate(t, items[0], "0x1040", "2.5")
	assertCandidate(t, items[1], "0x1050", "2.5")

	// And the int32 addresses are gone.
	for _, it := range items {
		switch addr := str(t, object(t, it), "addr"); addr {
		case "0x1010", "0x1020", "0x1030":
			t.Errorf("stale int32 candidate %s survived the type switch", addr)
		}
	}
}

func TestSearchValueRejectsBadInput(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	tests := []struct {
		name    string
		body    string
		wantSub string
	}{
		{"unknown type", `{"value":"1","type":"int128"}`, `unknown value type "int128"`},
		{"value not parseable as the type", `{"value":"2.5"}`, "invalid value"},
		{"unknown region", `{"value":"1","region":"nowhere"}`, "unknown region"},
		{"custom region without bounds", `{"value":"1","region":"custom"}`, "addr required"},
		{"custom region end below start", `{"value":"1","region":"custom","region_start":"0x2000","region_end":"0x1000"}`, "region end must be greater than start"},
		{"invalid JSON", `{`, "invalid JSON body"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := e.errMsg(http.MethodPost, "/api/search/value", tc.body, http.StatusBadRequest)
			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("error = %q, want it to contain %q", msg, tc.wantSub)
			}
		})
	}
}

func TestSearchCandidatesWithoutSession(t *testing.T) {
	e := newEnv(t, Config{})

	got := e.obj(http.MethodGet, "/api/search/candidates", "", http.StatusOK)
	if total := num(t, got, "total"); total != 0 {
		t.Errorf("total = %v, want 0", total)
	}
	if ps := num(t, got, "page_size"); ps != defaultPageSize {
		t.Errorf("page_size = %v, want %d", ps, defaultPageSize)
	}
	if items := slice(t, got, "items"); len(items) != 0 {
		t.Errorf("items = %v, want empty", items)
	}
}

func TestSearchFilterWithoutSession(t *testing.T) {
	e := newEnv(t, Config{})
	if msg := e.errMsg(http.MethodPost, "/api/search/filter", `{"mode":"changed"}`, http.StatusBadRequest); msg != "no active search session" {
		t.Errorf("error = %q, want %q", msg, "no active search session")
	}
}

func TestSearchFilterRejectsUnknownMode(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()
	e.obj(http.MethodPost, "/api/search/value", `{"value":"1337"}`, http.StatusOK)

	msg := e.errMsg(http.MethodPost, "/api/search/filter", `{"mode":"sideways"}`, http.StatusBadRequest)
	if !strings.Contains(msg, "unknown filter mode") {
		t.Errorf("error = %q, want it to mention the filter mode", msg)
	}
}

func TestSearchReset(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()
	e.obj(http.MethodPost, "/api/search/value", `{"value":"1337"}`, http.StatusOK)

	e.obj(http.MethodPost, "/api/search/reset", "", http.StatusOK)

	if c := num(t, e.obj(http.MethodGet, "/api/status", "", http.StatusOK), "candidates"); c != 0 {
		t.Errorf("candidates after reset = %v, want 0", c)
	}
	// Resetting again with no session is still a no-op success.
	e.obj(http.MethodPost, "/api/search/reset", "", http.StatusOK)
}

func TestSearchTypes(t *testing.T) {
	e := newEnv(t, Config{})

	got := e.obj(http.MethodGet, "/api/search/types", "", http.StatusOK)
	if cur := str(t, got, "current"); cur != "int32" {
		t.Errorf("current = %q, want %q", cur, "int32")
	}
	types := slice(t, got, "types")
	want := []string{"int32", "int64", "float32", "float64", "uint32", "uint64", "bytes"}
	if len(types) != len(want) {
		t.Fatalf("types = %v, want %d entries", types, len(want))
	}
	for i, w := range want {
		if types[i] != w {
			t.Errorf("types[%d] = %v, want %q", i, types[i], w)
		}
	}
}

func TestSearchSetType(t *testing.T) {
	e := newEnv(t, Config{})

	got := e.obj(http.MethodPost, "/api/search/type", `{"type":"uint64"}`, http.StatusOK)
	if !boolean(t, got, "ok") {
		t.Error("ok = false")
	}
	if vt := str(t, got, "value_type"); vt != "uint64" {
		t.Errorf("value_type = %q, want %q", vt, "uint64")
	}
	if cur := str(t, e.obj(http.MethodGet, "/api/search/types", "", http.StatusOK), "current"); cur != "uint64" {
		t.Errorf("current = %q, want %q", cur, "uint64")
	}
	if vt := str(t, e.obj(http.MethodGet, "/api/status", "", http.StatusOK), "value_type"); vt != "uint64" {
		t.Errorf("status value_type = %q, want %q", vt, "uint64")
	}
}

func TestSearchSetTypeRejectsBadInput(t *testing.T) {
	e := newEnv(t, Config{})

	tests := []struct {
		name    string
		body    string
		wantSub string
	}{
		{"missing type", `{}`, "type required"},
		{"empty type", `{"type":""}`, "type required"},
		{"unknown type", `{"type":"decimal"}`, `unknown value type "decimal"`},
		{"invalid JSON", `not json`, "invalid JSON body"},
		{"empty body", "", "invalid JSON body"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := e.errMsg(http.MethodPost, "/api/search/type", tc.body, http.StatusBadRequest)
			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("error = %q, want it to contain %q", msg, tc.wantSub)
			}
		})
	}
}

// TestSetTypeDiscardsCandidates covers the other half of the type/session
// contract: candidates recorded at one width must not survive a type change.
func TestSetTypeDiscardsCandidates(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()
	e.obj(http.MethodPost, "/api/search/value", `{"value":"1337"}`, http.StatusOK)

	e.obj(http.MethodPost, "/api/search/type", `{"type":"int64"}`, http.StatusOK)

	status := e.obj(http.MethodGet, "/api/status", "", http.StatusOK)
	if vt := str(t, status, "value_type"); vt != "int64" {
		t.Errorf("value_type = %q, want %q", vt, "int64")
	}
	if c := num(t, status, "candidates"); c != 0 {
		t.Errorf("candidates = %v, want 0 after a type change", c)
	}
}

func TestSearchString(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	got := e.obj(http.MethodPost, "/api/search/string", `{"value":"MEMDROID"}`, http.StatusOK)
	if c := num(t, got, "count"); c != 1 {
		t.Fatalf("count = %v, want 1", c)
	}
	if c := num(t, got, "candidates"); c != 1 {
		t.Errorf("candidates = %v, want 1", c)
	}
	if truncated, ok := got["truncated"].(bool); !ok || truncated {
		t.Errorf("truncated = %v, want false", got["truncated"])
	}

	// A string scan seeds the session with raw byte candidates, so both the
	// type and the value formatting switch to "bytes".
	if vt := str(t, e.obj(http.MethodGet, "/api/status", "", http.StatusOK), "value_type"); vt != "bytes" {
		t.Errorf("value_type = %q, want %q", vt, "bytes")
	}
	items := slice(t, e.obj(http.MethodGet, "/api/search/candidates", "", http.StatusOK), "items")
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	assertCandidate(t, items[0], "0x1080", "4d454d44524f4944")

	// UTF-16 finds nothing in this heap, which is a success with zero matches.
	got = e.obj(http.MethodPost, "/api/search/string", `{"value":"MEMDROID","encoding":"utf16"}`, http.StatusOK)
	if c := num(t, got, "count"); c != 0 {
		t.Errorf("utf16 count = %v, want 0", c)
	}
}

func TestSearchStringRejectsBadInput(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	if msg := e.errMsg(http.MethodPost, "/api/search/string", `{"value":""}`, http.StatusBadRequest); !strings.Contains(msg, "empty string") {
		t.Errorf("error = %q, want it to mention the empty string", msg)
	}
	msg := e.errMsg(http.MethodPost, "/api/search/string", `{"value":"x","encoding":"rot13"}`, http.StatusBadRequest)
	if !strings.Contains(msg, "unknown string encoding") {
		t.Errorf("error = %q, want it to mention the encoding", msg)
	}
}

func TestSearchPattern(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	// "MEMD" with the third byte wildcarded.
	got := e.obj(http.MethodPost, "/api/search/pattern", `{"pattern":"4d 45 ?? 44 52"}`, http.StatusOK)
	if c := num(t, got, "count"); c != 1 {
		t.Fatalf("count = %v, want 1 (body: %v)", c, got)
	}
	items := slice(t, e.obj(http.MethodGet, "/api/search/candidates", "", http.StatusOK), "items")
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	assertCandidate(t, items[0], "0x1080", "4d454d4452")
}

func TestSearchPatternRejectsBadInput(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	if msg := e.errMsg(http.MethodPost, "/api/search/pattern", `{"pattern":""}`, http.StatusBadRequest); !strings.Contains(msg, "empty pattern") {
		t.Errorf("error = %q, want it to mention the empty pattern", msg)
	}
	if msg := e.errMsg(http.MethodPost, "/api/search/pattern", `{"pattern":"zz"}`, http.StatusBadRequest); !strings.Contains(msg, "invalid token") {
		t.Errorf("error = %q, want it to name the bad token", msg)
	}
}
