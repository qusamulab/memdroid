package adb

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fakeADB(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "commands.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$MEMDROID_ADB_LOG\"\ncase \"$*\" in\n*devices*) printf 'List of devices attached\\ndevice-A\\tdevice\\n' ;;\n*) printf 'AQAAAA==\\n' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "adb"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MEMDROID_ADB_LOG", log)
	return log
}
func TestAttachedDeviceCannotBeRetargeted(t *testing.T) {
	log := fakeADB(t)
	d := New()
	// Attaching with one connected device pins its explicit serial.
	if err := d.Attach(41); err != nil {
		t.Fatal(err)
	}
	if d.DeviceSerial() != "device-A" {
		t.Fatal("implicit device was not pinned")
	}
	for _, action := range []func() error{
		func() error { return d.SelectDevice("device-B") },
		func() error { return d.ConnectWifi("device-B:5555") },
		func() error { return d.DisconnectWifi("device-A") },
	} {
		if err := action(); err == nil {
			t.Fatal("active device change succeeded")
		}
	}
	pinned := d.WithContext(context.Background())
	if err := d.Detach(41); err != nil {
		t.Fatal(err)
	}
	if err := d.SelectDevice("device-B"); err != nil {
		t.Fatal(err)
	}
	if err := pinned.Poke(41, 0x1000, []byte{2}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "-s device-B") {
		t.Fatalf("pinned I/O changed device: %s", data)
	}
	if !strings.Contains(string(data), "-s device-A shell su -c echo") {
		t.Fatal("missing pinned write")
	}
}
func TestSignalRejectsInvalidPIDsBeforeExec(t *testing.T) {
	log := fakeADB(t)
	d := New()
	for _, pid := range []int{0, -1} {
		for _, action := range []func(int) error{d.Attach, d.Detach, d.Stop, d.Continue} {
			if err := action(pid); err == nil {
				t.Errorf("signal for PID %d succeeded", pid)
			}
		}
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("invalid PID reached adb")
	}
}
func TestCancelledOperationDoesNotStartADB(t *testing.T) {
	log := fakeADB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New().WithContext(ctx).Peek(1, 0x1000, 4)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Peek=%v", err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("cancelled operation reached adb")
	}
}

func TestProcessIdentityIncludesBootAndStartTime(t *testing.T) {
	fakeADB(t)
	dir := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	script := "#!/bin/sh\nprintf 'boot-id\\n41 (process name) S " + strings.Repeat("0 ", 18) + "12345 0\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "adb"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	d := New()
	if err := d.SelectDevice("device-A"); err != nil {
		t.Fatal(err)
	}
	identity, err := d.ProcessIdentity(41)
	if err != nil || identity != "device-A:boot-id:41:12345" {
		t.Fatalf("identity=%s err=%v", identity, err)
	}
}

func TestInFlightADBStopsOnCancellation(t *testing.T) {
	log := fakeADB(t)
	script := "#!/bin/sh\nprintf ready > \"$MEMDROID_ADB_LOG\"\nexec sleep 60\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(log), "adb"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := New().WithContext(ctx).Peek(1, 0x1000, 4); done <- err }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(log); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("ADB did not start")
		case <-ticker.C:
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Peek=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not terminate ADB")
	}
}
