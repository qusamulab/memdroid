package server

import (
	"net/http"

	"memdroid/internal/memory/modify"
)

const (
	maxHexdumpBytes = 4096
	maxDumpBytes    = 256 << 20
)

func (h *handler) memoryModify(w http.ResponseWriter, r *http.Request) {
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
	if err := h.state.UndoStack.WithUndo(h.state.GetDriver(), pid, addr, val); err != nil {
		serverError(w, "modify", err)
		return
	}
	writeOK(w, map[string]any{"addr": hexAddr(addr), "undo_depth": h.state.UndoStack.Depth()})
}

// memoryWriteString overwrites a string in place with raw UTF-8 bytes.
func (h *handler) memoryWriteString(w http.ResponseWriter, r *http.Request) {
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
	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "value required")
		return
	}
	if err := modify.WriteString(h.state.GetDriver(), pid, addr, req.Value); err != nil {
		serverError(w, "write string", err)
		return
	}
	writeOK(w, map[string]any{"addr": hexAddr(addr), "bytes": len(req.Value)})
}

func (h *handler) memoryUndo(w http.ResponseWriter, _ *http.Request) {
	if err := h.state.UndoStack.Undo(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w, map[string]any{"undo_depth": h.state.UndoStack.Depth()})
}

func (h *handler) memoryHexdump(w http.ResponseWriter, r *http.Request) {
	pid, ok := h.requirePID(w)
	if !ok {
		return
	}
	addr, ok := requireAddr(w, r.URL.Query().Get("addr"))
	if !ok {
		return
	}
	size := queryInt(r, "size", 0, 1)
	if !requireSize(w, size, maxHexdumpBytes) {
		return
	}
	data, err := h.state.GetDriver().ReadRegion(pid, addr, size)
	if err != nil {
		serverError(w, "read region", err)
		return
	}

	type line struct {
		Offset int     `json:"offset"`
		Addr   hexAddr `json:"addr"`
		Hex    string  `json:"hex"`
		ASCII  string  `json:"ascii"`
	}
	src := modify.HexLines(addr, data)
	lines := make([]line, len(src))
	for i, l := range src {
		lines[i] = line{Offset: l.Offset, Addr: hexAddr(l.Addr), Hex: l.Hex, ASCII: l.ASCII}
	}
	writeJSON(w, map[string]any{"addr": hexAddr(addr), "size": len(data), "hex_lines": lines})
}

// memoryDump writes a region to a file on the host running memdroid.
func (h *handler) memoryDump(w http.ResponseWriter, r *http.Request) {
	pid, ok := h.requirePID(w)
	if !ok {
		return
	}
	req, ok := decodeJSON[struct {
		Addr string `json:"addr"`
		Size int    `json:"size"`
		Path string `json:"path"`
	}](w, r)
	if !ok {
		return
	}
	addr, ok := requireAddr(w, req.Addr)
	if !ok {
		return
	}
	if !requireSize(w, req.Size, maxDumpBytes) {
		return
	}
	path, ok := h.resolvePath(w, req.Path, "dump.hex")
	if !ok {
		return
	}
	if err := modify.DumpRegion(h.state.GetDriver(), pid, addr, req.Size, path); err != nil {
		serverError(w, "dump", err)
		return
	}
	writeOK(w, map[string]any{"path": path, "size": req.Size})
}
