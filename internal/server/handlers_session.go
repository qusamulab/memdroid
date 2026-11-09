package server

import (
	"net/http"

	"memdroid/internal/memory/store"
)

func (h *handler) sessionSave(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeOptionalJSON[struct {
		Path string `json:"path"`
	}](w, r)
	if !ok {
		return
	}
	path, ok := h.resolvePath(w, req.Path, store.DefaultStateFile)
	if !ok {
		return
	}
	if err := h.state.Save(path); err != nil {
		serverError(w, "save state", err)
		return
	}
	writeOK(w, map[string]any{"path": path})
}

func (h *handler) sessionLoad(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeOptionalJSON[struct {
		Path string `json:"path"`
	}](w, r)
	if !ok {
		return
	}
	path, ok := h.resolvePath(w, req.Path, store.DefaultStateFile)
	if !ok {
		return
	}
	candidates, err := h.state.Load(path)
	if err != nil {
		serverError(w, "load state", err)
		return
	}
	writeOK(w, map[string]any{"path": path, "candidates": candidates})
}
