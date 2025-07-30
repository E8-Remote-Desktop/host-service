//go:build windows
// +build windows

package rdp

import (
	"encoding/binary"
	"log"

	"github.com/go-vgo/robotgo"
	"github.com/pion/webrtc/v3"
)

// keyMap translates input event codes (sent by the client) to robotgo key strings.
var keyMap = map[uint16]string{
	1:  "escape",
	2:  "1",
	3:  "2",
	4:  "3",
	5:  "4",
	6:  "5",
	7:  "6",
	8:  "7",
	9:  "8",
	10: "9",
	11: "0",
	12: "-",
	13: "=",
	14: "backspace",
	15: "tab",
	16: "q",
	17: "w",
	18: "e",
	19: "r",
	20: "t",
	21: "y",
	22: "u",
	23: "i",
	24: "o",
	25: "p",
	26: "[",
	27: "]",
	28: "enter",
	// 29: "ControlLeft" - Handled by modifier byte
	30: "a",
	31: "s",
	32: "d",
	33: "f",
	34: "g",
	35: "h",
	36: "j",
	37: "k",
	38: "l",
	39: ";",
	40: "'",
	41: "`",
	// 42: "ShiftLeft" - Handled by modifier byte
	43: "\\",
	44: "z",
	45: "x",
	46: "c",
	47: "v",
	48: "b",
	49: "n",
	50: "m",
	51: ",",
	52: ".",
	53: "/",
	// 54: "ShiftRight" - Handled by modifier byte
	// 56: "AltLeft" - Handled by modifier byte
	57: "space",
	58: "capslock",
	59: "f1",
	60: "f2",
	61: "f3",
	62: "f4",
	63: "f5",
	64: "f6",
	65: "f7",
	66: "f8",
	67: "f9",
	68: "f10",
	87: "f11",
	88: "f12",
	// 97: "ControlRight" - Handled by modifier byte
	// 100: "AltRight" - Handled by modifier byte
	103: "up",
	105: "left",
	106: "right",
	108: "down",
	111: "delete",
	// 125: "MetaLeft" - Handled by modifier byte
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

			keyString, ok := keyMap[keyCode]
			if !ok {
				log.Printf("Unknown key code: %d\n", keyCode)
				return
			}

			// Build modifier list for robotgo
			var modStrings []string
			if modifiers&(1<<0) != 0 {
				modStrings = append(modStrings, "ctrl")
			}
			if modifiers&(1<<1) != 0 {
				modStrings = append(modStrings, "shift")
			}
			if modifiers&(1<<2) != 0 {
				modStrings = append(modStrings, "alt")
			}
			// In robotgo, "cmd" maps to the Windows key
			if modifiers&(1<<3) != 0 {
				modStrings = append(modStrings, "cmd")
			}

			// Use KeyTap to press the key with modifiers
			if err := robotgo.KeyTap(keyString, modStrings); err != nil {
				log.Printf("KeyTap error: %v", err)
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
			currentX, currentY := robotgo.Location()
			robotgo.Move(currentX+int(dx), currentY+int(dy))

			input.handleMouseButtons(buttons)

		case 3: // Scroll Event
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
