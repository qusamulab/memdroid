package app

import (
	"errors"
	"sync"
	"testing"

	"memdroid/internal/driver"
	"memdroid/internal/memory/search"
)

type processDriver struct {
	driver.Driver
	mu        sync.Mutex
	detached  map[int]int
	attachErr error
}

func (d *processDriver) Attach(pid int) error {
	if d.attachErr != nil {
		return d.attachErr
	}
	return d.Driver.Attach(pid)
}

func (d *processDriver) Detach(pid int) {
	d.mu.Lock()
	d.detached[pid]++
	d.mu.Unlock()
	d.Driver.Detach(pid)
}

func TestAttachFailureKeepsCurrentProcess(t *testing.T) {
	state, fake := newTestState()
	if err := state.Attach(1, "first"); err != nil {
		t.Fatal(err)
	}
	previous := state.GetSession()
	failure := errors.New("device disconnected")
	state.drv = &processDriver{Driver: fake, attachErr: failure}
	if err := state.Attach(2, "second"); !errors.Is(err, failure) {
		t.Fatalf("Attach error = %v, want %v", err, failure)
	}
	if state.GetPID() != 1 || state.GetSession() != previous || len(state.ListAttached()) != 1 {
		t.Fatal("failed attach changed the active process or session")
	}
}

func TestSwitchProcessValidatesBeforeReplacingSession(t *testing.T) {
	state, _ := newTestState()
	state.SetValueType(search.TypeFloat64)
	for _, pid := range []int{1, 2} {
		if err := state.Attach(pid, "app"); err != nil {
			t.Fatal(err)
		}
	}
	previous := state.GetSession()
	if err := state.SwitchProcess(99); !errors.Is(err, ErrProcessNotAttached) {
		t.Fatalf("SwitchProcess error = %v", err)
	}
	if state.GetSession() != previous || state.GetPID() != 2 {
		t.Fatal("invalid switch changed the active session")
	}
	if err := state.SwitchProcess(1); err != nil {
		t.Fatal(err)
	}
	if state.GetSession() == previous || state.GetSession().PID() != 1 || state.GetSession().Type() != search.TypeFloat64 {
		t.Fatal("switch did not create a fresh session for the selected process and type")
	}
}

func TestConcurrentDetachReleasesEachProcessOnce(t *testing.T) {
	_, fake := newTestState()
	tracked := &processDriver{Driver: fake, detached: make(map[int]int)}
	state := NewState(tracked)
	const processes = 20
	for pid := 1; pid <= processes; pid++ {
		if err := state.Attach(pid, "app"); err != nil {
			t.Fatal(err)
		}
	}
	var workers sync.WaitGroup
	for range processes {
		workers.Go(func() { state.Detach() })
	}
	workers.Wait()
	for pid := 1; pid <= processes; pid++ {
		if tracked.detached[pid] != 1 {
			t.Errorf("PID %d detached %d times, want once", pid, tracked.detached[pid])
		}
	}
	if state.GetPID() != 0 || state.GetSession() != nil || len(state.ListAttached()) != 0 {
		t.Fatal("detach left active process state behind")
	}
}
