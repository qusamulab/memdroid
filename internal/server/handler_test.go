package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestInvalidJSONBodies(t *testing.T) {
	e := newEnv(t, Config{})
	e.attach()
	// /api/search/filter checks for a session before it decodes, so give it one.
	e.obj(http.MethodPost, "/api/search/value", `{"value":"1337"}`, http.StatusOK)

	paths := []string{
		"/api/search/type",
		"/api/search/filter",
		"/api/memory/modify",
		"/api/memory/unfreeze",
		"/api/bookmark/add",
		"/api/bookmark/remove",
		"/api/watch/remove",
		"/api/snapshot/take",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rec := e.rec(http.MethodPost, path, `{"addr": `)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if !strings.HasPrefix(body["error"], "invalid JSON body") {
				t.Errorf("error = %q, want an invalid-JSON message", body["error"])
			}
		})
	}
}

// TestErrorResponsesAreJSON pins the content type callers rely on.
func TestErrorResponsesAreJSON(t *testing.T) {
	e := newEnv(t, Config{})

	rec := e.rec(http.MethodGet, "/api/maps", "")
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("error Content-Type = %q, want application/json", ct)
	}
	rec = e.rec(http.MethodGet, "/api/status", "")
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("success Content-Type = %q, want application/json", ct)
	}
}
