package windowsspecial

import (
	"fmt"
	"log"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// enablePrivileges enables the required SeTcbPrivilege and SeAssignPrimaryTokenPrivilege
// for the current process. This is mandatory for the token manipulation to work.
func enablePrivileges() error {
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

func main() {
	// This executable must be running as SYSTEM for this code to work.
	if err := enablePrivileges(); err != nil {
		log.Fatalf("Failed to enable necessary privileges: %v", err)
	}

	// The path to the helper application we want to launch.
	cmd, err := syscall.UTF16FromString("C:\\Windows\\System32\\notepad.exe")
	if err != nil {
		log.Fatalf("UTF16FromString failed: %v", err)
	}

	// --- The logic inside your event handler would start here ---

	// 1. Get the active console session ID.
	sessionID := windows.WTSGetActiveConsoleSessionId()
	if sessionID == 0xFFFFFFFF {
		log.Println("No active console session found.")
		return
	}

	// 2. Get the name of the active desktop.
	var desktopName string
	// the api doesn't exist in the windows package or syscall, thank you microsoft for being utterly useless
	hWinSta, err := openWindowStation(windows.StringToUTF16Ptr("WinSta0"), false, WINSTA_READATTRIBUTES)
	//hWinSta, err := windows.OpenWindowStation("WinSta0", false, windows.WINSTA_READATTRIBUTES)
	if err == nil {
		// todo make helper function to make sure this doesn't blow up
		defer procCloseWindowStation.Call(uintptr(hWinSta))

		// Temporarily set our process's window station to the interactive one.
		setProcessWindowStation(hWinSta)
		hDesktop, err := openInputDesktop(0, false, DESKTOP_READOBJECTS)
		if err == nil {
			// again make helper function to make sure this doesn't blow up
			defer procCloseDesktop.Call(uintptr(hDesktop))

			// Get the desktop's name.
			nameBuffer := make([]uint16, 256)
			desktopName, err = getUserObjectInformation(hDesktop, UOI_NAME)
			if err == nil {
				desktopName = windows.UTF16ToString(nameBuffer)
			}
		}
	}

	if desktopName == "" {
		log.Fatalf("Failed to get active desktop name. Last error: %v", windows.GetLastError())
	}

	log.Printf("Detected Active Desktop: %s", desktopName)

	var pi windows.ProcessInformation
	si := &windows.StartupInfo{
		Cb: uint32(unsafe.Sizeof(windows.StartupInfo{})),
	}

	// 3. The Decision: Choose the right path based on the desktop name.
	if desktopName != "Default" {
		// --- SECURE MODE ---
		log.Println("Secure Mode detected. Launching with Master Key.")

		// Get the current SYSTEM process token.
		var hSystemToken windows.Token
		processHandle := windows.CurrentProcess()
		err := windows.OpenProcessToken(processHandle, windows.TOKEN_DUPLICATE, &hSystemToken)
		if err != nil {
			log.Fatalf("OpenProcessToken (SYSTEM) failed: %v", err)
		}
		defer hSystemToken.Close()

		// Duplicate it to create a new primary token.
		var hMasterToken windows.Token
		err = windows.DuplicateTokenEx(hSystemToken, windows.TOKEN_ALL_ACCESS, nil, windows.SecurityIdentification, windows.TokenPrimary, &hMasterToken)
		if err != nil {
			log.Fatalf("DuplicateTokenEx failed: %v", err)
		}
		defer hMasterToken.Close()

		// Re-parent the new token to the active user's session.
		err = windows.SetTokenInformation(hMasterToken, windows.TokenSessionId, (*byte)(unsafe.Pointer(&sessionID)), uint32(unsafe.Sizeof(sessionID)))
		if err != nil {
			log.Fatalf("SetTokenInformation failed: %v", err)
		}

		// Set the target desktop in the startup info.
		si.Desktop, err = syscall.UTF16PtrFromString(desktopName)
		if err != nil {
			log.Fatalf("UTF16PtrFromString for Desktop failed: %v", err)
		}

		err = windows.CreateProcessAsUser(hMasterToken, nil, &cmd[0], nil, nil, false, 0, nil, nil, si, &pi)
		if err != nil {
			log.Fatalf("CreateProcessAsUser (Secure) failed: %v", err)
		}

	} else {
		// --- NORMAL MODE ---
		log.Println("Normal Mode detected. Launching with User Token.")

		// Get the token of the user in the active session.
		var hUserToken windows.Token
		err := windows.WTSQueryUserToken(sessionID, &hUserToken)
		if err != nil {
			log.Fatalf("WTSQueryUserToken failed: %v", err)
		}
		defer hUserToken.Close()

		// Explicitly set the desktop for clarity.
		si.Desktop, err = syscall.UTF16PtrFromString("winsta0\\default")
		if err != nil {
			log.Fatalf("UTF16PtrFromString for Desktop failed: %v", err)
		}

		err = windows.CreateProcessAsUser(hUserToken, nil, &cmd[0], nil, nil, false, 0, nil, nil, si, &pi)
		if err != nil {
			log.Fatalf("CreateProcessAsUser (Normal) failed: %v", err)
		}
	}

	if pi.Process != 0 {
		log.Printf("Process launched successfully with PID: %d", pi.ProcessId)
		// Clean up the process and thread handles from PROCESS_INFORMATION.
		defer windows.CloseHandle(pi.Process)
		defer windows.CloseHandle(pi.Thread)
	}
}
