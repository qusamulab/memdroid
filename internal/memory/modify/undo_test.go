package modify

import (
	"errors"
	"testing"

	"memdroid/internal/driver/drivertest"
)

func TestUndoFailureRetainsHistoryForRetry(t *testing.T) {
	d := drivertest.New(drivertest.Region{Start: 0x1000, Data: []byte{1}})
	u := NewUndoStack()
	if err := u.WithUndo(d, 1, 0x1000, []byte{2}); err != nil {
		t.Fatal(err)
	}
	d.PokeErr = errors.New("temporary failure")
	if err := u.Undo(); err == nil || u.Depth() != 1 {
		t.Fatal("failed undo lost history")
	}
	d.PokeErr = nil
	if err := u.Undo(); err != nil || u.Depth() != 0 {
		t.Fatalf("retry=%v depth=%d", err, u.Depth())
	}
	if got := d.Bytes(0x1000, 1)[0]; got != 1 {
		t.Fatalf("restored %d", got)
	}
}
