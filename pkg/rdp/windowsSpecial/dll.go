//go:build windows
// +build windows

package windowsspecial

import (
	"log"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                        = syscall.NewLazyDLL("user32.dll")
	procOpenWindowStationW        = user32.NewProc("OpenWindowStationW")
	procSetProcessWindowStation   = user32.NewProc("SetProcessWindowStation")
	procCloseWindowStation        = user32.NewProc("CloseWindowStation")
	procOpenInputDesktop          = user32.NewProc("OpenInputDesktop")
	procCloseDesktop              = user32.NewProc("CloseDesktop")
	procGetUserObjectInformationW = user32.NewProc("GetUserObjectInformationW")
)

const (
	// thank microsoft for not putting these in the go package, maybe I should've done this in C++
	// from	config.os, ok = ini.SectionGet("OS", "os")
	UOI_NAME            = 2 // string?
	WINSTA_ENUMDESKTOPS = 0x0001
	WINSTA_ENUMERATE    = 0x0100
	WINSTA_READSCREEN   = 0x0200
	DESKTOP_READOBJECTS = 0x0001

	// WINSTA_READATTRIBUTES is a combination of the above and standard rights.
	WINSTA_READATTRIBUTES = windows.STANDARD_RIGHTS_READ | WINSTA_ENUMDESKTOPS | WINSTA_ENUMERATE | WINSTA_READSCREEN
)

func openWindowStation(name string, inherit bool, desiredAccess uint32) (syscall.Handle, error) {
	var inheritInt uintptr
	if inherit {
		inheritInt = 1
	}
	wName, err := windows.UTF16PtrFromString(name)
	if err != nil {
		log.Printf("Could not openWindowStation name convert to string %v", err)
	}
	ret, _, err := procOpenWindowStationW.Call(uintptr(unsafe.Pointer(wName)), inheritInt, uintptr(desiredAccess))
	if ret == 0 {
		return 0, err
	}
	return syscall.Handle(ret), nil
}

func setProcessWindowStation(hWinSta syscall.Handle) error {
	ret, _, err := procSetProcessWindowStation.Call(uintptr(hWinSta))
	if ret == 0 {
		return err
	}
	return nil
}

func openInputDesktop(flags uint32, inherit bool, desiredAccess uint32) (syscall.Handle, error) {
	var inheritInt uintptr
	if inherit {
		inheritInt = 1
	}
	ret, _, err := procOpenInputDesktop.Call(uintptr(flags), inheritInt, uintptr(desiredAccess))
	if ret == 0 {
		return 0, err
	}
	return syscall.Handle(ret), nil
}

func getUserObjectInformation(hObject syscall.Handle, index uint32) (string, error) {
	nameBuffer := make([]uint16, 256)
	var needed uint32
	ret, _, err := procGetUserObjectInformationW.Call(
		uintptr(hObject),
		uintptr(index),
		uintptr(unsafe.Pointer(&nameBuffer[0])),
		uintptr(len(nameBuffer)*2),
		uintptr(unsafe.Pointer(&needed)),
	)
	if ret == 0 {
		return "", err
	}
	return windows.UTF16ToString(nameBuffer), nil
}

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
