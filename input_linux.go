//go:build linux
// +build linux

package rdp

import (
	"encoding/binary"
	"log"

	"github.com/bendahl/uinput"
	"github.com/pion/webrtc/v3"
)

func GetInput() Input {
	return &RDPLinuxInput{}
}

type RDPLinuxInput struct {
	keyboard    uinput.Keyboard
	mouse       uinput.Mouse
	lastButtons byte
}

func (input *RDPLinuxInput) Init() error {
	var err error

	input.keyboard, err = uinput.CreateKeyboard("/dev/uinput", []byte("rdp-keyboard"))
	if err != nil {
		return err
	}

	input.mouse, err = uinput.CreateMouse("/dev/uinput", []byte("rdp-mouse"))
	if err != nil {
		input.keyboard.Close()
		return err
	}

	log.Println("Input devices initialized")
	return nil
}

func (input *RDPLinuxInput) Close() {
	if input.keyboard != nil {
		input.keyboard.Close()
	}
	if input.mouse != nil {
		input.mouse.Close()
	}
}

func (input *RDPLinuxInput) AcceptDataChannel(pc *webrtc.PeerConnection) {
	pc.OnDataChannel(input.processor)
}

func (input *RDPLinuxInput) processor(dc *webrtc.DataChannel) {
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
			if err := input.keyboard.KeyDown(int(keyCode)); err != nil {
				log.Printf("KeyDown error: %v", err)
			}
			if err := input.keyboard.KeyUp(int(keyCode)); err != nil {
				log.Printf("KeyUp error: %v", err)
			}
			input.handleModifiers(modifiers, false)

		case 2: // Mouse move + buttons
			if len(data) < 6 {
				log.Println("Invalid mouse move packet")
				return
			}

			buttons := data[1]
			dx := int16(binary.BigEndian.Uint16(data[2:4]))
			dy := int16(binary.BigEndian.Uint16(data[4:6]))

			if err := input.mouse.Move(int32(dx), int32(dy)); err != nil {
				log.Printf("Mouse move error: %v", err)
			}
			input.handleMouseButtons(buttons)

		case 3: // Scroll
			if len(data) < 4 {
				log.Println("Invalid scroll packet")
				return
			}

			deltaY := int32(binary.BigEndian.Uint16(data[1:3]))
			//deltaX := int8(data[3]) // currently unused

			if deltaY > 0 || deltaY < 0 {
				if err := input.mouse.Wheel(false, deltaY); err != nil {
					log.Printf("Scroll down error: %v", err)
				}
			}

		default:
			log.Printf("Unknown input event type: %d", data[0])
		}
	})
}

func (input *RDPLinuxInput) handleModifiers(modifiers byte, press bool) {
	modKeys := []struct {
		mask byte
		key  int
	}{
		{1 << 0, uinput.KeyLeftctrl},
		{1 << 1, uinput.KeyLeftshift},
		{1 << 2, uinput.KeyLeftalt},
		{1 << 3, uinput.KeyLeftmeta},
	}

	for _, mod := range modKeys {
		if modifiers&mod.mask != 0 {
			if press {
				_ = input.keyboard.KeyDown(mod.key)
			} else {
				_ = input.keyboard.KeyUp(mod.key)
			}
		}
	}
}

func (input *RDPLinuxInput) handleMouseButtons(buttons byte) {
	changed := buttons ^ input.lastButtons

	// Debug logging (remove this in production)
	if changed != 0 {
		log.Printf("Mouse buttons: %08b -> %08b (changed: %08b)", input.lastButtons, buttons, changed)
	}

	buttonMap := []struct {
		mask    byte
		press   func() error
		release func() error
	}{
		{1 << 0, input.mouse.LeftPress, input.mouse.LeftRelease},
		{1 << 1, input.mouse.RightPress, input.mouse.RightRelease},
		{1 << 2, input.mouse.MiddlePress, input.mouse.MiddleRelease},
	}

	for _, btn := range buttonMap {
		// Only process if this button's state has changed
		if changed&btn.mask != 0 {
			if buttons&btn.mask != 0 {
				// Button is now pressed
				if err := btn.press(); err != nil {
					log.Printf("Mouse button press error: %v", err)
				}
			} else {
				// Button is now released
				if err := btn.release(); err != nil {
					log.Printf("Mouse button release error: %v", err)
				}
			}
		}
	}

	input.lastButtons = buttons
}
