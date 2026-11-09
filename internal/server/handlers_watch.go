package server

import (
	"net/http"
	"time"
)

// defaultPollInterval matches the CLI default for watches and alerts.
const defaultPollInterval = 500 * time.Millisecond

func (h *handler) watchAdd(w http.ResponseWriter, r *http.Request) {
	pid, ok := h.requirePID(w)
	if !ok {
		return
	}
	req, ok := decodeJSON[struct {
		Addr       string `json:"addr"`
		IntervalMs int    `json:"interval_ms"`
	}](w, r)
	if !ok {
		return
	}
	addr, ok := requireAddr(w, req.Addr)
	if !ok {
		return
	}
	interval, ok := requireDuration(w, req.IntervalMs, defaultPollInterval)
	if !ok {
		return
	}
	vt := h.state.GetValueType()
	if err := h.state.Watcher.Watch(h.state.GetDriver(), pid, addr, vt, interval); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w, map[string]any{
		"addr":        hexAddr(addr),
		"value_type":  vt.String(),
		"interval_ms": interval.Milliseconds(),
	})
}

func (h *handler) watchRemove(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[struct {
		Addr string `json:"addr"`
	}](w, r)
	if !ok {
		return
	}
	addr, ok := requireAddr(w, req.Addr)
	if !ok {
		return
	}
	if err := h.state.Watcher.Unwatch(addr); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w, nil)
}

func (h *handler) watchList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, hexAddrs(h.state.Watcher.List()))
}
