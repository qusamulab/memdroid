package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"golang.org/x/net/websocket"

	"memdroid/internal/app"
	"memdroid/internal/driver/adb"
	"memdroid/internal/memory/watch"
	"memdroid/internal/server/wswatch"
	"memdroid/internal/webui"
)

const (
	readHeaderTimeout = 15 * time.Second
	writeTimeout      = 120 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownTimeout   = 5 * time.Second
)

// Config controls how the HTTP server is exposed.
type Config struct {
	// Addr is the listen address, e.g. "127.0.0.1:8080".
	Addr string
	// Token, when non-empty, is required on all /api and /ws requests (via the
	// "token" query parameter, an "mdtoken" cookie, or an
	// "Authorization: Bearer" header).
	Token string
	// FileRoot confines the paths the file-touching endpoints may use. Empty
	// disables the restriction.
	FileRoot string
}

// Server is a configured but not-yet-listening HTTP server.
type Server struct {
	http *http.Server
	hub  *wswatch.Hub
	// unsubscribe detaches the event listeners registered at build time.
	unsubscribe []func()
}

// New wires up the API, the embedded Web UI, and the WebSocket event feed.
func New(cfg Config, state *app.State, d *adb.ADB) (*Server, error) {
	hub := wswatch.NewHub()
	h := &handler{state: state, adb: d, fileRoot: cfg.FileRoot}

	srv := &Server{hub: hub}

	// Push watch and alert activity to connected browsers. These are
	// registrations, not assignments, so the CLI's own printers keep working.
	srv.unsubscribe = append(srv.unsubscribe,
		state.Watcher.OnChange(func(ev watch.ChangeEvent) {
			hub.Broadcast(wswatch.Event{
				Kind: "change",
				Addr: hexAddr(ev.Addr).String(),
				Prev: ev.Prev,
				Cur:  ev.Cur,
			})
		}),
		state.AlertWatcher.OnAlert(func(ev watch.AlertEvent) {
			hub.Broadcast(wswatch.Event{
				Kind:      "alert",
				Addr:      hexAddr(ev.Addr).String(),
				Cur:       ev.Value,
				Condition: ev.Condition,
				Triggered: ev.Triggered,
			})
		}),
	)

	mux := http.NewServeMux()
	mux.Handle("/", webui.Handler())
	mux.Handle("/ws/watch", originChecked(cfg.Addr, websocket.Handler(hub.Register)))
	for _, rt := range h.routes() {
		mux.HandleFunc(rt.path, only(rt.method, rt.handler))
	}

	srv.http = &http.Server{
		Addr:              cfg.Addr,
		Handler:           secure(cfg.Token, mux),
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	return srv, nil
}

// ListenAndServe runs the server until it is shut down. It returns nil on a
// clean shutdown.
func (s *Server) ListenAndServe() error {
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown stops accepting requests, drops WebSocket clients, and unregisters
// the event listeners.
func (s *Server) Shutdown() error {
	for _, remove := range s.unsubscribe {
		remove()
	}
	s.hub.CloseAll()

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return s.http.Shutdown(ctx)
}
