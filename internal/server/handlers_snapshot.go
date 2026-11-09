package server

import (
	"net/http"

	"memdroid/internal/memory/modify"
)

const maxSnapshotBytes = 16 << 20

func (h *handler) snapshotTake(w http.ResponseWriter, r *http.Request) {
	pid, ok := h.requirePID(w)
	if !ok {
		return
	}
	addr, size, ok := h.decodeRegionRequest(w, r)
	if !ok {
		return
	}
	data, err := h.state.GetDriver().ReadRegion(pid, addr, size)
	if err != nil {
		serverError(w, "read region", err)
		return
	}
	h.state.SetSnapshot(addr, data)
	writeOK(w, map[string]any{"addr": hexAddr(addr), "size": len(data)})
}

func (h *handler) snapshotDiff(w http.ResponseWriter, r *http.Request) {
	pid, ok := h.requirePID(w)
	if !ok {
		return
	}
	addr, size, ok := h.decodeRegionRequest(w, r)
	if !ok {
		return
	}
	prev := h.state.GetSnapshot(addr)
	if prev == nil {
		writeError(w, http.StatusBadRequest, "no snapshot taken for this address — call /api/snapshot/take first")
		return
	}
	cur, err := h.state.GetDriver().ReadRegion(pid, addr, size)
	if err != nil {
		serverError(w, "read region", err)
		return
	}

	diffs, err := modify.DiffSnapshots(
		&modify.Snapshot{Addr: addr, Data: prev},
		&modify.Snapshot{Addr: addr, Data: cur},
	)
	if err != nil {
		serverError(w, "diff", err)
		return
	}

	type entry struct {
		Addr   hexAddr `json:"addr"`
		Offset int     `json:"offset"`
		Before int     `json:"before"`
		After  int     `json:"after"`
	}
	out := make([]entry, len(diffs))
	for i, d := range diffs {
		out[i] = entry{Addr: hexAddr(d.Addr), Offset: d.Offset, Before: int(d.Before), After: int(d.After)}
	}
	writeJSON(w, map[string]any{"total": len(out), "diffs": out})
}

// decodeRegionRequest reads the {addr, size} body shared by the snapshot
// endpoints.
func (h *handler) decodeRegionRequest(w http.ResponseWriter, r *http.Request) (uintptr, int, bool) {
	req, ok := decodeJSON[struct {
		Addr string `json:"addr"`
		Size int    `json:"size"`
	}](w, r)
	if !ok {
		return 0, 0, false
	}
	addr, ok := requireAddr(w, req.Addr)
	if !ok {
		return 0, 0, false
	}
	if !requireSize(w, req.Size, maxSnapshotBytes) {
		return 0, 0, false
	}
	return addr, req.Size, true
}
