package app

import (
	"memdroid/internal/memory/search"
)

// --- ValueType ---

// GetValueType returns the active value type. Once a search session exists the
// session owns the type, so that a scan and the formatting of its results can
// never disagree about the byte width.
func (s *State) GetValueType() search.ValueType {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.session != nil {
		return s.session.Type()
	}
	return s.valueType
}

// SetValueType changes the active value type, propagating it to the live
// session. Changing the type discards existing candidates, since they were
// recorded at a different width.
func (s *State) SetValueType(vt search.ValueType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.valueType = vt
	if s.session != nil {
		s.session.SetType(vt)
	}
}

// --- Session ---

func (s *State) GetSession() *search.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.session
}

func (s *State) SetSession(sess *search.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil && s.session != sess {
		s.session.Invalidate()
	}
	s.session = sess
	if sess != nil {
		// The session becomes the owner of the type; keep the fallback in sync
		// so it is correct again once the session is cleared.
		s.valueType = sess.Type()
	}
}

// NewSession replaces the session with a fresh one for pid at the current
// value type.
func (s *State) NewSession(pid int) *search.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil {
		s.session.Invalidate()
	}
	drv := s.drv
	if s.target != nil {
		drv = s.target
	}
	s.session = search.NewSession(pid, s.valueType, drv)
	s.session.SetIdentity(s.identities[pid])
	return s.session
}

// EnsureSession returns the active session, creating one for the current PID
// and value type if none exists.
func (s *State) EnsureSession() *search.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		drv := s.drv
		if s.target != nil {
			drv = s.target
		}
		s.session = search.NewSession(s.pid, s.valueType, drv)
		s.session.SetIdentity(s.identities[s.pid])
	}
	return s.session
}
