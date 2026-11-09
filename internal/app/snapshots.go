package app

import (
	"slices"
	"sync"
)

// maxSnapshotBytes bounds the total size of retained region snapshots. Each
// /api/snapshot/take keeps its buffer for a later diff; without a cap a long
// session could pin gigabytes.
const maxSnapshotBytes = 64 << 20

// --- Snapshots ---

func (s *State) SetSnapshot(addr uintptr, data []byte) {
	s.snapshots.set(addr, data)
}

func (s *State) GetSnapshot(addr uintptr) []byte {
	return s.snapshots.get(addr)
}

// snapshotStore keeps region snapshots for later diffing, evicting the oldest
// once the retained bytes exceed maxBytes.
type snapshotStore struct {
	mu       sync.Mutex
	maxBytes int
	bytes    int
	data     map[uintptr][]byte
	order    []uintptr // insertion order, oldest first
}

func newSnapshotStore(maxBytes int) *snapshotStore {
	return &snapshotStore{maxBytes: maxBytes, data: make(map[uintptr][]byte)}
}

func (st *snapshotStore) set(addr uintptr, data []byte) {
	cp := slices.Clone(data)

	st.mu.Lock()
	defer st.mu.Unlock()

	if old, ok := st.data[addr]; ok {
		st.bytes -= len(old)
		st.order = slices.DeleteFunc(st.order, func(a uintptr) bool { return a == addr })
	}
	st.data[addr] = cp
	st.order = append(st.order, addr)
	st.bytes += len(cp)

	for st.bytes > st.maxBytes && len(st.order) > 1 {
		oldest := st.order[0]
		st.order = st.order[1:]
		st.bytes -= len(st.data[oldest])
		delete(st.data, oldest)
	}
}

func (st *snapshotStore) get(addr uintptr) []byte {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.data[addr]
}
