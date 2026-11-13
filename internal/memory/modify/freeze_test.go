package modify

import (
	"testing"

	"memdroid/internal/driver/drivertest"
	"memdroid/internal/memory/search"
	"memdroid/internal/poller"
)

func TestFreezeAllRejectsTooManyCandidatesBeforeStarting(t *testing.T) {
	d := drivertest.New()
	s := search.NewSession(1, search.TypeInt32, d)
	candidates := make(map[uintptr][]byte)
	for i := range poller.MaxTasks + 1 {
		candidates[uintptr(i)] = []byte{1, 0, 0, 0}
	}
	s.SetCandidates(candidates)
	f := NewFreezer()
	defer f.UnfreezeAll()
	n, err := f.FreezeAllCandidates(d, s)
	if err == nil || n != 0 || len(f.List()) != 0 {
		t.Fatalf("count=%d err=%v tasks=%v", n, err, f.List())
	}
}
func TestFreezeRejectsOversizedValue(t *testing.T) {
	f := NewFreezer()
	defer f.UnfreezeAll()
	if err := f.Freeze(drivertest.New(), 1, 0x1000, make([]byte, (64<<10)+1)); err == nil {
		t.Fatal("oversized freeze accepted")
	}
}
