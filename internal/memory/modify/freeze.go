package modify

import (
	"context"
	"fmt"
	"sync"
	"time"

	"memdroid/internal/driver"
	"memdroid/internal/memory/search"
	"memdroid/internal/poller"
)

const defaultFreezeInterval = 100 * time.Millisecond

// Freezer manages a set of frozen addresses, each periodically re-written to
// hold its value. Task lifecycle is delegated to poller.Pool.
type Freezer struct {
	pool *poller.Pool

	mu       sync.Mutex
	interval time.Duration
}

func NewFreezer() *Freezer {
	return &Freezer{
		pool:     poller.New(),
		interval: defaultFreezeInterval,
	}
}

// SetInterval changes the freeze write interval for newly frozen addresses.
func (f *Freezer) SetInterval(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.interval = d
}

// GetInterval returns the current freeze interval.
func (f *Freezer) GetInterval() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.interval
}

// Freeze starts a goroutine that repeatedly writes value to addr at the
// configured interval. Returns an error if addr is already frozen.
func (f *Freezer) Freeze(drv driver.Driver, pid int, addr uintptr, value []byte) error {
	if len(value) == 0 || len(value) > 64<<10 {
		return fmt.Errorf("freeze value must contain 1-65536 bytes")
	}
	val := make([]byte, len(value))
	copy(val, value)
	iv := f.GetInterval()
	if iv <= 0 {
		return fmt.Errorf("freeze interval must be positive")
	}

	return f.pool.StartContext(driver.Context(drv), addr, func(ctx context.Context) {
		drv = driver.WithContext(drv, ctx)
		f.pool.Every(ctx, iv, func() {
			_ = drv.Poke(pid, addr, val)
		})
	})
}

// FreezeAllCandidates freezes every address in the session with its last-seen value.
func (f *Freezer) FreezeAllCandidates(drv driver.Driver, s *search.Session) (int, error) {
	if s.CandidateCount()+len(f.List()) > poller.MaxTasks {
		return 0, fmt.Errorf("freeze limit (%d) exceeded; narrow the candidates", poller.MaxTasks)
	}
	pid := s.PID()
	count := 0
	for addr, val := range s.Snapshot() {
		if err := f.Freeze(drv, pid, addr, val); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// Unfreeze stops the freeze goroutine for addr.
func (f *Freezer) Unfreeze(addr uintptr) error { return f.pool.Stop(addr) }

// UnfreezeAll stops all freeze goroutines.
func (f *Freezer) UnfreezeAll() { f.pool.StopAll() }

// List returns all currently frozen addresses.
func (f *Freezer) List() []uintptr { return f.pool.List() }
