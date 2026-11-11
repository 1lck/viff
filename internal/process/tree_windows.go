//go:build windows

package process

import (
	"fmt"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RunTree runs a context-bound command and owns its descendants as well.
// Git launches git-lfs as a child, so killing only Git leaves the scan alive.
func RunTree(cmd *exec.Cmd) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create process job: %w", err)
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return fmt.Errorf("configure process job: %w", err)
	}

	HideConsoleWindow(cmd)
	// Start suspended so no child can escape before the job assignment.
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	ready := make(chan struct{})
	attached := false
	if cmd.Cancel != nil {
		cmd.Cancel = func() error {
			<-ready
			if attached {
				return windows.TerminateJobObject(job, 1)
			}
			return cmd.Process.Kill()
		}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	pid := uint32(cmd.Process.Pid)
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err == nil {
		err = windows.AssignProcessToJobObject(job, proc)
		windows.CloseHandle(proc)
	}
	if err == nil {
		attached = true
		err = resumeProcessThread(pid)
	}
	close(ready)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("start process in job: %w", err)
	}
	return cmd.Wait()
}

func resumeProcessThread(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err := windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(thread)
		_, err = windows.ResumeThread(thread)
		return err
	}
	return fmt.Errorf("find suspended thread: %w", windows.ERROR_NOT_FOUND)
}
