//go:build windows
// +build windows

package rdp

/*
#include <windows.h>

// Helper function to send keyboard input
void sendKeyboardInput(WORD vk, BOOL keyDown) {
	INPUT input = {0};
	input.type = INPUT_KEYBOARD;
	input.ki.wVk = vk;
	if (!keyDown) {
		input.ki.dwFlags = KEYEVENTF_KEYUP;
	}
	SendInput(1, &input, sizeof(INPUT));
}

// Helper function to send mouse input
void sendMouseInput(DWORD flags, LONG dx, LONG dy, DWORD mouseData) {
	INPUT input = {0};
	input.type = INPUT_MOUSE;
	input.mi.dx = dx;
	input.mi.dy = dy;
	input.mi.dwFlags = flags;
	input.mi.mouseData = mouseData;
	SendInput(1, &input, sizeof(INPUT));
}
*/
import "C"

import (
	"encoding/binary"
	"log"

	"github.com/pion/webrtc/v3"
)

func GetInput() Input {
	return &RDPWindowsInput{}
}

type RDPWindowsInput struct {
	lastButtons byte
}

func (input *RDPWindowsInput) Init() error {
	// Nothing special to initialize for Windows input injection
	log.Println("Windows input initialized")
	return nil
}

func (input *RDPWindowsInput) Close() {
	// Nothing to close on Windows
}

func (input *RDPWindowsInput) AcceptDataChannel(pc *webrtc.PeerConnection) {
	pc.OnDataChannel(input.processor)
}

func (input *RDPWindowsInput) processor(dc *webrtc.DataChannel) {
	log.Printf("Data Channel %s request received\n", dc.Label())

	dc.OnOpen(func() {
		log.Printf("Data Channel %s opened, Input HID Ready\n", dc.Label())
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
			input.keyPressWindows(keyCode)
			input.handleModifiers(modifiers, false)

		case 2: // Mouse move + buttons
			if len(data) < 6 {
				log.Println("Invalid mouse move packet")
				return
			}

			buttons := data[1]
			dx := int16(binary.BigEndian.Uint16(data[2:4]))
			dy := int16(binary.BigEndian.Uint16(data[4:6]))

			input.mouseMoveWindows(int32(dx), int32(dy))
			input.handleMouseButtons(buttons)

		case 3: // Scroll
			if len(data) < 4 {
				log.Println("Invalid scroll packet")
				return
			}

			deltaY := int16(binary.BigEndian.Uint16(data[1:3]))
			input.mouseScrollWindows(deltaY)

		default:
			log.Printf("Unknown input event type: %d", data[0])
		}
	})
}

// KeyPress sends a key press (down + up) to Windows using Virtual-Key codes
func (input *RDPWindowsInput) keyPressWindows(keyCode uint16) {
	vk := windowsVirtualKey(keyCode)
	if vk == 0 {
		log.Printf("Unknown keycode %d", keyCode)
		return
	}
	// Key down
	C.sendKeyboardInput(C.WORD(vk), C.TRUE)
	// Key up
	C.sendKeyboardInput(C.WORD(vk), C.FALSE)
}

func (input *RDPWindowsInput) handleModifiers(modifiers byte, press bool) {
	modKeys := []struct {
		mask byte
		vk   uint16
	}{
		{1 << 0, 0xA2}, // Left Ctrl: VK_LCONTROL
		{1 << 1, 0xA0}, // Left Shift: VK_LSHIFT
		{1 << 2, 0xA4}, // Left Alt: VK_LMENU
		{1 << 3, 0x5B}, // Left Meta (Windows key): VK_LWIN
	}

	for _, mod := range modKeys {
		if modifiers&mod.mask != 0 {
			if press {
				C.sendKeyboardInput(C.WORD(mod.vk), C.TRUE)
			} else {
				C.sendKeyboardInput(C.WORD(mod.vk), C.FALSE)
			}
		}
	}
}

func (input *RDPWindowsInput) mouseMoveWindows(dx, dy int32) {
	// Send relative mouse movement
	C.sendMouseInput(C.MOUSEEVENTF_MOVE, C.LONG(dx), C.LONG(dy), 0)
}

func (input *RDPWindowsInput) mouseScrollWindows(deltaY int16) {
	// Vertical scroll wheel: WHEEL_DELTA = 120 per notch
	const WHEEL_DELTA = 120
	C.sendMouseInput(C.MOUSEEVENTF_WHEEL, 0, 0, C.DWORD(int32(deltaY)*WHEEL_DELTA))
}

func (input *RDPWindowsInput) handleMouseButtons(buttons byte) {
	changed := buttons ^ input.lastButtons

	if changed != 0 {
		log.Printf("Mouse buttons: %08b -> %08b (changed: %08b)", input.lastButtons, buttons, changed)
	}

	buttonMap := []struct {
		mask    byte
		press   uint32
		release uint32
	}{
		{1 << 0, C.MOUSEEVENTF_LEFTDOWN, C.MOUSEEVENTF_LEFTUP},
		{1 << 1, C.MOUSEEVENTF_RIGHTDOWN, C.MOUSEEVENTF_RIGHTUP},
		{1 << 2, C.MOUSEEVENTF_MIDDLEDOWN, C.MOUSEEVENTF_MIDDLEUP},
	}

	for _, btn := range buttonMap {
		if changed&btn.mask != 0 {
			if buttons&btn.mask != 0 {
				// Press
				C.sendMouseInput(btn.press, 0, 0, 0)
			} else {
				// Release
				C.sendMouseInput(btn.release, 0, 0, 0)
			}
		}
	}

	input.lastButtons = buttons
}

// windowsVirtualKey converts your incoming keyCode to a Windows Virtual-Key code.
// This is a simplified example; you will likely need a full mapping for your use case.
func windowsVirtualKey(keyCode uint16) uint16 {
	// Directly use the keyCode if it maps well or implement mapping
	// For example: 0x41 = 'A' in ASCII and VK_A = 0x41
	// You should expand this mapping as per your protocol

	// For demo: assume keyCode is ASCII and map A-Z, 0-9, arrows, etc.
	if keyCode >= 0x41 && keyCode <= 0x5A { // A-Z
		return keyCode
	}
	if keyCode >= 0x30 && keyCode <= 0x39 { // 0-9
		return keyCode
	}

	// Add special keys mapping as needed here

	return 0 // unknown key
}
