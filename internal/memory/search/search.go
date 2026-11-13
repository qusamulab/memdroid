package search

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"

	"memdroid/internal/driver"
)

const (
	// scanWorkers limits concurrent ADB calls during parallel memory reads.
	scanWorkers = 8

	// Candidate budgets bound map overhead as well as retained payload.
	MaxCandidates     = 100000
	MaxCandidateBytes = 16 << 20

	// scanChunkBytes bounds how much of a region is held in memory at once.
	// Without it a single multi-gigabyte mapping, times scanWorkers, would
	// exhaust RAM; regions larger than this are read in overlapping chunks so
	// no match is lost at a boundary.
	scanChunkBytes = 32 << 20
)

// emitFunc records one match found by a scanFunc.
type emitFunc func(addr uintptr, val []byte)

// scanFunc examines buf, which holds the bytes mapped at address base, and
// reports every match through emit. Implementations must not retain buf.
type scanFunc func(buf []byte, base uintptr, emit emitFunc)

// Search scans all rw memory regions for target and stores results in the session.
func (s *Session) Search(target []byte) error {
	return s.SearchFiltered(target, driver.RegionAll, 0, 0)
}

// SearchFiltered scans memory with a region filter applied.
func (s *Session) SearchFiltered(target []byte, filter driver.RegionFilter, customStart, customEnd uintptr) error {
	return s.searchTyped(target, nil, filter, customStart, customEnd)
}

// SearchTyped keeps request parsing and scan width together without publishing
// a type change before the scan succeeds.
func (s *Session) SearchTyped(target []byte, vt ValueType, filter driver.RegionFilter, start, end uintptr) error {
	return s.searchTyped(target, &vt, filter, start, end)
}
func (s *Session) searchTyped(target []byte, requested *ValueType, filter driver.RegionFilter, customStart, customEnd uintptr) error {
	op, err := s.begin(false)
	if err != nil {
		return err
	}
	defer op.finish()
	pid, vt, drv := op.pid, op.vt, op.drv
	if requested != nil {
		vt = *requested
	}

	regions, err := drv.ReadMapsFiltered(pid, filter, customStart, customEnd)
	if err != nil {
		return err
	}

	// Fixed-width types are scanned on their natural alignment (as Cheat Engine
	// does); a byte sequence can start anywhere, so it uses stride 1.
	width, stride := len(target), 1
	if vt != TypeBytes {
		width = vt.Size()
		stride = width
	}

	if len(target) == 0 || len(target) != width {
		return fmt.Errorf("target width does not match the active type")
	}
	capCount := min(MaxCandidates, MaxCandidateBytes/width)
	if capCount == 0 {
		return fmt.Errorf("search value exceeds candidate byte budget")
	}
	found := scanRegions(drv, pid, regions, width, equalScan(target, width, stride), newScanLimit(capCount+1))
	truncated := len(found) > capCount
	if truncated {
		for addr := range found {
			delete(found, addr)
			break
		}
	}
	return op.commit(vt, found, truncated)
}

// scanLimit caps how many matches a scan collects across all workers. A nil
// *scanLimit means unlimited.
type scanLimit struct {
	max int
	n   atomic.Int64
}

func newScanLimit(n int) *scanLimit { return &scanLimit{max: n} }

func (l *scanLimit) reached() bool {
	return l != nil && l.max > 0 && l.n.Load() >= int64(l.max)
}

func (l *scanLimit) add(d int) {
	if l != nil {
		l.n.Add(int64(d))
	}
}

// equalScan builds a scanFunc matching target at every stride-aligned offset.
func equalScan(target []byte, width, stride int) scanFunc {
	return func(buf []byte, base uintptr, emit emitFunc) {
		// Align to the absolute address rather than to the start of this
		// buffer, so a chunked read visits the same offsets a single read
		// would even when a later chunk begins on an unaligned address.
		start := 0
		if stride > 1 {
			start = int((uintptr(stride) - base%uintptr(stride)) % uintptr(stride))
		}
		for i := start; i+width <= len(buf); i += stride {
			if bytes.Equal(buf[i:i+width], target) {
				emit(base+uintptr(i), buf[i:i+width])
			}
		}
	}
}

// scanRegions reads every region (in parallel, bounded by scanWorkers) and
// invokes scan on each buffer to collect matches. Regions smaller than width
// are skipped, and oversized regions are read in scanChunkBytes pieces that
// overlap by width-1 bytes so a match straddling a chunk boundary is still
// found. It is the shared engine behind value, byte-sequence and pattern scans.
//
// limit may be nil for an unbounded scan; otherwise workers stop once it is
// reached, so the result may be a prefix of all matches.
func scanRegions(drv driver.Driver, pid int, regions []driver.Region, width int, scan scanFunc, limit *scanLimit) map[uintptr][]byte {
	if width <= 0 {
		return map[uintptr][]byte{}
	}

	found := make(map[uintptr][]byte)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, scanWorkers)

	for _, r := range regions {
		size := int(r.End - r.Start)
		if size < width {
			continue
		}
		if limit.reached() {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(r driver.Region, size int) {
			defer wg.Done()
			defer func() { <-sem }()

			local := make(map[uintptr][]byte)
			emit := func(addr uintptr, val []byte) {
				if _, exists := local[addr]; exists {
					return
				}
				if limit != nil && !limit.reserve() {
					return
				}
				local[addr] = bytes.Clone(val)
			}
			scanRegionChunked(drv, pid, r, size, width, scan, emit, limit)

			if len(local) == 0 {
				return
			}
			mu.Lock()
			for addr, val := range local {
				found[addr] = val
			}
			mu.Unlock()
		}(r, size)
	}
	wg.Wait()
	return found
}

// scanRegionChunked walks one region in bounded, overlapping pieces. A read
// failure ends the region: mappings can disappear or contain guard pages, and
// there is nothing useful to report for the remainder.
func scanRegionChunked(drv driver.Driver, pid int, r driver.Region, size, width int, scan scanFunc, emit emitFunc, limit *scanLimit) {
	overlap := width - 1
	for off := 0; off < size; {
		if limit.reached() || driver.Context(drv).Err() != nil {
			return
		}
		n := min(scanChunkBytes, size-off)
		base := r.Start + uintptr(off)
		buf, err := drv.ReadRegion(pid, base, n)
		if err != nil || len(buf) < width {
			return
		}
		scan(buf, base, emit)

		if len(buf) < n {
			return // short read — end of the readable area
		}
		if off+n >= size {
			return // this chunk reached the end of the region
		}
		// Step back by overlap so a match spanning the boundary is still seen.
		advance := n - overlap
		if advance <= 0 {
			return // chunk smaller than the overlap; no forward progress possible
		}
		off += advance
	}
}

func (l *scanLimit) reserve() bool {
	if l == nil || l.max <= 0 {
		return true
	}
	for {
		n := l.n.Load()
		if n >= int64(l.max) {
			return false
		}
		if l.n.CompareAndSwap(n, n+1) {
			return true
		}
	}
}
