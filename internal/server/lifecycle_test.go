package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"memdroid/internal/app"
	"memdroid/internal/driver"
	"memdroid/internal/driver/drivertest"
)

type blockedPatternDriver struct {
	*drivertest.Fake
	started, release chan struct{}
}

func (d *blockedPatternDriver) ReadRegion(pid int, addr uintptr, n int) ([]byte, error) {
	close(d.started)
	<-d.release
	return d.Fake.ReadRegion(pid, addr, n)
}
func TestPatternScanCannotCommitAcrossProcessSwitch(t *testing.T) {
	d := &blockedPatternDriver{Fake: drivertest.New(drivertest.Region{Start: 0x1000, Data: []byte{0xab, 0xcd}}), started: make(chan struct{}), release: make(chan struct{})}
	s := app.NewState(d)
	if err := s.Attach(1, "first"); err != nil {
		t.Fatal(err)
	}
	old := s.GetDriver()
	h := &handler{state: s}
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.searchPattern(rec, httptest.NewRequest("POST", "/api/search/pattern", strings.NewReader(`{"pattern":"ab cd"}`)))
		close(done)
	}()
	<-d.started
	switched := make(chan error, 1)
	go func() { switched <- s.Attach(2, "second") }()
	<-driver.Context(old).Done()
	close(d.release)
	if err := <-switched; err != nil {
		t.Fatal(err)
	}
	<-done
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if s.GetSession().PID() != 2 || s.GetSession().CandidateCount() != 0 {
		t.Fatal("old results entered new session")
	}
}
