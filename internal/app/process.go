package app

import (
	"crypto/rand"
	"errors"
	"fmt"
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
	if pid <= 0 {
		return fmt.Errorf("pid must be positive")
	}
	if s.closed {
		return fmt.Errorf("application is closing")
	}
	if err := s.drv.Attach(pid); err != nil {
		return err
	}
	identity := rand.Text()
	if d, ok := s.drv.(interface{ ProcessIdentity(int) (string, error) }); ok {
		var err error
		identity, err = d.ProcessIdentity(pid)
		if err != nil {
			resumeErr := s.drv.Detach(pid)
			if resumeErr != nil {
				s.mu.Lock()
				s.attached[pid] = name
				s.mu.Unlock()
			}
			return errors.Join(err, resumeErr)
		}
	}
	s.stopActive()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.identities[pid] = identity
	s.attached[pid] = name
	s.activateLocked(pid)
	return nil
}

// SwitchProcess activates an already attached process with a fresh session.
func (s *State) SwitchProcess(pid int) error {
	s.processMu.Lock()
	defer s.processMu.Unlock()
	s.mu.RLock()
	_, ok := s.attached[pid]
	s.mu.RUnlock()
	if !ok || s.closed {
		return ErrProcessNotAttached
	}
	s.stopActive()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activateLocked(pid)
	return nil
}

// activateLocked requires s.mu to be held for writing.
func (s *State) activateLocked(pid int) {
	s.pid = pid
	s.target = newTarget(s.drv, pid)
	s.session = search.NewSession(pid, s.valueType, s.target)
	s.session.SetIdentity(s.identities[pid])
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
func (s *State) Detach() (detached int, next AttachedProcess, err error) {
	s.processMu.Lock()
	defer s.processMu.Unlock()
	s.mu.RLock()
	pid := s.pid
	s.mu.RUnlock()
	if pid == 0 {
		return 0, AttachedProcess{}, nil
	}
	s.stopActive()
	if err := s.drv.Detach(pid); err != nil {
		s.mu.Lock()
		s.activateLocked(pid)
		s.mu.Unlock()
		return 0, AttachedProcess{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.attached, pid)
	delete(s.identities, pid)
	for _, p := range s.sortedAttachedLocked() {
		s.activateLocked(p.PID)
		return pid, p, nil
	}
	s.pid, s.session, s.target = 0, nil, nil
	return pid, AttachedProcess{}, nil
}

// Close rejects future attachments, joins background work and resumes every
// attached process. Failures remain tracked so a subsequent Close can retry.
func (s *State) Close() error {
	s.processMu.Lock()
	defer s.processMu.Unlock()
	s.closed = true
	s.stopActive()
	var errs []error
	for _, p := range s.ListAttached() {
		if err := s.drv.Detach(p.PID); err != nil {
			errs = append(errs, fmt.Errorf("detach PID %d: %w", p.PID, err))
			continue
		}
		s.mu.Lock()
		delete(s.attached, p.PID)
		delete(s.identities, p.PID)
		s.mu.Unlock()
	}
	s.mu.Lock()
	s.pid, s.session, s.target = 0, nil, nil
	s.mu.Unlock()
	return errors.Join(errs...)
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
