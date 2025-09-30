//go:build windows
// +build windows

package windowsspecial

import (
	"fmt"
	"log"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type DesktopRunner struct {
	procHandles []windows.Handle
}

// enablePrivileges enables the required SeTcbPrivilege and SeAssignPrimaryTokenPrivilege
// for the current process. This is mandatory for the token manipulation to work.
func (runner *DesktopRunner) enablePrivileges() error {
	var hToken windows.Token
	// Get the current process token.
	processHandle := windows.CurrentProcess()
	err := windows.OpenProcessToken(processHandle, windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &hToken)
	if err != nil {
		return fmt.Errorf("OpenProcessToken failed: %w", err)
	}
	defer hToken.Close()

	// Prepare the token privileges structure to enable two privileges.
	// ?? why is the privileges array size statically set to 1
	// make a new struct for 2?
	type tokenPrivilegesWith2 struct {
		PrivilegeCount uint32
		Privileges     [2]windows.LUIDAndAttributes // Array with space for 2
	} // this does not have the AllPrivilleges Helper
	tp := tokenPrivilegesWith2{
		PrivilegeCount: 2,
	}

	// Look up the LUID for SeTcbPrivilege ("Act as part of the TCB").
	var tcbLuid windows.LUID
	err = windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeTcbPrivilege"), &tcbLuid)
	if err != nil {
		return fmt.Errorf("LookupPrivilegeValue(SE_TCB_NAME) failed: %w", err)
	}
	tp.Privileges[0].Luid = tcbLuid
	tp.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED

	// Look up the LUID for SeAssignPrimaryTokenPrivilege ("Assign primary token").
	var assignLuid windows.LUID
	err = windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeAssignPrimaryTokenPrivilege"), &assignLuid)
	if err != nil {
		return fmt.Errorf("LookupPrivilegeValue(SE_ASSIGNPRIMARYTOKEN_NAME) failed: %w", err)
	}
	tp.Privileges[1].Luid = assignLuid
	tp.Privileges[1].Attributes = windows.SE_PRIVILEGE_ENABLED

	// Adjust the token privileges.
	// unsafe pointer since our struct is custom, praying this works
	// go gc gods have mercy
	err = windows.AdjustTokenPrivileges(hToken, false, (*windows.Tokenprivileges)(unsafe.Pointer(&tp)), 0, nil, nil)
	if err != nil {
		return fmt.Errorf("AdjustTokenPrivileges failed: %w", err)
	}
	// AdjustTokenPrivileges can "succeed" even if it does nothing.
	// We must check GetLastError to be sure the privileges were actually changed.
	if windows.GetLastError() == windows.ERROR_NOT_ALL_ASSIGNED {
		return fmt.Errorf("the token does not have one of the required privileges")
	}

	return nil
}

func (runner *DesktopRunner) RunProcesses(processes []string) error {
	if runner.procHandles != nil {
		return fmt.Errorf("processes were already started, please tear down first")
	}
	// This executable must be running as SYSTEM for this code to work.
	if err := runner.enablePrivileges(); err != nil {
		return fmt.Errorf("failed to enable necessary privileges: %v", err)
	}

	// 1. Get the active console session ID.
	sessionID := windows.WTSGetActiveConsoleSessionId()
	if sessionID == 0xFFFFFFFF {
		log.Println("No active console session found.")
		return fmt.Errorf("could not find desktop")
	}

	// Get the active desktop.
	var desktopName string
	// the api doesn't exist in the windows package or syscall, thank you microsoft for being utterly useless
	hWinSta, err := openWindowStation("WinSta0", false, WINSTA_READATTRIBUTES)
	//hWinSta, err := windows.OpenWindowStation("WinSta0", false, windows.WINSTA_READATTRIBUTES)
	if err != nil {
		return fmt.Errorf("could not find window station %v", err)
	}

	// todo make helper function to make sure this doesn't blow up
	defer procCloseWindowStation.Call(uintptr(hWinSta))

	// Temporarily set our process's window station to the interactive one.
	setProcessWindowStation(hWinSta)
	hDesktop, err := openInputDesktop(0, false, DESKTOP_READOBJECTS)
	if err == nil {
		// again make helper function to make sure this doesn't blow up
		defer procCloseDesktop.Call(uintptr(hDesktop))

		// Get the desktop's name.
		desktopName, err = getUserObjectInformation(hDesktop, UOI_NAME)
		if err != nil {
			return fmt.Errorf("could not get desktop name %v", err)
		}
	}

	if desktopName == "" {
		return fmt.Errorf("failed to get active desktop name. Last error: %v", windows.GetLastError())
	}

	log.Printf("Detected Active Desktop: %s", desktopName)

	// get token
	var token windows.Token
	//useMaster := (strings.Contains(desktopName, "Default"))
	//if useMaster {
	// --- SECURE MODE ---
	log.Println("Secure Mode detected. Launching with Master Key.")

	// Get the current SYSTEM process token.
	var hSystemToken windows.Token
	processHandle := windows.CurrentProcess()
	err = windows.OpenProcessToken(processHandle, windows.TOKEN_DUPLICATE, &hSystemToken)
	if err != nil {
		return fmt.Errorf("OpenProcessToken (SYSTEM) failed: %v", err)
	}
	defer hSystemToken.Close()

	// Duplicate it to create a new primary token.
	err = windows.DuplicateTokenEx(
		hSystemToken,
		windows.TOKEN_ALL_ACCESS,
		nil,
		windows.SecurityImpersonation,
		windows.TokenPrimary,
		&token,
	)
	if err != nil {
		return fmt.Errorf("DuplicateTokenEx failed: %v", err)
	}
	defer token.Close()

	// Re-parent the new token to the active user's session.
	err = windows.SetTokenInformation(token, windows.TokenSessionId, (*byte)(unsafe.Pointer(&sessionID)), uint32(unsafe.Sizeof(sessionID)))
	if err != nil {
		return fmt.Errorf("SetTokenInformation failed: %v", err)
	}

	//} else {
	//// --- NORMAL MODE ---
	//log.Println("Normal Mode detected. Launching with User Token.")

	//// Get the token of the user in the active session.
	//err := windows.WTSQueryUserToken(sessionID, &token)
	//if err != nil {
	//return fmt.Errorf("WTSQueryUserToken failed: %v", err)
	//}
	//defer token.Close()
	//}

	// start processes
	for _, proc := range processes {
		cmdLine, err := syscall.UTF16FromString(proc)
		if err != nil {
			log.Printf("Skipping invalid command %q: %v", proc, err)
			continue
		}

		si := &windows.StartupInfo{
			Cb: uint32(unsafe.Sizeof(windows.StartupInfo{})),
		}
		si.Desktop, _ = syscall.UTF16PtrFromString("winsta0\\default")
		//if useMaster {
		si.Desktop, _ = syscall.UTF16PtrFromString(desktopName)
		//}

		var pi windows.ProcessInformation
		err = windows.CreateProcessAsUser(
			token,
			nil,
			&cmdLine[0],
			nil,
			nil,
			false,
			0,
			nil,
			nil,
			si,
			&pi,
		)
		if err != nil {
			log.Printf("CreateProcessAsUser failed for %q: %v", proc, err)
			continue
		}

		log.Printf("Launched %q with PID %d", proc, pi.ProcessId)
		runner.procHandles = append(runner.procHandles, pi.Process)

		// Clean up handles
		windows.CloseHandle(pi.Thread)
	}

	return nil
}

func (runner *DesktopRunner) StopProcesses() {
	for _, h := range runner.procHandles {
		err := windows.TerminateProcess(h, 1) // exit code 1
		if err != nil {
			log.Printf("Could not close process, assuming all is good %v", err)
		}
		windows.CloseHandle(h) // cleanup after stopping
	}
	runner.procHandles = nil
}
