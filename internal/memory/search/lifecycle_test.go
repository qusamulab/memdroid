package search

import (
	"errors"
	"testing"

	"memdroid/internal/driver"
	"memdroid/internal/driver/drivertest"
)

type blockedScanDriver struct {
	*drivertest.Fake
	started chan struct{}
	release chan struct{}
}

func (d *blockedScanDriver) ReadRegion(pid int, addr uintptr, n int) ([]byte, error) {
	close(d.started)
	<-d.release
	return d.Fake.ReadRegion(pid, addr, n)
}
func TestScanRejectsResultsAfterInvalidation(t *testing.T) {
	for _, action := range []string{"reset", "type", "invalidate", "replacement"} {
		t.Run(action, func(t *testing.T) {
			d := &blockedScanDriver{Fake: drivertest.New(drivertest.Region{Start: 0x1000, Data: le32(7)}), started: make(chan struct{}), release: make(chan struct{})}
			s := NewSession(1, TypeInt32, d)
			done := make(chan error, 1)
			go func() { done <- s.Search(le32(7)) }()
			<-d.started
			if err := s.Search(le32(7)); !errors.Is(err, ErrSearchBusy) {
				t.Errorf("concurrent Search = %v", err)
			}
			switch action {
			case "reset":
				s.Reset()
			case "type":
				s.SetType(TypeInt64)
			case "invalidate":
				s.Invalidate()
			case "replacement":
				s.SetCandidates(map[uintptr][]byte{0x2000: le32(9)})
			}
			close(d.release)
			if err := <-done; !errors.Is(err, ErrSearchConflict) {
				t.Errorf("old search returned %v", err)
			}
			if _, ok := s.Snapshot()[0x1000]; ok {
				t.Fatal("stale scan repopulated candidates")
			}
		})
	}
}

type hugeFilterDriver struct {
	*drivertest.Fake
	maxRead int
	reads   int
}

func (d *hugeFilterDriver) ReadMaps(int) ([]driver.Region, error) {
	return []driver.Region{{Start: 0x1000, End: 0x1000 + (1 << 30)}}, nil
}
func (d *hugeFilterDriver) ReadRegion(_ int, _ uintptr, n int) ([]byte, error) {
	d.maxRead = max(d.maxRead, n)
	d.reads++
	return le32(7), nil
}
func TestFilterUsesBoundedWindowsForHugeMappings(t *testing.T) {
	d := &hugeFilterDriver{Fake: drivertest.New()}
	s := NewSession(1, TypeInt32, d)
	s.SetCandidates(map[uintptr][]byte{0x1000: le32(7), 0x1000 + (512 << 20): le32(7)})
	if err := s.Filter(FilterUnchanged, nil); err != nil {
		t.Fatal(err)
	}
	if d.maxRead > scanChunkBytes || d.reads != 2 || s.CandidateCount() != 2 {
		t.Fatalf("read size=%d, reads=%d, candidates=%d", d.maxRead, d.reads, s.CandidateCount())
	}
}
func TestScanLimitBoundsMatchesBeforeResultTrimming(t *testing.T) {
	d := drivertest.New(drivertest.Region{Start: 0x1000, Data: make([]byte, 65536)}, drivertest.Region{Start: 0x20000, Data: make([]byte, 65536)})
	regions, _ := d.ReadMaps(1)
	found := scanRegions(d, 1, regions, 1, equalScan([]byte{0}, 1, 1), newScanLimit(PatternMaxResults+1))
	if len(found) != PatternMaxResults+1 {
		t.Fatalf("retained %d matches", len(found))
	}
}
func TestValueSearchReportsCandidateTruncation(t *testing.T) {
	d := drivertest.New(drivertest.Region{Start: 0x1000, Data: make([]byte, (MaxCandidates+10)*4)})
	s := NewSession(1, TypeInt32, d)
	if err := s.Search(le32(0)); err != nil {
		t.Fatal(err)
	}
	if !s.Truncated() || s.CandidateCount() != MaxCandidates {
		t.Fatalf("count=%d truncated=%v", s.CandidateCount(), s.Truncated())
	}
	if err := s.Filter(FilterUnchanged, nil); err != nil {
		t.Fatal(err)
	}
	if !s.Truncated() {
		t.Fatal("filter lost incomplete-result marker")
	}
	s.Reset()
	if s.Truncated() {
		t.Fatal("reset retained truncation marker")
	}
}
