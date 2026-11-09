package app

import "memdroid/internal/memory/store"

// Save persists the current bookmarks and search candidates.
func (s *State) Save(path string) error {
	return store.SaveState(path, s.GetBookmarks(), s.GetSession())
}

// Load restores a session and binds it to the live driver before publishing it.
// It returns the number of restored candidates.
func (s *State) Load(path string) (int, error) {
	loaded, err := store.LoadState(path, s.GetBookmarks())
	if err != nil {
		return 0, err
	}
	count := 0
	if loaded != nil {
		loaded.SetDriver(s.GetDriver())
		count = loaded.CandidateCount()
	}
	s.SetSession(loaded)
	return count, nil
}
