package app

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"memdroid/internal/driver/adb"
	"memdroid/internal/driver/drivertest"
	"memdroid/internal/memory/search"
)

func TestAttachRejectsNonpositivePID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		s, d := newTestState()
		if err := s.Attach(pid, "invalid"); err == nil {
			t.Errorf("Attach(%d) succeeded", pid)
		}
		if d.Attached(pid) {
			t.Fatal("invalid PID reached driver")
		}
	}
}
func TestCloseResumesAllAndRejectsLateOperations(t *testing.T) {
	s, d := newTestState()
	for _, pid := range []int{1, 2} {
		if err := s.Attach(pid, "app"); err != nil {
			t.Fatal(err)
		}
	}
	old := s.GetDriver()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, pid := range []int{1, 2} {
		if d.Attached(pid) {
			t.Errorf("PID %d still attached", pid)
		}
	}
	if err := old.Poke(2, 0x1000, []byte{9}); err == nil {
		t.Fatal("old operation survived Close")
	}
	if err := s.GetDriver().Poke(2, 0x1000, []byte{9}); err == nil {
		t.Fatal("unattached driver allowed write")
	}
	if err := s.Attach(3, "late"); err == nil {
		t.Fatal("Attach after Close succeeded")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestSwitchRevokesOldHandlesAndResources(t *testing.T) {
	s, _ := newTestState()
	if err := s.Attach(1, "first"); err != nil {
		t.Fatal(err)
	}
	old, _ := s.Capture()
	s.SetSnapshot(0x1000, []byte{1})
	if err := s.UndoStack.WithUndo(old.Driver, 1, 0x1000, []byte{3}); err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(2, "second"); err != nil {
		t.Fatal(err)
	}
	if err := old.Driver.Poke(1, 0x1000, []byte{9}); err == nil {
		t.Fatal("stale driver wrote after switch")
	}
	if s.UndoStack.Depth() != 0 || s.GetSnapshot(0x1000) != nil {
		t.Fatal("old process resources survived switch")
	}
	if err := s.SetSnapshotFor(old, 0x1000, []byte{5}); err == nil {
		t.Fatal("old snapshot committed after switch")
	}
}
func TestLoadRejectsWrongProcessWithoutPartialPublication(t *testing.T) {
	s, d := newTestState()
	if err := s.Attach(1, "first"); err != nil {
		t.Fatal(err)
	}
	s.GetSession().SetCandidatesAs(search.TypeInt32, map[uintptr][]byte{0x1000: {7, 0, 0, 0}})
	s.GetBookmarks().Add(0x1000, "saved", search.TypeInt32)
	path := filepath.Join(t.TempDir(), "session.json")
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	other := NewState(d)
	if _, err := other.Load(path); err == nil {
		t.Fatal("unattached load accepted candidates")
	}
	if err := other.Attach(2, "second"); err != nil {
		t.Fatal(err)
	}
	previous := other.GetSession()
	other.GetBookmarks().Add(0x2000, "current", search.TypeInt32)
	if _, err := other.Load(path); err == nil {
		t.Fatal("different PID accepted candidates")
	}
	if other.GetSession() != previous || other.GetBookmarks().Entries()[0].Label != "current" {
		t.Fatal("failed load partially published state")
	}
}

type failedDetachDriver struct {
	*drivertest.Fake
	fail bool
}

func (d *failedDetachDriver) Detach(pid int) error {
	if d.fail {
		return errors.New("device disconnected")
	}
	return d.Fake.Detach(pid)
}
func TestCloseRetainsFailedDetachForRetry(t *testing.T) {
	d := &failedDetachDriver{Fake: drivertest.New(), fail: true}
	s := NewState(d)
	if err := s.Attach(1, "app"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err == nil || len(s.ListAttached()) != 1 {
		t.Fatal("failed detach was lost")
	}
	d.fail = false
	if err := s.Close(); err != nil || len(s.ListAttached()) != 0 {
		t.Fatal("retry failed")
	}
}

type blockedFreezeDriver struct {
	*drivertest.Fake
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
	detached chan struct{}
}

func (d *blockedFreezeDriver) Poke(pid int, addr uintptr, b []byte) error {
	close(d.started)
	<-d.release
	err := d.Fake.Poke(pid, addr, b)
	close(d.finished)
	return err
}
func (d *blockedFreezeDriver) Detach(pid int) error {
	close(d.detached)
	return d.Fake.Detach(pid)
}
func TestDetachWaitsForInFlightFreeze(t *testing.T) {
	d := &blockedFreezeDriver{Fake: drivertest.New(drivertest.Region{Start: 0x1000, Data: []byte{1}}), started: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{}), detached: make(chan struct{})}
	s := NewState(d)
	if err := s.Attach(1, "app"); err != nil {
		t.Fatal(err)
	}
	old := s.GetDriver()
	s.Freezer.SetInterval(time.Millisecond)
	if err := s.Freezer.Freeze(old, 1, 0x1000, []byte{2}); err != nil {
		t.Fatal(err)
	}
	<-d.started
	done := make(chan error, 1)
	go func() { _, _, err := s.Detach(); done <- err }()
	<-old.(*targetDriver).Context().Done()
	select {
	case <-d.detached:
		t.Error("detach ran before freeze completed")
	default:
	}
	close(d.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	<-d.finished
	if len(s.Freezer.List()) != 0 {
		t.Fatal("freeze remained registered")
	}
}

type identityDriver struct {
	*drivertest.Fake
	identity string
}

func (d *identityDriver) ProcessIdentity(int) (string, error) { return d.identity, nil }
func TestLoadRejectsPIDReuseAndDeviceChanges(t *testing.T) {
	d := &identityDriver{Fake: drivertest.New(), identity: "device-A:boot:1:100"}
	s := NewState(d)
	if err := s.Attach(1, "app"); err != nil {
		t.Fatal(err)
	}
	s.GetSession().SetCandidates(map[uintptr][]byte{0x1000: {7, 0, 0, 0}})
	path := filepath.Join(t.TempDir(), "session.json")
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	for _, identity := range []string{"device-A:boot:1:200", "device-B:boot:1:100", "device-A:new-boot:1:100"} {
		d.identity = identity
		if _, err := s.Load(path); err == nil {
			t.Fatalf("accepted identity %s", identity)
		}
	}
}

func TestUnattachedDriverStillListsProcesses(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no ADB invocation can reach a device
	s := NewState(adb.New())
	_, err := s.GetDriver().ListProcesses()
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("unattached catalogue request used revoked transport: %v", err)
	}
}
