package helpers

import (
	"log"
	"unsafe"

	"github.com/stephen-fox/user32util"
)

const (
	KEYEVENTF_KEYDOWN  = 0x0000
	KEYEVENTF_UNICODE  = 0x0004
	KEYEVENTF_KEYUP    = 0x0002
	KEYEVENTF_SCANCODE = 0x0008
	INPUT_KEYBOARD     = 1
)

type KEYBDINPUT struct {
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
	_         [8]byte // fix padding
}

type INPUT struct {
	Type uint32
	_    [4]byte
	Ki   KEYBDINPUT
}

func SendKeyboardInput(dll *user32util.User32DLL, sc uint16, keyDown bool) {
	var flags uint32 = KEYEVENTF_SCANCODE
	if !keyDown {
		flags |= KEYEVENTF_KEYUP
	}
	if sc&0xFF00 == 0xE000 {
		flags |= 0x0001 // KEYEVENTF_EXTENDEDKEY
		sc &= 0xFF      // Only pass the low byte (e.g. 0x4D instead of 0xE04D)
	}
	input := INPUT{
		Type: INPUT_KEYBOARD,
		Ki: KEYBDINPUT{
			Vk:        0,
			Scan:      sc,
			Flags:     flags,
			Time:      0,
			ExtraInfo: 0,
		},
	}

	if err := user32util.SendInput(1, unsafe.Pointer(&input), unsafe.Sizeof(input), dll); err != nil {
		log.Printf("Failed to send keyboard input err: %v", err)
		log.Printf("sizeof(KEYBDINPUT): %d", unsafe.Sizeof(KEYBDINPUT{})) // should be 24 on 64-bit
		log.Printf("sizeof(INPUT): %d", unsafe.Sizeof(INPUT{}))
	}
}
