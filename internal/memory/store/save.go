package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"memdroid/internal/memory/search"
)

const (
	stateFilePerms = 0o600
	stateVersion   = 2

	// DefaultStateFile is the filename used when the CLI or API is asked to
	// save/load without an explicit path.
	DefaultStateFile = "memdroid.json"
)

type savedBookmark struct {
	Addr  uint64 `json:"addr"`
	Label string `json:"label"`
	VType int    `json:"vtype"`
}

type savedSession struct {
	Identity   string            `json:"identity,omitempty"`
	Truncated  bool              `json:"truncated,omitempty"`
	PID        int               `json:"pid"`
	ValueType  int               `json:"value_type"`
	Candidates map[string][]byte `json:"candidates"`
}

type saveFile struct {
	Version   int             `json:"version"`
	Bookmarks []savedBookmark `json:"bookmarks"`
	Session   *savedSession   `json:"session,omitempty"`
}

// SaveState serializes bookmarks and the active search session to a JSON file.
func SaveState(path string, bl *BookmarkList, s *search.Session) error {
	sf := saveFile{Version: stateVersion}

	for _, b := range bl.Entries() {
		sf.Bookmarks = append(sf.Bookmarks, savedBookmark{
			Addr:  uint64(b.Addr),
			Label: b.Label,
			VType: int(b.VType),
		})
	}

	if s != nil && s.HasCandidates() {
		pid, vt, identity, snap, truncated := s.Export()
		sc := &savedSession{
			PID:        pid,
			ValueType:  int(vt),
			Identity:   identity,
			Truncated:  truncated,
			Candidates: make(map[string][]byte, len(snap)),
		}
		for addr, val := range snap {
			sc.Candidates["0x"+strconv.FormatUint(uint64(addr), 16)] = val
		}
		sf.Session = sc
	}

	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, stateFilePerms)
}

// LoadState deserializes bookmarks (into bl, replacing its contents) and returns
// the saved search session if present. The returned session has a nil Driver;
// the caller must set it before searching.
func LoadState(path string, bl *BookmarkList) (*search.Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	const maxStateBytes = 64 << 20
	data, err := io.ReadAll(io.LimitReader(f, maxStateBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxStateBytes {
		return nil, fmt.Errorf("state file exceeds 64 MiB")
	}

	var sf saveFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, err
	}
	if sf.Version < 0 || sf.Version > stateVersion {
		return nil, fmt.Errorf("unsupported state file version %d (expected %d)", sf.Version, stateVersion)
	}

	entries := make([]Bookmark, 0, len(sf.Bookmarks))
	for _, b := range sf.Bookmarks {
		if _, err := search.ParseValueType(search.ValueType(b.VType).String()); err != nil {
			return nil, err
		}
		entries = append(entries, Bookmark{
			Addr:  uintptr(b.Addr),
			Label: b.Label,
			VType: search.ValueType(b.VType),
		})
	}

	if sf.Session == nil {
		bl.Replace(entries)
		return nil, nil
	}

	vt := search.ValueType(sf.Session.ValueType)
	if _, err := search.ParseValueType(vt.String()); err != nil {
		return nil, err
	}
	if sf.Session.PID <= 0 {
		return nil, fmt.Errorf("saved session PID must be positive")
	}
	if len(sf.Session.Candidates) > search.MaxCandidates {
		return nil, fmt.Errorf("saved candidate count exceeds limit")
	}
	totalBytes := 0
	cands := make(map[uintptr][]byte, len(sf.Session.Candidates))
	for key, val := range sf.Session.Candidates {
		addr, err := strconv.ParseUint(key, 0, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid candidate address %q: %w", key, err)
		}
		if len(val) == 0 || (vt.Size() != 0 && len(val) != vt.Size()) {
			return nil, fmt.Errorf("invalid candidate width at %s", key)
		}
		totalBytes += len(val)
		if totalBytes > search.MaxCandidateBytes {
			return nil, fmt.Errorf("saved candidate bytes exceed limit")
		}
		cands[uintptr(addr)] = val
	}

	// The driver is not serializable; the caller must rebind it via SetDriver.
	loaded := search.NewSession(sf.Session.PID, vt, nil)
	loaded.SetCandidatesAs(vt, cands)
	loaded.SetIdentity(sf.Session.Identity)
	loaded.SetTruncated(sf.Session.Truncated)
	bl.Replace(entries)
	return loaded, nil
}
