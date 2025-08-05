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

// keyMap translates linux input codes  to windows scancodes
// linux ftw

var scMap = map[uint16]uint16{
	1:   0x01, // Escape
	2:   0x02, // '1'
	3:   0x03, // '2'
	4:   0x04, // '3'
	5:   0x05, // '4'
	6:   0x06, // '5'
	7:   0x07, // '6'
	8:   0x08, // '7'
	9:   0x09, // '8'
	10:  0x0A, // '9'
	11:  0x0B, // '0'
	12:  0x0C, // '-' (OEM_MINUS)
	13:  0x0D, // '=' (OEM_PLUS)
	14:  0x0E, // Backspace
	15:  0x0F, // Tab
	16:  0x10, // 'Q'
	17:  0x11, // 'W'
	18:  0x12, // 'E'
	19:  0x13, // 'R'
	20:  0x14, // 'T'
	21:  0x15, // 'Y'
	22:  0x16, // 'U'
	23:  0x17, // 'I'
	24:  0x18, // 'O'
	25:  0x19, // 'P'
	26:  0x1A, // '['
	27:  0x1B, // ']'
	28:  0x1C, // Enter
	29:  0x1D, // Left ctrl
	30:  0x1E, // 'A'
	31:  0x1F, // 'S'
	32:  0x20, // 'D'
	33:  0x21, // 'F'
	34:  0x22, // 'G'
	35:  0x23, // 'H'
	36:  0x24, // 'J'
	37:  0x25, // 'K'
	38:  0x26, // 'L'
	39:  0x27, // ';'
	40:  0x28, // '''
	41:  0x29, // '`'
	42:  0x2A, // Left shift
	43:  0x2B, // '\'
	44:  0x2C, // 'Z'
	45:  0x2D, // 'X'
	46:  0x2E, // 'C'
	47:  0x2F, // 'V'
	48:  0x30, // 'B'
	49:  0x31, // 'N'
	50:  0x32, // 'M'
	51:  0x33, // ','
	52:  0x34, // '.'
	53:  0x35, // '/'
	54:  0x36, // 'Right Shift'
	56:  0x38, // Left Alt
	57:  0x39, // Space
	58:  0x3A, // Caps Lock
	59:  0x3B, // F1
	60:  0x3C, // F2
	61:  0x3D, // F3
	62:  0x3E, // F4
	63:  0x3F, // F5
	64:  0x40, // F6
	65:  0x41, // F7
	66:  0x42, // F8
	67:  0x43, // F9
	68:  0x44, // F10
	87:  0x57, // F11
	88:  0x58, // F12
	97:  0x1C,
	103: 0x48,   // Up Arrow
	105: 0x4B,   // Left Arrow
	106: 0x4D,   // Right Arrow
	108: 0x50,   // Down Arrow
	111: 0xE053, // Delete (extended)
	125: 0xE05B,
	126: 0xE05C,
}

const (
	KEYEVENTF_KEYDOWN  = 0x0000
	KEYEVENTF_UNICODE  = 0x0004
	KEYEVENTF_KEYUP    = 0x0002
	KEYEVENTF_SCANCODE = 0x0008
	INPUT_KEYBOARD     = 1
)

const (
	SC_PAUSE = 0x45   // Pause/Break (complex key in practice)
	SC_APPS  = 0xE05D // Menu key (extended)
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

// GetInput returns the Windows-specific input handler.
func GetInput() Input {
	return &RDPWindowsInput{}
}

// RDPWindowsInput handles remote input events on Windows using robotgo.
type RDPWindowsInput struct {
	lastButtons byte
	pressedKeys map[uint16]bool
}

// Init initializes the input handler. For Windows, this is a no-op
// as robotgo doesn't require explicit device creation like uinput.
func (input *RDPWindowsInput) Init() error {
	input.pressedKeys = make(map[uint16]bool)
	log.Println("Windows Input handler initialized")
	return nil
}

func (input *RDPWindowsInput) Close() {
	dll, err := user32util.LoadUser32DLL()
	if err != nil {
		log.Fatal("Could not initalize windows input layer")
	}
	for key, value := range input.pressedKeys {
		if value {
			SendKeyboardInput(dll, key, false)
		}
	}
	input.pressedKeys = make(map[uint16]bool)
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
			if len(data) < 5 {
				log.Println("Invalid keyboard packet")
				return
			}
			keyCode := binary.BigEndian.Uint16(data[1:3])
			keyDown := data[4] == 1

			sc, ok := scMap[keyCode]
			if !ok {
				log.Printf("Unknown key code: %d\n", keyCode)
				return
			}

			// Check if state already matches
			if !input.pressedKeys[sc] && !keyDown {
				return // already pressed or already released, skip
			}

			// Send main key event
			SendKeyboardInput(dll, sc, keyDown)
			input.pressedKeys[sc] = keyDown

		case 2: // Mouse Move
			if len(data) < 6 {
				log.Println("Invalid mouse move packet")
				return
			}
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
