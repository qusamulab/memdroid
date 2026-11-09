package server

import (
	"net/http"
	"strings"
	"testing"
)

// TestADBEndpointsValidateBeforeShellingOut covers only the branches that run
// before the first exec, so the suite never depends on a real adb binary or a
// connected device.
func TestADBEndpointsValidateBeforeShellingOut(t *testing.T) {
	e := newEnv(t, Config{})

	tests := []struct {
		name    string
		path    string
		body    string
		wantSub string
	}{
		{"device select without serial", "/api/device/select", `{}`, "serial required"},
		{"device select with empty serial", "/api/device/select", `{"serial":""}`, "serial required"},
		{"device select with invalid JSON", "/api/device/select", `{`, "invalid JSON body"},
		{"connect wifi without addr", "/api/device/connect-wifi", `{}`, "addr required"},
		{"disconnect wifi without addr", "/api/device/disconnect-wifi", `{}`, "addr required"},
		{"process search without name", "/api/process/search", `{}`, "name required"},
		{"process search with invalid JSON", "/api/process/search", `nope`, "invalid JSON body"},
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
