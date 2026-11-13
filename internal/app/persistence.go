package app

import (
	"fmt"

	"memdroid/internal/memory/store"
)

func (s *State) Save(path string) error {
	return store.SaveState(path, s.GetBookmarks(), s.GetSession())
}

// Load validates into temporary data before publishing either bookmarks or a
// live session. Candidates must belong to the same device, boot and process.
func (s *State) Load(path string) (int, error) {
	s.processMu.Lock()
	defer s.processMu.Unlock()
	if s.closed {
		return 0, fmt.Errorf("application is closing")
	}
	bookmarks := store.NewBookmarkList()
	loaded, err := store.LoadState(path, bookmarks)
	if err != nil {
		return 0, err
	}
	s.mu.RLock()
	pid, target, identity := s.pid, s.target, s.identities[s.pid]
	s.mu.RUnlock()
	count := 0
	if loaded != nil {
		if pid == 0 || target == nil || loaded.PID() != pid || loaded.Identity() == "" || loaded.Identity() != identity {
			return 0, fmt.Errorf("saved candidates do not match the attached process; attach the original process or start a new search")
		}
		if d, ok := s.drv.(interface{ ProcessIdentity(int) (string, error) }); ok {
			current, err := d.ProcessIdentity(pid)
			if err != nil {
				return 0, err
			}
			if current != identity {
				return 0, fmt.Errorf("attached process identity changed; reattach and search again")
			}
		}
		loaded.SetDriver(target)
		count = loaded.CandidateCount()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil {
		s.valueType = s.session.Type()
		s.session.Invalidate()
	}
	s.bookmarks.Replace(bookmarks.Entries())
	s.session = loaded
	if loaded != nil {
		s.valueType = loaded.Type()
	}
	return count, nil
}
