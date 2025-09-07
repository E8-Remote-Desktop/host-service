//go:build windows
// +build windows

package rdp

import (
	"log"

	"github.com/pion/webrtc/v3"
)

// keyMap translates linux input codes  to windows scancodes
// linux ftw

type DatachannelMsg struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type RDPWindowsInput struct {
}

// Init initializes the input handler. For Windows, this is a no-op
func (input *RDPWindowsInput) Init() error {
}

// AcceptDataChannel sets up the handler for incoming WebRTC data channels.
func (input *RDPWindowsInput) AcceptDataChannel(pc *webrtc.PeerConnection) {
	pc.OnDataChannel(input.processor)
}

// processor handles messages from the WebRTC data channel.
func (input *RDPWindowsInput) processor(dc *webrtc.DataChannel) {
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
	})
}
