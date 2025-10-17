//go:build windows
// +build windows

package windowsspecial

import (
	"fmt"
	"log"
	"syscall"
	"unsafe"

	"github.com/vito/houdini/win32"
	"github.com/winlabs/gowin32/wrappers"
	"golang.org/x/sys/windows"
)

type DesktopRunner struct {
	procHandles []windows.Handle
}

// enablePrivileges enables the required SeTcbPrivilege and SeAssignPrimaryTokenPrivilege
// for the current process. This is mandatory for the token manipulation to work.
func (runner *DesktopRunner) enablePrivileges(hToken windows.Token, superpower bool) error {
	// defer hToken.Close(0)
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
	var err error
	if superpower {
		err = windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeTcbPrivilege"), &tcbLuid)
	} else {
		err = windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeIncreaseQuotaPrivilege"), &tcbLuid)
	}
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
func (runner *DesktopRunner) Init() error {
	// Enable the required permissions
	// This executable must be running as SYSTEM for this code to work.
	processHandle := windows.CurrentProcess()
	var systemToken windows.Token
	if err := windows.OpenProcessToken(processHandle, windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &systemToken); err != nil {
		return fmt.Errorf("could not open process token %v", err)
	}
	if err := runner.enablePrivileges(systemToken, true); err != nil {
		return fmt.Errorf("failed to enable necessary privileges: %v", err)
	}

	return nil
}

// Function consuming this token now owns said token and has to close it
func (runner *DesktopRunner) getActiveUserToken() windows.Token {
	forceSystem := false
	sessionID := windows.WTSGetActiveConsoleSessionId()
	if sessionID == 0xFFFFFFFF {
		log.Println("ERROR: No active console session found.")
	}
	var token windows.Token
	//var blankToken windows.Token
	err := windows.WTSQueryUserToken(sessionID, &token)
	if err != nil {
		log.Printf("ERROR: Could not query user token, %v", err)
	}
	user, err := token.GetTokenUser()
	if err != nil {
		log.Printf("WARNING: Could not get user attached to token assuming system mode?: %v", err)
		forceSystem = true
	}
	if !forceSystem {
		sid := user.User.Sid
		_, _, _, err := sid.LookupAccount(sid.String())
		if err != nil {
			log.Printf("WARNING: Could not find account attached to SID: %v", err)
		}
		return token

	}
	log.Printf("Detected SYSTEM running Terminal Service")
	var hSystemToken windows.Token
	processHandle := windows.CurrentProcess()
	err = windows.OpenProcessToken(processHandle, windows.TOKEN_DUPLICATE, &hSystemToken)
	if err != nil {
		log.Printf("OpenProcessToken (SYSTEM) failed: %v", err)
		return token
	}
	defer hSystemToken.Close()
	log.Printf("DEBUG: Opened Process Token, about to duplicate")
	// TODO refactor this to not potentially leak a token (maybe close the token here if possible)

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
		log.Printf("DuplicateTokenEx failed: %v", err)
		return token
	}
	//defer token.Close() // don't close so we can use it for later

	// Re-parent the new token to the active user's session.
	err = windows.SetTokenInformation(token, windows.TokenSessionId, (*byte)(unsafe.Pointer(&sessionID)), uint32(unsafe.Sizeof(sessionID)))
	if err != nil {
		log.Printf("Could not grant permissions to system token")
	}
	log.Printf("DEBUG: returning active SYSTEM token")
	return token
}

func (runner *DesktopRunner) getCurrentIOSID() (string, error) {
	sessionID := windows.WTSGetActiveConsoleSessionId()
	if sessionID == 0xFFFFFFFF {
		log.Println("No active console session found.")
		return "", fmt.Errorf("could not find active session")
	}

	var token windows.Token
	err := windows.WTSQueryUserToken(sessionID, &token)
	if err != nil {
		log.Printf("Could not SID associated with token for desktop, returning SYSTEM sid: %v", err)
		return "S-1-5-18", nil
	}
	user, err := token.GetTokenUser()
	if err != nil {
		log.Printf("Could not get user attached to token please contact support: %v", err)
		return "", err
	}
	sid := user.User.Sid
	return sid.String(), nil
}

// Get a regular user token, remember you own this token from now on and are responsible for closing it

func (runner *DesktopRunner) ResetPermissions() error {
	if err := windows.RevertToSelf(); err != nil {
		return fmt.Errorf("could not revert to system permissions %v", err)
	}
	processHandle := windows.CurrentProcess()
	var systemToken windows.Token
	if err := windows.OpenProcessToken(processHandle, windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &systemToken); err != nil {
		return fmt.Errorf("could not open token %v", err)
	}
	if err := runner.enablePrivileges(systemToken, true); err != nil {

		return fmt.Errorf("could not re-enable desktop permissions %v", err)
	}
	return nil

}

func (runner *DesktopRunner) ImpersonateRunningUser(fToken windows.Token) error {
	//var dupToken windows.Token
	//hToken := runner.getActiveUserToken()
	//err := windows.DuplicateTokenEx(
	//hToken,
	//windows.TOKEN_ALL_ACCESS,
	//nil,
	//windows.SecurityImpersonation,
	//windows.TokenImpersonation,
	//&dupToken,
	//)
	//defer dupToken.Close()
	//if err != nil {
	//return fmt.Errorf("could not duplicate token %v", err)
	//}
	//if err := runner.enablePrivileges(dupToken, false); err != nil {
	//return fmt.Errorf("could not enable new privlleges on impersonated token")
	//}
	//impersonateActiveUser(dupToken)
	impersonateActiveUser(fToken)

	log.Printf("DEBUG: Impersonate User Called (this cannot return an error so assume sucess)")
	return nil
}

func (runner *DesktopRunner) GetActiveDesktop(hToken windows.Token, login bool) (string, error) {

	log.Printf("DEBUG: Searching for active desktop")
	//oldHWinSta, err := win32.GetProcessWindowStation()
	//if err != nil {
	//return "", fmt.Errorf("could not store old WindowsStation, %v", err)
	//}
	//log.Printf("DEBUG: Got Old Window Station")
	//defer win32.SetProcessWindowStation(oldHWinSta)
	//log.Printf("DEBUG: Deferred Setback")
	hWinSta, err := openWindowStation("WinSta0", false, WINSTA_READATTRIBUTES|WINSTA_ENUMDESKTOPS|WINSTA_READSCREEN|WINSTA_ENUMERATE|WINSTA_WRITE_ATTRIBUTES|WINSTA_ACCESSCLIPBOARD|WINSTA_ACCESSGLOBALATOMS|WINSTA_CREATEDESKTOP|WINSTA_EXITWINDOWS)
	// TODO make helper function for this call
	defer procCloseWindowStation.Call(uintptr(hWinSta))
	if err != nil {
		return "", fmt.Errorf("could not find window station %v", err)
	}
	log.Printf("DEBUG: Window Station Opened")
	// Temporarily set our process's window station to the interactive one.

	if err := win32.SetProcessWindowStation(win32.Hwinsta(uintptr(hWinSta))); err != nil {
		return "", fmt.Errorf("could not assign window station to process, %v", err)
	}
	log.Printf("DEBUG: Window Station Set")

	var desktopName string
	hDesktop, err := wrappers.OpenInputDesktop(0, false,
		DESKTOP_READOBJECTS)
	if err != nil {
		log.Printf("ERROR: Could not open desktop %v", err)
		return "", fmt.Errorf("could not open desktop because of %v", err)
	}
	//if err := win32.SetThreadDesktop(win32.Hdesk(hDesktop)); err != nil {
	//log.Printf("ERROR: Could not set thread  to running desktop")
	//return "", fmt.Errorf("could not set thread to input desktop")
	//}
	// again make helper function to make sure this doesn't blow up
	defer procCloseDesktop.Call(uintptr(hDesktop))

	// Get the desktop's name.
	var desktopNameLength uint32
	if err := wrappers.GetUserObjectInformation(hDesktop, UOI_NAME, uintptr(unsafe.Pointer(nil)), 0, &desktopNameLength); err != nil {
		//return "", fmt.Errorf("could not get desktop name length, %v", err)
		// this okay because it always says it could not get the length when it does
		log.Printf("DEBUG: Errors from desktop Length %v", err)
	}
	log.Printf("DEBUG: Desktop Name Length: %v", desktopNameLength)
	if desktopNameLength == 0 {
		return "", fmt.Errorf("desktop not ready yet, length too short ")
	}

	desktopNameUTF16 := make([]uint16, 256)
	var blankuint32 uint32

	err = wrappers.GetUserObjectInformation(hDesktop, UOI_NAME, uintptr(unsafe.Pointer(&desktopNameUTF16[0])), desktopNameLength, &blankuint32)

	if err != nil {
		return "", fmt.Errorf("could not get desktop name %v", err)
	}
	desktopName = windows.UTF16ToString(desktopNameUTF16)
	log.Printf("DEBUG: Found Desktop, %s", desktopName)

	if desktopName == "" {
		return "", fmt.Errorf("failed to get active desktop name. Last error: %v", windows.GetLastError())
	}

	log.Printf("Detected Active Desktop: %s", desktopName)
	return desktopName, nil

}

// Run Process does not own the token it takes, it is up to the parent caller to close the token
func (runner *DesktopRunner) RunProcesses(processes []string, token windows.Token) error {
	log.Printf("DEBUG: Running processes for stream and input")

	for _, proc := range processes {
		cmdLine, err := syscall.UTF16FromString(proc)
		if err != nil {
			log.Printf("Skipping invalid command %q: %v", proc, err)
			continue
		}

		si := &windows.StartupInfo{
			Cb: uint32(unsafe.Sizeof(windows.StartupInfo{})),
		}
		//si.Desktop, _ = syscall.UTF16PtrFromString(desktopName)
		//if !login {
		//si.Desktop, _ = syscall.UTF16PtrFromString("winsta0\\default")
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
			return fmt.Errorf("CreateProcessAsUser failed for %q: %v", proc, err)
		}

		//log.Printf("DEBUG: Launched %q with PID %d", proc, pi.ProcessId)
		//if runner.procHandles != nil {
		//for _, handle := range runner.procHandles {
		//windows.CloseHandle(handle)
		//}
		//}

		//var procHandles []windows.Handle
		//procHandles = append(procHandles, pi.Process)
		//runner.procHandles = procHandles

		// Clean up handles
		windows.CloseHandle(pi.Process)
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
