//go:build windows

package adapters

import (
	"fmt"
	"log/slog"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobObject wraps a Windows Job Object handle. A job object groups child
// processes so they can be terminated as a unit (replacing Unix process
// groups) and automatically cleaned up when the parent exits (replacing
// Pdeathsig).
type jobObject struct {
	handle windows.Handle
}

// newJobObject creates a job object configured with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE: when the last handle to the job is
// closed (including on parent crash), Windows terminates every process in
// the job. This is the Windows equivalent of Linux's Pdeathsig.
//
// Known limitations:
//   - Nested jobs: If fuse runs inside a CI runner's job object (Windows 8+),
//     child job creation works but AssignProcessToJobObject may fail with
//     ERROR_ACCESS_DENIED if the parent job has restrictive limits. This is
//     handled gracefully — the child runs without job tracking.
//   - UAC elevation: If a child triggers UAC elevation, the elevated process
//     runs at a higher integrity level and escapes job containment. This is
//     an inherent Windows security boundary, not a fuse limitation.
func newJobObject() (*jobObject, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateJobObject: %w", err)
	}

	// SECURITY: Only KILL_ON_JOB_CLOSE is set. Do NOT add
	// JOB_OBJECT_LIMIT_BREAKAWAY_OK — it would allow child processes to
	// escape the job via CREATE_BREAKAWAY_FROM_JOB, defeating containment.
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	// SAFETY: info is stack-allocated and does not escape; the pointer is
	// valid for the duration of the SetInformationJobObject syscall.
	if _, err := windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("SetInformationJobObject: %w", err)
	}

	return &jobObject{handle: h}, nil
}

// assign adds a running process to the job object. The process is identified
// by PID; we open a handle with the minimum required access rights.
func (j *jobObject) assign(pid int) error {
	proc, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(pid),
	)
	if err != nil {
		return fmt.Errorf("OpenProcess pid=%d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(proc) }()

	if err := windows.AssignProcessToJobObject(j.handle, proc); err != nil {
		return fmt.Errorf("AssignProcessToJobObject pid=%d: %w", pid, err)
	}
	return nil
}

// terminate kills every process in the job with the given exit code.
// This is the Windows equivalent of syscall.Kill(-pid, SIGKILL).
func (j *jobObject) terminate(exitCode uint32) error {
	if err := windows.TerminateJobObject(j.handle, exitCode); err != nil {
		return fmt.Errorf("TerminateJobObject: %w", err)
	}
	return nil
}

// close releases the job object handle. If JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
// is set and processes are still running, they are terminated.
func (j *jobObject) close() {
	if j.handle != 0 {
		if err := windows.CloseHandle(j.handle); err != nil {
			slog.Debug("close job object handle", "err", err)
		}
		j.handle = 0
	}
}

// ProbeJobObject is an opaque handle used by the doctor check to verify
// job object support without exposing the internal jobObject type.
type ProbeJobObject struct{ inner *jobObject }

// NewProbeJobObject creates a job object for diagnostic probing.
func NewProbeJobObject() (*ProbeJobObject, error) {
	j, err := newJobObject()
	if err != nil {
		return nil, err
	}
	return &ProbeJobObject{inner: j}, nil
}

// AssignProbeJobObject assigns a process to a probe job object.
func AssignProbeJobObject(p *ProbeJobObject, pid int) error {
	return p.inner.assign(pid)
}

// CloseProbeJobObject releases the probe job object handle.
func CloseProbeJobObject(p *ProbeJobObject) {
	p.inner.close()
}
