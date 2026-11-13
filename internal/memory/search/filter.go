package search

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"

	"memdroid/internal/driver"
)

type FilterMode int

const (
	FilterChanged FilterMode = iota
	FilterUnchanged
	FilterIncreased
	FilterDecreased
	FilterValue
)

func (m FilterMode) valid() bool {
	return m >= FilterChanged && m <= FilterValue
}

// Filter narrows the candidate list by re-reading current values and applying mode.
// For FilterValue, pass the target bytes; otherwise target may be nil.
func (s *Session) Filter(mode FilterMode, target []byte) error {
	if !mode.valid() {
		return fmt.Errorf("invalid filter mode %d", mode)
	}
	op, err := s.begin(true)
	if err != nil {
		return err
	}
	defer op.finish()
	pid, vt, drv := op.pid, op.vt, op.drv
	if mode == FilterValue && len(target) == 0 {
		return fmt.Errorf("filter %q requires a target value", mode)
	}

	regions, err := drv.ReadMaps(pid)
	if err != nil {
		return fmt.Errorf("filter: read maps: %w", err)
	}
	slices.SortFunc(regions, func(a, b driver.Region) int { return cmp.Compare(a.Start, b.Start) })

	entries, _ := planFilterReads(op.candidates, regions)
	next := make(map[uintptr][]byte, len(entries))
	// Entries are sorted. Retain only one bounded window, releasing each buffer
	// as soon as its candidates have been consumed, including across regions.
	var buf []byte
	var base uintptr
	cachedRegion := -1
	for _, e := range entries {
		if op.ctx.Err() != nil {
			return ErrSearchConflict
		}
		sz := len(e.prev)
		if vt != TypeBytes {
			sz = vt.Size()
		}
		var cur []byte
		if e.regIdx >= 0 {
			if cachedRegion != e.regIdx || e.addr < base || e.addr-base+uintptr(sz) > uintptr(len(buf)) {
				base = e.addr
				cachedRegion = e.regIdx
				r := regions[e.regIdx]
				n := min(scanChunkBytes, int(r.End-base))
				buf, _ = drv.ReadRegion(pid, base, n)
			}
			off := int(e.addr - base)
			if off >= 0 && off+sz <= len(buf) {
				cur = buf[off : off+sz]
			}
		}
		if cur == nil {
			c, readErr := drv.Peek(pid, e.addr, sz)
			if readErr != nil || len(c) != sz {
				continue
			}
			cur = c
		}

		if filterKeep(mode, cur, e.prev, target, vt) {
			next[e.addr] = bytes.Clone(cur)
		}
	}

	return op.commit(vt, next, op.truncated)
}

// candEntry pairs a candidate with the index of the region containing it.
type candEntry struct {
	addr   uintptr
	prev   []byte
	regIdx int
}

// planFilterReads pairs every candidate with its containing region (via binary
// search over the sorted region list) and reports which regions must be read.
func planFilterReads(snap map[uintptr][]byte, regions []driver.Region) (entries []candEntry, needed map[int]struct{}) {
	regionFor := func(addr uintptr) int {
		i, _ := slices.BinarySearchFunc(regions, addr, func(r driver.Region, a uintptr) int {
			return cmp.Compare(r.Start, a)
		})
		// BinarySearchFunc returns the first region with Start >= addr; the
		// containing region is that one (exact hit) or the one before it.
		if i < len(regions) && regions[i].Start == addr {
			return i
		}
		i--
		if i >= 0 && addr < regions[i].End {
			return i
		}
		return -1
	}

	entries = make([]candEntry, 0, len(snap))
	needed = make(map[int]struct{})
	for addr, prev := range snap {
		idx := regionFor(addr)
		entries = append(entries, candEntry{addr: addr, prev: prev, regIdx: idx})
		if idx >= 0 {
			needed[idx] = struct{}{}
		}
	}
	slices.SortFunc(entries, func(a, b candEntry) int { return cmp.Compare(a.addr, b.addr) })
	return entries, needed
}

func filterKeep(mode FilterMode, cur, prev, target []byte, vt ValueType) bool {
	switch mode {
	case FilterChanged:
		return !bytes.Equal(cur, prev)
	case FilterUnchanged:
		return bytes.Equal(cur, prev)
	case FilterIncreased:
		return CompareValues(cur, prev, vt) > 0
	case FilterDecreased:
		return CompareValues(cur, prev, vt) < 0
	case FilterValue:
		return bytes.Equal(cur, target)
	}
	return false
}
