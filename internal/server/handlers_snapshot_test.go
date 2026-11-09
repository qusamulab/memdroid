package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestSnapshotTakeAndDiff(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	// A diff with no prior take is a client error.
	msg := e.errMsg(http.MethodPost, "/api/snapshot/diff", `{"addr":"0x1200","size":16}`, http.StatusBadRequest)
	if !strings.Contains(msg, "no snapshot taken") {
		t.Errorf("error = %q, want it to mention the missing snapshot", msg)
	}

	got := e.obj(http.MethodPost, "/api/snapshot/take", `{"addr":"0x1000","size":16}`, http.StatusOK)
	if !boolean(t, got, "ok") {
		t.Error("ok = false")
	}
	if size := num(t, got, "size"); size != 16 {
		t.Errorf("size = %v, want 16", size)
	}

	// An unchanged region diffs to nothing.
	got = e.obj(http.MethodPost, "/api/snapshot/diff", `{"addr":"0x1000","size":16}`, http.StatusOK)
	if total := num(t, got, "total"); total != 0 {
		t.Fatalf("total = %v, want 0 before any change", total)
	}

	// 9999 == 0x270f, so exactly the low two bytes of the region change.
	e.obj(http.MethodPost, "/api/memory/modify", `{"addr":"0x1000","value":"9999"}`, http.StatusOK)

	got = e.obj(http.MethodPost, "/api/snapshot/diff", `{"addr":"0x1000","size":16}`, http.StatusOK)
	if total := num(t, got, "total"); total != 2 {
		t.Fatalf("total = %v, want 2 (body: %v)", total, got)
	}
	diffs := slice(t, got, "diffs")
	if len(diffs) != 2 {
		t.Fatalf("diffs = %d, want 2", len(diffs))
	}
	wantDiffs := []struct {
		addr   string
		offset float64
		before float64
		after  float64
	}{
		{"0x1000", 0, 0, 0x0f},
		{"0x1001", 1, 0, 0x27},
	}
	for i, want := range wantDiffs {
		d := object(t, diffs[i])
		if addr := str(t, d, "addr"); addr != want.addr {
			t.Errorf("diff %d addr = %q, want %q", i, addr, want.addr)
		}
		if off := num(t, d, "offset"); off != want.offset {
			t.Errorf("diff %d offset = %v, want %v", i, off, want.offset)
		}
		if b := num(t, d, "before"); b != want.before {
			t.Errorf("diff %d before = %v, want %v", i, b, want.before)
		}
		if a := num(t, d, "after"); a != want.after {
			t.Errorf("diff %d after = %v, want %v", i, a, want.after)
		}
	}
}

func TestSnapshotRejectsBadRegion(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	tests := []struct {
		name    string
		body    string
		wantSub string
	}{
		{"missing addr", `{"size":16}`, "addr required"},
		{"malformed addr", `{"addr":"nope","size":16}`, "invalid addr"},
		{"size zero", `{"addr":"0x1000","size":0}`, "size must be 1-"},
		{"size above the cap", `{"addr":"0x1000","size":16777217}`, "size must be 1-"},
		{"invalid JSON", `{"addr":`, "invalid JSON body"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := e.errMsg(http.MethodPost, "/api/snapshot/take", tc.body, http.StatusBadRequest)
			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("error = %q, want it to contain %q", msg, tc.wantSub)
			}
		})
	}
}
