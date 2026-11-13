package app

import (
	"fmt"

	"memdroid/internal/driver"
	"memdroid/internal/memory/search"
)

// Target is one coherent attachment snapshot for an API/CLI operation. Driver
// access is revoked by transitions even if PID numbers happen to be reused.
type Target struct {
	PID       int
	Driver    driver.Driver
	Session   *search.Session
	ValueType search.ValueType
}

func (s *State) Capture() (Target, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pid <= 0 || s.target == nil || s.target.Context().Err() != nil {
		return Target{}, fmt.Errorf("not attached")
	}
	if s.session == nil {
		s.session = search.NewSession(s.pid, s.valueType, s.target)
		s.session.SetIdentity(s.identities[s.pid])
	}
	return Target{PID: s.pid, Driver: s.target, Session: s.session, ValueType: s.session.Type()}, nil
}

func (s *State) SetSnapshotFor(t Target, addr uintptr, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.target != t.Driver || s.target.Context().Err() != nil {
		return fmt.Errorf("process changed")
	}
	s.snapshots.set(addr, data)
	return nil
}
func (s *State) SnapshotFor(t Target, addr uintptr) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.target != t.Driver || s.target.Context().Err() != nil {
		return nil, fmt.Errorf("process changed")
	}
	return s.snapshots.get(addr), nil
}
