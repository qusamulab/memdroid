package server

import (
	"testing"
)

func assertCandidate(t *testing.T, item any, wantAddr, wantValue string) {
	t.Helper()
	m := object(t, item)
	if addr := str(t, m, "addr"); addr != wantAddr {
		t.Errorf("candidate addr = %q, want %q", addr, wantAddr)
	}
	if v := str(t, m, "value"); v != wantValue {
		t.Errorf("candidate %s value = %q, want %q", wantAddr, v, wantValue)
	}
}
