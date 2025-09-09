package windowsspecial

import (
	"time"
	"unsafe"

	"syscall"

	"golang.org/x/sys/windows"
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

type WindowsOSHelper struct {
	input    *WindowsInputProcessor
	streamer *WindowsMediaStreamer
}

func (helper *WindowsOSHelper) Init() {
	helper.input = input
	helper.streamer = streamer
	go helper.MonitorDesktops()

}

func (helper *WindowsOSHelper) Close() error {
	helper.input.Close()
	helper.streamer.Cancel()
	return nil
}

func (helper *WindowsOSHelper) RestartInteractiveServices() {
	// we should only need to check input
	if !helper.input.IsStarted {
		return
	}
	helper.input.Close()
	helper.streamer.Cancel()
	helper.input.Start()
	helper.streamer.Start()

}

// monitorDesktops loops every 50ms and prints the active desktop name when it changes
func (helper *WindowsOSHelper) MonitorDesktops() {
	var lastDesktop string

	for {
		desktop, err := getActiveDesktop()
		if err == nil && desktop != lastDesktop {
			helper.RestartInteractiveServices()
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
	wsName := windows.UTF16PtrToString((*uint16)(unsafe.Pointer(pName)))

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
