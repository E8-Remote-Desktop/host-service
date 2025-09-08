package windowsspecial

import (
	"fmt"
	"time"
	"unsafe"

	"syscall"
)

var (
	wtsapi32                         = syscall.NewLazyDLL("wtsapi32.dll")
	procWTSGetActiveConsoleSessionId = wtsapi32.NewProc("WTSGetActiveConsoleSessionId")
	procWTSQuerySessionInformationW  = wtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSFreeMemory                = wtsapi32.NewProc("WTSFreeMemory")
)

const (
	WTSWinStationName = 7
)

// monitorDesktops loops every 50ms and prints the active desktop name when it changes
func MonitorDesktops() {
	var lastDesktop string

	for {
		desktop, err := getActiveDesktop()
		if err == nil && desktop != lastDesktop {
			fmt.Println("Desktop changed:", desktop)
			lastDesktop = desktop
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// getActiveDesktop returns the current interactive desktop name
func getActiveDesktop() (string, error) {
	// Save current process window station
	origWS, err := getProcessWindowStation()
	if err != nil {
		return "", err
	}

	// Get active console session ID
	ret, _, _ := procWTSGetActiveConsoleSessionId.Call()
	sessionID := uint32(ret)

	// Query WinStation name
	var pName uintptr
	var bytesReturned uint32
	ret2, _, err := procWTSQuerySessionInformationW.Call(
		uintptr(0),
		uintptr(sessionID),
		uintptr(WTSWinStationName),
		uintptr(unsafe.Pointer(&pName)),
		uintptr(unsafe.Pointer(&bytesReturned)),
	)
	if ret2 == 0 {
		return "", err
	}
	defer procWTSFreeMemory.Call(pName)
	// go gc have mercy plz this should work
	wsName := (*uint16)(unsafe.Pointer(pName)) // ignore

	// Open interactive window station
	hWS, err := openWindowStation(wsName, false, WINSTA_READATTRIBUTES)
	if err != nil {
		return "", err
	}
	defer CloseWindowStation(hWS)

	// Switch process to interactive window station
	if err := setProcessWindowStation(hWS); err != nil {
		return "", err
	}
	defer setProcessWindowStation(origWS)

	// Open input desktop
	hDesk, err := openInputDesktop(0, false, DESKTOP_READOBJECTS)
	if err != nil {
		return "", err
	}
	defer CloseDesktop(hDesk)

	// Get desktop name
	name, err := getUserObjectInformation(hDesk, UOI_NAME)
	if err != nil {
		return "", err
	}

	return name, nil
}

// helper to get current process window station
func getProcessWindowStation() (syscall.Handle, error) {
	ret, _, err := user32.NewProc("GetProcessWindowStation").Call()
	if ret == 0 {
		return 0, err
	}
	return syscall.Handle(ret), nil
}

// helper to close desktop
func CloseDesktop(h syscall.Handle) {
	procCloseDesktop.Call(uintptr(h))
}

// helper to close window station
func CloseWindowStation(h syscall.Handle) {
	procCloseWindowStation.Call(uintptr(h))
}
