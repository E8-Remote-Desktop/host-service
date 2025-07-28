//go:build windows
// +build windows

package rdp

import (
	"encoding/binary"
	"log"
	"syscall"
	"unsafe"

	"github.com/pion/webrtc/v3"
)

func GetInput() Input {
	return &RDPWindowsInput{}
}

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procSendInput        = user32.NewProc("SendInput")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procSetCursorPos     = user32.NewProc("SetCursorPos")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

const (
	INPUT_MOUSE    = 0
	INPUT_KEYBOARD = 1

	// Mouse flags
	MOUSEEVENTF_MOVE       = 0x0001
	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_LEFTUP     = 0x0004
	MOUSEEVENTF_RIGHTDOWN  = 0x0008
	MOUSEEVENTF_RIGHTUP    = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP   = 0x0040
	MOUSEEVENTF_WHEEL      = 0x0800
	MOUSEEVENTF_ABSOLUTE   = 0x8000

	// Keyboard flags
	KEYEVENTF_KEYUP = 0x0002

	// Virtual key codes
	VK_LCONTROL = 0xA2
	VK_LSHIFT   = 0xA0
	VK_LMENU    = 0xA4 // Left Alt
	VK_LWIN     = 0x5B // Left Windows key

	// System metrics
	SM_CXSCREEN = 0
	SM_CYSCREEN = 1

	WHEEL_DELTA = 120
)

type POINT struct {
	X, Y int32
}

type INPUT struct {
	Type uint32
	_    [4]byte // padding for alignment
	Data [24]byte
}

type MOUSEINPUT struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type KEYBDINPUT struct {
	Vk          uint16
	Scan        uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type RDPWindowsInput struct {
	lastButtons  byte
	screenWidth  int32
	screenHeight int32
}

func (input *RDPWindowsInput) Init() error {
	// Get screen dimensions for absolute positioning if needed
	input.screenWidth = int32(getSystemMetrics(SM_CXSCREEN))
	input.screenHeight = int32(getSystemMetrics(SM_CYSCREEN))

	log.Println("Windows input handler initialized")
	return nil
}

func (input *RDPWindowsInput) Close() {
	// No cleanup needed for Windows API
	log.Println("Windows input handler closed")
}

func (input *RDPWindowsInput) AcceptDataChannel(pc *webrtc.PeerConnection) {
	pc.OnDataChannel(input.processor)
}

func (input *RDPWindowsInput) processor(dc *webrtc.DataChannel) {
	log.Printf("Data Channel %s request received\n", dc.Label())

	dc.OnOpen(func() {
		log.Printf("Data Channel %s opened, Input Ready\n", dc.Label())
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		data := msg.Data
		if len(data) == 0 {
			return
		}

		switch data[0] {
		case 1: // Keyboard
			if len(data) < 4 {
				log.Println("Invalid keyboard packet")
				return
			}
			keyCode := binary.BigEndian.Uint16(data[1:3])
			modifiers := data[3]

			input.handleModifiers(modifiers, true)
			input.sendKeyboardInput(uint16(keyCode), false) // Key down
			input.sendKeyboardInput(uint16(keyCode), true)  // Key up
			input.handleModifiers(modifiers, false)

		case 2: // Mouse move + buttons
			if len(data) < 6 {
				log.Println("Invalid mouse move packet")
				return
			}
			buttons := data[1]
			dx := int16(binary.BigEndian.Uint16(data[2:4]))
			dy := int16(binary.BigEndian.Uint16(data[4:6]))

			input.sendMouseMove(int32(dx), int32(dy))
			input.handleMouseButtons(buttons)

		case 3: // Scroll
			if len(data) < 4 {
				log.Println("Invalid scroll packet")
				return
			}
			deltaY := int32(binary.BigEndian.Uint16(data[1:3]))

			if deltaY != 0 {
				input.sendMouseWheel(deltaY)
			}

		default:
			log.Printf("Unknown input event type: %d", data[0])
		}
	})
}

func (input *RDPWindowsInput) sendKeyboardInput(vkCode uint16, keyUp bool) {
	var keyInput INPUT
	keyInput.Type = INPUT_KEYBOARD

	kbd := (*KEYBDINPUT)(unsafe.Pointer(&keyInput.Data[0]))
	kbd.Vk = vkCode
	kbd.Scan = 0
	kbd.DwFlags = 0
	if keyUp {
		kbd.DwFlags = KEYEVENTF_KEYUP
	}
	kbd.Time = 0
	kbd.DwExtraInfo = 0

	sendInput(1, &keyInput)
}

func (input *RDPWindowsInput) sendMouseMove(dx, dy int32) {
	var mouseInput INPUT
	mouseInput.Type = INPUT_MOUSE

	mouse := (*MOUSEINPUT)(unsafe.Pointer(&mouseInput.Data[0]))
	mouse.Dx = dx
	mouse.Dy = dy
	mouse.MouseData = 0
	mouse.DwFlags = MOUSEEVENTF_MOVE
	mouse.Time = 0
	mouse.DwExtraInfo = 0

	sendInput(1, &mouseInput)
}

func (input *RDPWindowsInput) sendMouseWheel(delta int32) {
	var mouseInput INPUT
	mouseInput.Type = INPUT_MOUSE

	mouse := (*MOUSEINPUT)(unsafe.Pointer(&mouseInput.Data[0]))
	mouse.Dx = 0
	mouse.Dy = 0
	mouse.MouseData = uint32(delta * WHEEL_DELTA)
	mouse.DwFlags = MOUSEEVENTF_WHEEL
	mouse.Time = 0
	mouse.DwExtraInfo = 0

	sendInput(1, &mouseInput)
}

func (input *RDPWindowsInput) handleModifiers(modifiers byte, press bool) {
	modKeys := []struct {
		mask byte
		vk   uint16
	}{
		{1 << 0, VK_LCONTROL},
		{1 << 1, VK_LSHIFT},
		{1 << 2, VK_LMENU},
		{1 << 3, VK_LWIN},
	}

	for _, mod := range modKeys {
		if modifiers&mod.mask != 0 {
			input.sendKeyboardInput(mod.vk, !press)
		}
	}
}

func (input *RDPWindowsInput) handleMouseButtons(buttons byte) {
	changed := buttons ^ input.lastButtons

	if changed != 0 {
		log.Printf("Mouse buttons: %08b -> %08b (changed: %08b)", input.lastButtons, buttons, changed)
	}

	buttonMap := []struct {
		mask     byte
		downFlag uint32
		upFlag   uint32
	}{
		{1 << 0, MOUSEEVENTF_LEFTDOWN, MOUSEEVENTF_LEFTUP},
		{1 << 1, MOUSEEVENTF_RIGHTDOWN, MOUSEEVENTF_RIGHTUP},
		{1 << 2, MOUSEEVENTF_MIDDLEDOWN, MOUSEEVENTF_MIDDLEUP},
	}

	for _, btn := range buttonMap {
		if changed&btn.mask != 0 {
			var flag uint32
			if buttons&btn.mask != 0 {
				flag = btn.downFlag
			} else {
				flag = btn.upFlag
			}
			input.sendMouseButtonInput(flag)
		}
	}

	input.lastButtons = buttons
}

func (input *RDPWindowsInput) sendMouseButtonInput(flag uint32) {
	var mouseInput INPUT
	mouseInput.Type = INPUT_MOUSE

	mouse := (*MOUSEINPUT)(unsafe.Pointer(&mouseInput.Data[0]))
	mouse.Dx = 0
	mouse.Dy = 0
	mouse.MouseData = 0
	mouse.DwFlags = flag
	mouse.Time = 0
	mouse.DwExtraInfo = 0

	sendInput(1, &mouseInput)
}

// Helper functions for Windows API calls
func sendInput(nInputs uint32, pInputs *INPUT) uint32 {
	ret, _, _ := procSendInput.Call(
		uintptr(nInputs),
		uintptr(unsafe.Pointer(pInputs)),
		unsafe.Sizeof(INPUT{}),
	)
	return uint32(ret)
}

func getSystemMetrics(nIndex int32) int32 {
	ret, _, _ := procGetSystemMetrics.Call(uintptr(nIndex))
	return int32(ret)
}

func getCursorPos() (x, y int32) {
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return pt.X, pt.Y
}

func setCursorPos(x, y int32) {
	procSetCursorPos.Call(uintptr(x), uintptr(y))
}
