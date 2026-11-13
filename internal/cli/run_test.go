package cli

import (
	"testing"

	"memdroid/internal/app"
	"memdroid/internal/driver/drivertest"
)

func TestShutdownResumesEveryAttachedProcess(t *testing.T) {
	d := drivertest.New()
	s := app.NewState(d)
	for _, pid := range []int{1, 2} {
		if err := s.Attach(pid, "app"); err != nil {
			t.Fatal(err)
		}
	}
	shutdown(s)
	for _, pid := range []int{1, 2} {
		if d.Attached(pid) {
			t.Errorf("PID %d was left stopped", pid)
		}
	}
}
