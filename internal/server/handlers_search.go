package server

import (
	"context"
	"errors"
	"net/http"

	"memdroid/internal/driver"
	"memdroid/internal/memory/search"
)

const defaultPageSize = 100

// applyValueType switches the active type when the request names one. The type
// lives on the session, so this keeps a scan and the formatting of its results
// in agreement.
func (h *handler) applyValueType(w http.ResponseWriter, name string) (search.ValueType, bool) {
	if name == "" {
		return h.state.GetValueType(), true
	}
	vt, err := search.ParseValueType(name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return 0, false
	}
	h.state.SetValueType(vt)
	return vt, true
}

// parseRegionFilter maps the optional region selector in a search request.
func parseRegionFilter(w http.ResponseWriter, name, startStr, endStr string) (driver.RegionFilter, uintptr, uintptr, bool) {
	var start, end uintptr
	var filter driver.RegionFilter
	switch name {
	case "", "all":
		filter = driver.RegionAll
	case "heap":
		filter = driver.RegionHeap
	case "stack":
		filter = driver.RegionStack
	case "anon":
		filter = driver.RegionAnon
	case "custom":
		filter = driver.RegionCustom
		var ok bool
		if start, ok = requireAddr(w, startStr); !ok {
			return 0, 0, 0, false
		}
		if end, ok = requireAddr(w, endStr); !ok {
			return 0, 0, 0, false
		}
		if end <= start {
			writeError(w, http.StatusBadRequest, "region end must be greater than start")
			return 0, 0, 0, false
		}
	default:
		writeError(w, http.StatusBadRequest, "unknown region "+name+" (use: all, heap, stack, anon, custom)")
		return 0, 0, 0, false
	}
	return filter, start, end, true
}

func (h *handler) searchValue(w http.ResponseWriter, r *http.Request) {
	target, ok := h.requireTarget(w)
	if !ok {
		return
	}
	req, ok := decodeJSON[struct {
		Value       string `json:"value"`
		Type        string `json:"type"`
		Region      string `json:"region"`
		RegionStart string `json:"region_start"`
		RegionEnd   string `json:"region_end"`
	}](w, r)
	if !ok {
		return
	}

	vt := target.ValueType
	if req.Type != "" {
		var err error
		vt, err = search.ParseValueType(req.Type)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	val, ok := requireValue(w, req.Value, vt)
	if !ok {
		return
	}
	filter, start, end, ok := parseRegionFilter(w, req.Region, req.RegionStart, req.RegionEnd)
	if !ok {
		return
	}

	sess := target.Session
	if err := sess.SearchTyped(val, vt, filter, start, end); err != nil {
		searchError(w, "search", err)
		return
	}
	writeJSON(w, map[string]any{"candidates": sess.CandidateCount(), "truncated": sess.Truncated()})
}

func (h *handler) searchPattern(w http.ResponseWriter, r *http.Request) {
	target, ok := h.requireTarget(w)
	if !ok {
		return
	}
	req, ok := decodeJSON[struct {
		Pattern string `json:"pattern"`
	}](w, r)
	if !ok {
		return
	}
	pat, err := search.ParsePattern(req.Pattern)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(pat) > search.MaxPatternBytes {
		writeError(w, http.StatusBadRequest, "pattern exceeds candidate byte budget")
		return
	}
	sess := target.Session
	result, err := sess.SearchPattern(pat)
	if err != nil {
		searchError(w, "pattern search", err)
		return
	}
	h.writeScanResult(w, sess, result)
}

func (h *handler) searchString(w http.ResponseWriter, r *http.Request) {
	target, ok := h.requireTarget(w)
	if !ok {
		return
	}
	req, ok := decodeJSON[struct {
		Value    string `json:"value"`
		Encoding string `json:"encoding"`
	}](w, r)
	if !ok {
		return
	}
	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "empty string")
		return
	}
	enc, err := search.ParseStringEncoding(req.Encoding)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(search.StringBytes(req.Value, enc)) > search.MaxPatternBytes {
		writeError(w, http.StatusBadRequest, "string exceeds candidate byte budget")
		return
	}
	sess := target.Session
	result, err := sess.SearchString(req.Value, enc)
	if err != nil {
		searchError(w, "string search", err)
		return
	}
	h.writeScanResult(w, sess, result)
}

// writeScanResult reports a result already committed to the captured session.
func (h *handler) writeScanResult(w http.ResponseWriter, sess *search.Session, result search.PatternResult) {
	writeJSON(w, map[string]any{
		"count":      len(result.Matches),
		"candidates": sess.CandidateCount(),
		"truncated":  result.Truncated,
	})
}

func (h *handler) searchFilter(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.requireSession(w)
	if !ok {
		return
	}
	req, ok := decodeJSON[struct {
		Mode  string `json:"mode"`
		Value string `json:"value"`
	}](w, r)
	if !ok {
		return
	}
	mode, err := search.ParseFilterMode(req.Mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var target []byte
	if mode == search.FilterValue {
		if target, ok = requireValue(w, req.Value, sess.Type()); !ok {
			return
		}
	}
	if err := sess.Filter(mode, target); err != nil {
		if errors.Is(err, search.ErrSearchConflict) || errors.Is(err, search.ErrSearchBusy) {
			searchError(w, "filter", err)
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"candidates": sess.CandidateCount(), "truncated": sess.Truncated()})
}

func (h *handler) searchCandidates(w http.ResponseWriter, r *http.Request) {
	pageSize := min(queryInt(r, "page_size", defaultPageSize, 1), 1000)
	page := queryInt(r, "page", 0, 0)

	sess := h.state.GetSession()
	if sess == nil {
		writeJSON(w, map[string]any{"total": 0, "page": page, "page_size": pageSize, "items": []any{}})
		return
	}

	// Page copies only the requested slice, so this stays cheap even when the
	// candidate set has millions of entries.
	candidates, total, vt := sess.TypedPage(page*pageSize, pageSize)

	type entry struct {
		Addr  hexAddr `json:"addr"`
		Value string  `json:"value"`
	}
	items := make([]entry, len(candidates))
	for i, c := range candidates {
		items[i] = entry{Addr: hexAddr(c.Addr), Value: search.FormatValue(c.Value, vt)}
	}

	writeJSON(w, map[string]any{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"items":     items,
	})
}

func (h *handler) searchReset(w http.ResponseWriter, _ *http.Request) {
	if sess := h.state.GetSession(); sess != nil {
		sess.Reset()
	}
	writeOK(w, nil)
}

func (h *handler) searchTypes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"types":   search.ValueTypeNames(),
		"current": h.state.GetValueType().String(),
	})
}

func (h *handler) searchSetType(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[struct {
		Type string `json:"type"`
	}](w, r)
	if !ok {
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type required")
		return
	}
	vt, ok := h.applyValueType(w, req.Type)
	if !ok {
		return
	}
	writeOK(w, map[string]any{"value_type": vt.String()})
}

func searchError(w http.ResponseWriter, op string, err error) {
	if errors.Is(err, search.ErrSearchConflict) || errors.Is(err, search.ErrSearchBusy) || errors.Is(err, context.Canceled) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	serverError(w, op, err)
}
