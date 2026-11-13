package app

import (
	"context"
	"fmt"
	"sync"

	"memdroid/internal/driver"
)

// targetLease prevents late CLI requests and background tasks from using an
// attachment after a transition. Only transport calls hold mu, never prompts.
type targetLease struct {
	mu     sync.RWMutex
	closed bool
	pid    int
	ctx    context.Context
	cancel context.CancelFunc
}

type targetDriver struct {
	driver.Driver
	lease *targetLease
	ctx   context.Context
}

func newTarget(d driver.Driver, pid int) *targetDriver {
	ctx, cancel := context.WithCancel(context.Background())
	bound := d
	if pid > 0 {
		bound = driver.WithContext(d, ctx)
	}
	return &targetDriver{Driver: bound, lease: &targetLease{pid: pid, ctx: ctx, cancel: cancel}, ctx: ctx}
}
func (d *targetDriver) Context() context.Context { return d.ctx }
func (d *targetDriver) WithContext(ctx context.Context) driver.Driver {
	return &targetDriver{Driver: driver.WithContext(d.Driver, ctx), lease: d.lease, ctx: ctx}
}
func targetCall[T any](d *targetDriver, pid int, fn func() (T, error)) (T, error) {
	d.lease.mu.RLock()
	defer d.lease.mu.RUnlock()
	var zero T
	if d.lease.closed {
		return zero, context.Canceled
	}
	if pid != d.lease.pid {
		return zero, fmt.Errorf("process changed; retry the operation")
	}
	if err := d.lease.ctx.Err(); err != nil {
		return zero, err
	}
	if err := d.ctx.Err(); err != nil {
		return zero, err
	}
	return fn()
}
func (d *targetDriver) Peek(pid int, addr uintptr, n int) ([]byte, error) {
	return targetCall(d, pid, func() ([]byte, error) { return d.Driver.Peek(pid, addr, n) })
}
func (d *targetDriver) ReadBytes(pid int, addr uintptr, n int) ([]byte, error) {
	return targetCall(d, pid, func() ([]byte, error) { return d.Driver.ReadBytes(pid, addr, n) })
}
func (d *targetDriver) ReadRegion(pid int, addr uintptr, n int) ([]byte, error) {
	return targetCall(d, pid, func() ([]byte, error) { return d.Driver.ReadRegion(pid, addr, n) })
}
func (d *targetDriver) ReadMaps(pid int) ([]driver.Region, error) {
	return targetCall(d, pid, func() ([]driver.Region, error) { return d.Driver.ReadMaps(pid) })
}
func (d *targetDriver) ReadMapsFiltered(pid int, f driver.RegionFilter, a, b uintptr) ([]driver.Region, error) {
	return targetCall(d, pid, func() ([]driver.Region, error) { return d.Driver.ReadMapsFiltered(pid, f, a, b) })
}
func (d *targetDriver) Poke(pid int, addr uintptr, val []byte) error {
	_, err := targetCall(d, pid, func() (struct{}, error) { return struct{}{}, d.Driver.Poke(pid, addr, val) })
	return err
}
func (d *targetDriver) Stop(pid int) error {
	_, err := targetCall(d, pid, func() (struct{}, error) { return struct{}{}, d.Driver.Stop(pid) })
	return err
}
func (d *targetDriver) Continue(pid int) error {
	_, err := targetCall(d, pid, func() (struct{}, error) { return struct{}{}, d.Driver.Continue(pid) })
	return err
}

// stopActive is called with processMu held, without holding the status lock.
func (s *State) stopActive() {
	s.mu.RLock()
	target, sess := s.target, s.session
	s.mu.RUnlock()
	if sess != nil {
		s.mu.Lock()
		s.valueType = sess.Type()
		s.mu.Unlock()
		sess.Invalidate()
	}
	if target != nil {
		target.lease.cancel()
	}
	s.Freezer.UnfreezeAll()
	s.Watcher.UnwatchAll()
	s.AlertWatcher.RemoveAll()
	if target != nil {
		target.lease.mu.Lock()
		target.lease.closed = true
		target.lease.mu.Unlock()
	}
	s.UndoStack.Clear()
	s.snapshots.clear()
}
