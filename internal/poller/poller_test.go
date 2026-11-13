package poller

import "testing"

func TestPoolLifecycle(t *testing.T) {
	p := New()

	started := make(chan struct{})
	done := make(chan struct{})

	err := p.Start(0x10, func(stop <-chan struct{}) {
		close(started)
		<-stop
		close(done)
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started

	if !p.Has(0x10) {
		t.Errorf("Has(0x10) should be true")
	}
	if got := p.List(); len(got) != 1 || got[0] != 0x10 {
		t.Errorf("List = %v, want [0x10]", got)
	}

	// Starting the same key again must fail.
	if err := p.Start(0x10, func(<-chan struct{}) {}); err == nil {
		t.Errorf("duplicate Start should error")
	}

	if err := p.Stop(0x10); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	<-done // task goroutine observed the stop signal
	if p.Has(0x10) {
		t.Errorf("Has(0x10) should be false after Stop")
	}
	if err := p.Stop(0x10); err == nil {
		t.Errorf("Stop of missing key should error")
	}
}

func TestPoolListSortedAndStopAll(t *testing.T) {
	p := New()
	for _, k := range []uintptr{0x30, 0x10, 0x20} {
		if err := p.Start(k, func(stop <-chan struct{}) { <-stop }); err != nil {
			t.Fatalf("Start(0x%x): %v", k, err)
		}
	}
	got := p.List()
	want := []uintptr{0x10, 0x20, 0x30}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List = %v, want %v", got, want)
		}
	}
	p.StopAll()
	if len(p.List()) != 0 {
		t.Errorf("StopAll should clear all tasks")
	}
}

func TestStopAllJoinsWorkersAndRejectsRegistration(t *testing.T) {
	p := New()
	started, release := make(chan struct{}), make(chan struct{})
	if err := p.Start(1, func(stop <-chan struct{}) { close(started); <-stop; <-release }); err != nil {
		t.Fatal(err)
	}
	<-started
	// Wait until cancellation is observed without using a scheduling delay.
	p.mu.Lock()
	task := p.tasks[1]
	p.mu.Unlock()
	done := make(chan struct{})
	go func() { p.StopAll(); close(done) }()
	<-task.ctx.Done()
	if err := p.Start(2, func(stop <-chan struct{}) { <-stop }); err == nil {
		t.Error("registration during StopAll succeeded")
	}
	select {
	case <-done:
		t.Error("StopAll did not join worker")
	default:
	}
	close(release)
	<-done
	if len(p.List()) != 0 {
		t.Fatal("tasks remain after join")
	}
}
func TestPoolLimitsAndCancellation(t *testing.T) {
	p := New()
	defer p.StopAll()
	for i := range MaxTasks {
		if err := p.Start(uintptr(i), func(stop <-chan struct{}) { <-stop }); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.Start(MaxTasks, func(<-chan struct{}) {}); err == nil {
		t.Fatal("task limit exceeded")
	}
}
