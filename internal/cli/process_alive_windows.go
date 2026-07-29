//go:build windows

package cli

import "golang.org/x/sys/windows"

// GetExitCodeProcess returns this Win32 value while a process has not exited.
const windowsStillActive = 259

func defaultProcessAlive(pid int) bool {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	var code uint32
	return windows.GetExitCodeProcess(process, &code) == nil && code == windowsStillActive
}
