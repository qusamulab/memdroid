package adb

import (
	"fmt"
	"strconv"
	"strings"

	"memdroid/internal/driver"
)

// ListProcesses returns all running processes via "adb shell ps -A".
func (a *ADB) ListProcesses() ([]driver.ProcessInfo, error) {
	out, err := a.shell("ps", "-A")
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	var procs []driver.ProcessInfo
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// ps -A columns: USER PID PPID VSZ RSS WCHAN ADDR S NAME
		if len(fields) < 9 {
			continue
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		name := fields[len(fields)-1]
		procs = append(procs, driver.ProcessInfo{PID: pid, Name: name})
	}
	return procs, nil
}

// FindProcessByName returns the PIDs of all processes whose name contains substr.
func (a *ADB) FindProcessByName(substr string) ([]driver.ProcessInfo, error) {
	all, err := a.ListProcesses()
	if err != nil {
		return nil, err
	}
	var out []driver.ProcessInfo
	lower := strings.ToLower(substr)
	for _, p := range all {
		if strings.Contains(strings.ToLower(p.Name), lower) {
			out = append(out, p)
		}
	}
	return out, nil
}

// signal sends a POSIX signal to pid via "kill -<sig> <pid>" as root.
func (a *ADB) signal(pid int, sig string) error {
	if pid <= 0 {
		return fmt.Errorf("pid must be positive")
	}
	_, err := a.shellRoot(fmt.Sprintf("kill -%s %d", sig, pid))
	return err
}

// Attach sends SIGSTOP to the process to pause it before memory operations.
// On Android we don't use ptrace; instead we stop the process with kill -STOP
// which requires root.
func (a *ADB) Attach(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("pid must be positive")
	}
	a.deviceMu.Lock()
	defer a.deviceMu.Unlock()
	if a.fixedSerial != nil {
		return fmt.Errorf("cannot attach through an operation binding")
	}
	if a.DeviceSerial() == "" {
		devices, err := a.ListDevices()
		if err != nil {
			return err
		}
		if len(devices) != 1 {
			return fmt.Errorf("select exactly one device before attaching")
		}
		a.mu.Lock()
		a.serial = devices[0]
		a.mu.Unlock()
	}

	if err := a.signal(pid, "STOP"); err != nil {
		return fmt.Errorf("attach (SIGSTOP) pid %d: %w", pid, err)
	}
	if a.attached == nil {
		a.attached = make(map[int]struct{})
	}
	a.attached[pid] = struct{}{}
	return nil
}

// Detach resumes the process with SIGCONT.
func (a *ADB) Detach(pid int) error {
	a.deviceMu.Lock()
	defer a.deviceMu.Unlock()
	if err := a.signal(pid, "CONT"); err != nil {
		return fmt.Errorf("detach pid %d: %w", pid, err)
	}
	delete(a.attached, pid)
	return nil
}

// Stop sends SIGSTOP (same as Attach, exposed as an explicit control command).
func (a *ADB) Stop(pid int) error {
	if err := a.signal(pid, "STOP"); err != nil {
		return fmt.Errorf("stop pid %d: %w", pid, err)
	}
	return nil
}

// Continue sends SIGCONT to resume a stopped process.
func (a *ADB) Continue(pid int) error {
	if err := a.signal(pid, "CONT"); err != nil {
		return fmt.Errorf("continue pid %d: %w", pid, err)
	}
	return nil
}

// ProcessIdentity distinguishes PID reuse and device reboots.
func (a *ADB) ProcessIdentity(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("pid must be positive")
	}
	out, err := a.shellRoot(fmt.Sprintf("cat /proc/sys/kernel/random/boot_id; cat /proc/%d/stat", pid))
	if err != nil {
		return "", err
	}
	boot, stat, ok := strings.Cut(strings.TrimSpace(string(out)), "\n")
	end := strings.LastIndex(stat, ")")
	if !ok || end < 0 {
		return "", fmt.Errorf("invalid process identity for pid %d", pid)
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) < 20 {
		return "", fmt.Errorf("invalid process stat for pid %d", pid)
	}
	return fmt.Sprintf("%s:%s:%d:%s", a.DeviceSerial(), boot, pid, fields[19]), nil
}
