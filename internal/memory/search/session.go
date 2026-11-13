package search

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"memdroid/internal/driver"
)

// Session holds the current search state for a single attached process.
//
// Every field is guarded by mu; nothing is exported directly. The session owns
// the active ValueType, so callers cannot end up searching with one width while
// formatting results with another. Long-running scans (Search/Filter) do their
// ADB I/O without holding the lock and only take it to swap in the finished
// result, so concurrent CLI/HTTP access never observes a half-built candidate
// map.
type Session struct {
	revision   uint64
	invalid    bool
	busy       bool
	cancel     context.CancelFunc
	identity   string
	truncated  bool
	mu         sync.Mutex
	pid        int
	valueType  ValueType
	drv        driver.Driver
	candidates map[uintptr][]byte
	active     bool
}

func NewSession(pid int, vt ValueType, drv driver.Driver) *Session {
	return &Session{pid: pid, valueType: vt, drv: drv}
}

// PID returns the process this session searches.
func (s *Session) PID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pid
}

// Type returns the value type all candidates in this session are stored as.
func (s *Session) Type() ValueType {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.valueType
}

// SetType changes the active value type. Candidates recorded under a different
// byte width are meaningless afterwards, so they are discarded — a type change
// always starts a fresh search.
func (s *Session) SetType(vt ValueType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.valueType == vt {
		return
	}
	s.invalidateLocked()
	s.valueType = vt
	s.candidates = nil
	s.active = false
}

// SetDriver rebinds the session to a driver. Used after loading a session from
// disk, which cannot serialize a live connection.
func (s *Session) SetDriver(drv driver.Driver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidateLocked()
	s.drv = drv
}

// HasCandidates reports whether a scan has run and left at least one match.
func (s *Session) HasCandidates() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active && len(s.candidates) > 0
}

// Searched reports whether a scan has run at all, regardless of how many
// matches it found. A search that returned nothing is a different situation
// from never having searched.
func (s *Session) Searched() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *Session) CandidateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.candidates)
}

func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidateLocked()
	s.candidates = nil
	s.active = false
	s.truncated = false
}

// SetCandidates replaces the candidate map with c and marks the session active.
// Used after loading from disk or a pattern/string scan.
func (s *Session) SetCandidates(c map[uintptr][]byte) {
	s.replace(c)
}

// SetCandidatesAs replaces both the value type and the candidate map in one
// atomic step. Pattern and string scans need this: they produce TypeBytes
// candidates, and setting the type first would discard them.
func (s *Session) SetCandidatesAs(vt ValueType, c map[uintptr][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidateLocked()
	s.valueType = vt
	s.candidates = c
	s.truncated = false
	s.active = true
}

// replace swaps in a freshly built candidate map and marks the session active.
func (s *Session) replace(found map[uintptr][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidateLocked()
	s.candidates = found
	s.truncated = false
	s.active = true
}

// scanContext captures everything a scan needs from the session in one locked
// read, so the scan itself runs lock-free.
func (s *Session) scanContext() (pid int, vt ValueType, drv driver.Driver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pid, s.valueType, s.drv
}

// candidateWidth returns the byte width of the stored candidates: the type's
// fixed size, or for TypeBytes the length of an arbitrary candidate (all
// candidates from a single scan share a length). Returns 1 when empty.
func (s *Session) candidateWidth() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if size := s.valueType.Size(); size != 0 {
		return size
	}
	for _, v := range s.candidates {
		return len(v)
	}
	return 1
}

// Snapshot returns a deep copy of the current address -> value map.
func (s *Session) Snapshot() map[uintptr][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[uintptr][]byte, len(s.candidates))
	for addr, val := range s.candidates {
		out[addr] = bytes.Clone(val)
	}
	return out
}

// Candidate is one address/value pair from a search result.
type Candidate struct {
	Addr  uintptr
	Value []byte
}

// Page returns candidates sorted by address, starting at offset and at most
// limit entries, together with the total candidate count. Unlike Snapshot it
// copies only the requested page, so paging a multi-million-entry result set
// stays cheap.
func (s *Session) Page(offset, limit int) (page []Candidate, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.pageLocked(offset, limit)
}

func (s *Session) pageLocked(offset, limit int) (page []Candidate, total int) {
	total = len(s.candidates)
	if offset < 0 {
		offset = 0
	}
	if offset >= total || limit <= 0 {
		return nil, total
	}

	addrs := make([]uintptr, 0, total)
	for addr := range s.candidates {
		addrs = append(addrs, addr)
	}
	slices.Sort(addrs)

	end := offset + min(limit, total-offset)
	page = make([]Candidate, 0, end-offset)
	for _, addr := range addrs[offset:end] {
		page = append(page, Candidate{Addr: addr, Value: bytes.Clone(s.candidates[addr])})
	}
	return page, total
}

var ErrSearchConflict = errors.New("search changed while operation was running; retry")
var ErrSearchBusy = errors.New("another search operation is running")

func (s *Session) invalidateLocked() {
	s.revision++
	if s.cancel != nil {
		s.cancel()
	}
}

// Invalidate permanently revokes a session when its attachment is replaced.
func (s *Session) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalid = true
	s.invalidateLocked()
}
func (s *Session) SetIdentity(identity string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.identity = identity
}
func (s *Session) Identity() string { s.mu.Lock(); defer s.mu.Unlock(); return s.identity }
func (s *Session) Truncated() bool  { s.mu.Lock(); defer s.mu.Unlock(); return s.truncated }

type scanOperation struct {
	s          *Session
	revision   uint64
	pid        int
	vt         ValueType
	drv        driver.Driver
	ctx        context.Context
	cancel     context.CancelFunc
	candidates map[uintptr][]byte
	truncated  bool
}

func (s *Session) begin(filter bool) (*scanOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.invalid {
		return nil, ErrSearchConflict
	}
	if s.busy {
		return nil, ErrSearchBusy
	}
	if s.drv == nil {
		return nil, fmt.Errorf("session has no driver")
	}
	if filter && !s.active {
		return nil, fmt.Errorf("no active search session")
	}
	if filter && len(s.candidates) == 0 {
		return nil, fmt.Errorf("no candidates remain to filter")
	}
	ctx, cancel := context.WithCancel(driver.Context(s.drv))
	s.busy = true
	s.cancel = cancel
	op := &scanOperation{s: s, revision: s.revision, pid: s.pid, vt: s.valueType, drv: driver.WithContext(s.drv, ctx), ctx: ctx, cancel: cancel, truncated: s.truncated}
	if filter {
		op.candidates = cloneCandidates(s.candidates)
	}
	return op, nil
}
func cloneCandidates(c map[uintptr][]byte) map[uintptr][]byte {
	out := make(map[uintptr][]byte, len(c))
	for a, v := range c {
		out[a] = bytes.Clone(v)
	}
	return out
}
func (op *scanOperation) finish() {
	op.cancel()
	op.s.mu.Lock()
	op.s.busy = false
	op.s.cancel = nil
	op.s.mu.Unlock()
}
func (op *scanOperation) commit(vt ValueType, c map[uintptr][]byte, truncated bool) error {
	s := op.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.invalid || s.revision != op.revision || op.ctx.Err() != nil {
		return ErrSearchConflict
	}
	s.valueType = vt
	s.candidates = c
	s.active = true
	s.truncated = truncated
	s.revision++
	return nil
}
func (s *Session) SearchPattern(p Pattern) (PatternResult, error) {
	op, err := s.begin(false)
	if err != nil {
		return PatternResult{}, err
	}
	defer op.finish()
	result, err := SearchPattern(op.drv, op.pid, p)
	if err != nil {
		return PatternResult{}, err
	}
	err = op.commit(TypeBytes, result.CandidateMap(), result.Truncated)
	return result, err
}
func (s *Session) SearchString(value string, enc StringEncoding) (PatternResult, error) {
	op, err := s.begin(false)
	if err != nil {
		return PatternResult{}, err
	}
	defer op.finish()
	result, err := SearchString(op.drv, op.pid, value, enc)
	if err != nil {
		return PatternResult{}, err
	}
	err = op.commit(TypeBytes, result.CandidateMap(), result.Truncated)
	return result, err
}

// Export captures type, identity and candidates in one revision.
func (s *Session) Export() (int, ValueType, string, map[uintptr][]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pid, s.valueType, s.identity, cloneCandidates(s.candidates), s.truncated
}

func (s *Session) SetTruncated(value bool) { s.mu.Lock(); defer s.mu.Unlock(); s.truncated = value }

func (s *Session) Driver() driver.Driver { s.mu.Lock(); defer s.mu.Unlock(); return s.drv }
func (s *Session) TypedPage(offset, limit int) ([]Candidate, int, ValueType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page, total := s.pageLocked(offset, limit)
	return page, total, s.valueType
}
