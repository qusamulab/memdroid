package server

import (
	"net/http"
)

func (h *handler) memoryFreeze(w http.ResponseWriter, r *http.Request) {
	pid, ok := h.requirePID(w)
	if !ok {
		return
	}
	req, ok := decodeJSON[struct {
		Addr  string `json:"addr"`
		Value string `json:"value"`
	}](w, r)
	if !ok {
		return
	}
	addr, ok := requireAddr(w, req.Addr)
	if !ok {
		return
	}
	val, ok := requireValue(w, req.Value, h.state.GetValueType())
	if !ok {
		return
	}
	if err := h.state.Freezer.Freeze(h.state.GetDriver(), pid, addr, val); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w, map[string]any{"addr": hexAddr(addr)})
}

func (h *handler) freezeSetInterval(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[struct {
		IntervalMs int `json:"interval_ms"`
	}](w, r)
	if !ok {
		return
	}
	if req.IntervalMs <= 0 {
		writeError(w, http.StatusBadRequest, "interval_ms must be positive")
		return
	}
	interval, ok := requireDuration(w, req.IntervalMs, 0)
	if !ok {
		return
	}
	h.state.Freezer.SetInterval(interval)
	writeOK(w, map[string]any{"interval_ms": req.IntervalMs})
}

func (h *handler) memoryFreezeAll(w http.ResponseWriter, _ *http.Request) {
	sess, ok := h.requireSession(w)
	if !ok {
		return
	}
	count := h.state.Freezer.FreezeAllCandidates(h.state.GetDriver(), sess)
	writeOK(w, map[string]any{"count": count})
}

func (h *handler) memoryUnfreeze(w http.ResponseWriter, r *http.Request) {
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
	if err := h.state.Freezer.Unfreeze(addr); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w, nil)
}

func (h *handler) memoryFrozen(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, hexAddrs(h.state.Freezer.List()))
}
