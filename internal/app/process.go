package app

import (
	"errors"
	"slices"

	"memdroid/internal/memory/search"
)

// AttachedProcess holds info about an attached process.
type AttachedProcess struct {
	PID  int
	Name string
}

var ErrProcessNotAttached = errors.New("pid is not attached")

// Attach pauses the target and publishes its PID and fresh search session as
// one state change. Both frontends use this operation.
func (s *State) Attach(pid int, name string) error {
	s.processMu.Lock()
	defer s.processMu.Unlock()
	if err := s.drv.Attach(pid); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attached[pid] = name
	s.activateLocked(pid)
	return nil
}

// SwitchProcess activates an already attached process with a fresh session.
func (s *State) SwitchProcess(pid int) error {
	s.processMu.Lock()
	defer s.processMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.attached[pid]; !ok {
		return ErrProcessNotAttached
	}
	s.activateLocked(pid)
	return nil
}

// activateLocked requires s.mu to be held for writing.
func (s *State) activateLocked(pid int) {
	s.pid = pid
	s.session = search.NewSession(pid, s.valueType, s.drv)
}

func (s *State) GetPID() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pid
}

func (s *State) SetPID(pid int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pid = pid
}

// --- Multi-attach ---

func (s *State) AddAttached(pid int, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attached[pid] = name
}

func (s *State) RemoveAttached(pid int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.attached, pid)
}

// ListAttached returns every attached process, ordered by PID so the CLI's
// numbered selection list is stable between renders.
func (s *State) ListAttached() []AttachedProcess {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sortedAttachedLocked()
}

// Detach stops all background activity for the active process, detaches it, and
// promotes the next attached process (if any) to active. It returns the PID
// that was detached and the process now active, or 0 when none remains.
//
// The CLI and the HTTP API both need exactly this sequence; keeping it here
// stops the two paths from drifting apart.
func (s *State) Detach() (detached int, next AttachedProcess) {
	s.processMu.Lock()
	defer s.processMu.Unlock()
	// Keep device I/O outside the state lock so reads remain available.
	s.mu.Lock()
	pid := s.pid
	if pid == 0 {
		s.mu.Unlock()
		return 0, AttachedProcess{}
	}
	delete(s.attached, pid)
	drv := s.drv
	s.mu.Unlock()

	s.Freezer.UnfreezeAll()
	s.Watcher.UnwatchAll()
	s.AlertWatcher.RemoveAll()
	drv.Detach(pid)

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.sortedAttachedLocked() {
		s.activateLocked(p.PID)
		return pid, p
	}
	s.pid = 0
	s.session = nil
	return pid, AttachedProcess{}
}

// sortedAttachedLocked returns the attached processes ordered by PID. The
// caller must hold s.mu.
func (s *State) sortedAttachedLocked() []AttachedProcess {
	procs := make([]AttachedProcess, 0, len(s.attached))
	for pid, name := range s.attached {
		procs = append(procs, AttachedProcess{PID: pid, Name: name})
	}
	slices.SortFunc(procs, func(a, b AttachedProcess) int { return a.PID - b.PID })
	return procs
}
