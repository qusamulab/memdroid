package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestPointerRejectsBadInput(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()

	tests := []struct {
		name    string
		path    string
		body    string
		wantSub string
	}{
		{"scan without addr", "/api/pointer/scan", `{}`, "addr required"},
		{"scan with negative max_offset", "/api/pointer/scan", `{"addr":"0x1010","max_offset":-1}`, "max_offset must not be negative"},
		{"resolve without a label", "/api/pointer/resolve", `{"offsets":[0]}`, "label and offsets required"},
		{"resolve without offsets", "/api/pointer/resolve", `{"label":"[heap]"}`, "label and offsets required"},
		{"resolve with a bad base offset", "/api/pointer/resolve", `{"label":"[heap]","base_offset":"zz","offsets":[0]}`, "invalid addr"},
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
