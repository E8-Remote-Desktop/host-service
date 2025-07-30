//go:build windows
// +build windows

package rdp

import (
	"encoding/binary"
	"log"
	"unsafe"

	"github.com/go-vgo/robotgo"
	"github.com/pion/webrtc/v3"
	"github.com/stephen-fox/user32util"
)

// keyMap translates linux input codes  to windows input keycodes
// linux ftw
var keyMap = map[uint16]uint16{
	1:   0x1B, // Escape
	2:   0x31, // '1'
	3:   0x32, // '2'
	4:   0x33, // '3'
	5:   0x34, // '4'
	6:   0x35, // '5'
	7:   0x36, // '6'
	8:   0x37, // '7'
	9:   0x38, // '8'
	10:  0x39, // '9'
	11:  0x30, // '0'
	12:  0xBD, // '-' (VK_OEM_MINUS)
	13:  0xBB, // '=' (VK_OEM_PLUS)
	14:  0x08, // Backspace
	15:  0x09, // Tab
	16:  0x51, // 'Q'
	17:  0x57, // 'W'
	18:  0x45, // 'E'
	19:  0x52, // 'R'
	20:  0x54, // 'T'
	21:  0x59, // 'Y'
	22:  0x55, // 'U'
	23:  0x49, // 'I'
	24:  0x4F, // 'O'
	25:  0x50, // 'P'
	26:  0xDB, // '[' (VK_OEM_4)
	27:  0xDD, // ']' (VK_OEM_6)
	28:  0x0D, // Enter
	30:  0x41, // 'A'
	31:  0x53, // 'S'
	32:  0x44, // 'D'
	33:  0x46, // 'F'
	34:  0x47, // 'G'
	35:  0x48, // 'H'
	36:  0x4A, // 'J'
	37:  0x4B, // 'K'
	38:  0x4C, // 'L'
	39:  0xBA, // ';' (VK_OEM_1)
	40:  0xDE, // ''' (VK_OEM_7)
	41:  0xC0, // '`' (VK_OEM_3)
	43:  0xDC, // '\' (VK_OEM_5)
	44:  0x5A, // 'Z'
	45:  0x58, // 'X'
	46:  0x43, // 'C'
	47:  0x56, // 'V'
	48:  0x42, // 'B'
	49:  0x4E, // 'N'
	50:  0x4D, // 'M'
	51:  0xBC, // ',' (VK_OEM_COMMA)
	52:  0xBE, // '.' (VK_OEM_PERIOD)
	53:  0xBF, // '/' (VK_OEM_2)
	57:  0x20, // Space
	58:  0x14, // Caps Lock
	59:  0x70, // F1
	60:  0x71, // F2
	61:  0x72, // F3
	62:  0x73, // F4
	63:  0x74, // F5
	64:  0x75, // F6
	65:  0x76, // F7
	66:  0x77, // F8
	67:  0x78, // F9
	68:  0x79, // F10
	87:  0x7A, // F11
	88:  0x7B, // F12
	103: 0x26, // Up Arrow
	105: 0x25, // Left Arrow
	106: 0x27, // Right Arrow
	108: 0x28, // Down Arrow
	111: 0x2E, // Delete
}

const (
	KEYEVENTF_KEYDOWN = 0x0000
	KEYEVENTF_KEYUP   = 0x0002
	INPUT_KEYBOARD    = 1
)

const (
	VK_BACK    = 0x08
	VK_TAB     = 0x09
	VK_RETURN  = 0x0D
	VK_SHIFT   = 0x10
	VK_CONTROL = 0x11
	VK_MENU    = 0x12 // Alt key
	VK_PAUSE   = 0x13
	VK_CAPITAL = 0x14
	VK_ESCAPE  = 0x1B
	VK_SPACE   = 0x20
	VK_LEFT    = 0x25
	VK_UP      = 0x26
	VK_RIGHT   = 0x27
	VK_DOWN    = 0x28
	VK_LWIN    = 0x5B
	VK_RWIN    = 0x5C
	VK_APPS    = 0x5D
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

func SendKeyboardInput(dll *user32util.User32DLL, vk uint16, keyDown bool) {
	flags := uint32(0)
	if !keyDown {
		flags = KEYEVENTF_KEYUP
	}

	input := INPUT{
		Type: INPUT_KEYBOARD,
		Ki: KEYBDINPUT{
			Vk:        vk,
			Scan:      0,
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

// GetInput returns the Windows-specific input handler.
func GetInput() Input {
	return &RDPWindowsInput{}
}

// RDPWindowsInput handles remote input events on Windows using robotgo.
type RDPWindowsInput struct {
	lastButtons byte
}

// Init initializes the input handler. For Windows, this is a no-op
// as robotgo doesn't require explicit device creation like uinput.
func (input *RDPWindowsInput) Init() error {
	log.Println("Windows Input handler initialized (robotgo)")
	return nil
}

// Close cleans up resources. For Windows, this is a no-op.
func (input *RDPWindowsInput) Close() {
	// robotgo does not require explicit closing of devices.
}

// AcceptDataChannel sets up the handler for incoming WebRTC data channels.
func (input *RDPWindowsInput) AcceptDataChannel(pc *webrtc.PeerConnection) {
	pc.OnDataChannel(input.processor)
}

// processor handles messages from the WebRTC data channel.
func (input *RDPWindowsInput) processor(dc *webrtc.DataChannel) {
	dll, err := user32util.LoadUser32DLL()
	if err != nil {
		log.Fatal("Could not initalize windows input layer")
	}
	log.Printf("Data Channel '%s' request received\n", dc.Label())

	dc.OnOpen(func() {
		log.Printf("Data Channel '%s' opened, Input HID Ready\n", dc.Label())
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		data := msg.Data
		if len(data) == 0 {
			return
		}

		switch data[0] {
		case 1: // Keyboard Event
			if len(data) < 4 {
				log.Println("Invalid keyboard packet")
				return
			}
			keyCode := binary.BigEndian.Uint16(data[1:3])
			modifiers := data[3]

			vk, ok := keyMap[keyCode]
			if !ok {
				log.Printf("Unknown key code: %d\n", keyCode)
				return
			}

			// Send modifiers down
			if modifiers&(1<<0) != 0 {
				SendKeyboardInput(dll, VK_CONTROL, true)
			}
			if modifiers&(1<<1) != 0 {
				SendKeyboardInput(dll, VK_SHIFT, true)
			}
			if modifiers&(1<<2) != 0 {
				SendKeyboardInput(dll, VK_MENU, true) // Alt
			}
			if modifiers&(1<<3) != 0 {
				SendKeyboardInput(dll, VK_LWIN, true) // Win key
			}

			// Send main key down and up
			SendKeyboardInput(dll, vk, true)
			SendKeyboardInput(dll, vk, false)

			// Release modifiers
			if modifiers&(1<<3) != 0 {
				SendKeyboardInput(dll, VK_LWIN, false)
			}
			if modifiers&(1<<2) != 0 {
				SendKeyboardInput(dll, VK_MENU, false)
			}
			if modifiers&(1<<1) != 0 {
				SendKeyboardInput(dll, VK_SHIFT, false)
			}
			if modifiers&(1<<0) != 0 {
				SendKeyboardInput(dll, VK_CONTROL, false)
			}

		case 2: // Mouse Move + Buttons Event
			if len(data) < 6 {
				log.Println("Invalid mouse move packet")
				return
			}
			buttons := data[1]
			dx := int16(binary.BigEndian.Uint16(data[2:4]))
			dy := int16(binary.BigEndian.Uint16(data[4:6]))

			// Move mouse relative to current position
			//currentX, currentY := robotgo.Location()
			//robotgo.MoveRelative(int(dx), int(dy))
			user32util.SendMouseInput(user32util.MouseInput{
				DwFlags: user32util.MouseEventFMove,
				Dx:      int32(dx),
				Dy:      int32(dy),
			}, dll)

			input.handleMouseButtons(buttons)
		case 3: // Mouse Button Event
			if len(data) < 3 {
				log.Println("Invalid mouse button packet")
				return
			}
			button := data[1]
			action := data[2] // 1 = down, 0 = up

			var btnName string
			switch button {
			case 0:
				btnName = "left"
			case 1:
				btnName = "middle"
			case 2:
				btnName = "right"
			default:
				log.Printf("Unknown mouse button: %d\n", button)
				return
			}

			var direction string
			if action == 1 {
				direction = "down"
			} else if action == 0 {
				direction = "up"
			} else {
				log.Printf("Unknown mouse button action: %d\n", action)
				return
			}

			if err := robotgo.Toggle(btnName, direction); err != nil {
				log.Printf("Mouse %s %s error: %v", btnName, direction, err)
			}
		case 4: // Scroll Event
			if len(data) < 4 {
				log.Println("Invalid scroll packet")
				return
			}
			// robotgo scroll direction is inverted compared to many systems.
			// Positive deltaY usually means scrolling down, which for robotgo is "up".
			deltaY := int16(binary.BigEndian.Uint16(data[1:3]))

			// robotgo's Scroll function takes two arguments, x and y scroll amount.
			// We are only using the y-scroll here.
			if deltaY != 0 {
				// We negate deltaY because robotgo's "up" direction is negative.
				robotgo.Scroll(0, -int(deltaY))
			}

		default:
			log.Printf("Unknown input event type: %d", data[0])
		}
	})
}

// handleMouseButtons processes mouse button press and release events.
func (input *RDPWindowsInput) handleMouseButtons(buttons byte) {
	changed := buttons ^ input.lastButtons

	if changed == 0 {
		return // No change in button state
	}

	buttonMap := []struct {
		mask byte
		name string
	}{
		{1 << 0, "left"},
		{1 << 1, "right"},
		{1 << 2, "center"}, // "center" is often the middle button click
	}

	for _, btn := range buttonMap {
		if changed&btn.mask != 0 {
			// Determine if it was a press or release
			isPressed := buttons&btn.mask != 0
			direction := "up"
			if isPressed {
				direction = "down"
			}

			// robotgo.Toggle handles both press and release
			if err := robotgo.Toggle(btn.name, direction); err != nil {
				log.Printf("Mouse %s %s error: %v", btn.name, direction, err)
			}
		}
	}

	input.lastButtons = buttons
}
