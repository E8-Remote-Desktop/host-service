//go:build windows
// +build windows

package e8protocol

import (
	"log"

	"github.com/e8-remote-desktop/host-service/pkg/rdp"
	"github.com/pion/webrtc/v3"
)

type RDPInputConnector struct {
	inputProcessor rdp.InputProcessor
}

func (input *RDPInputConnector) Init(ip rdp.InputProcessor) error {
	input.inputProcessor = ip
	return nil
}

// AcceptDataChannel sets up the handler for incoming WebRTC data channels.
func (input *RDPInputConnector) AcceptDataChannel(pc *webrtc.PeerConnection) {
	pc.OnDataChannel(input.processor)
}

// processor handles messages from the WebRTC data channel.
func (input *RDPInputConnector) processor(dc *webrtc.DataChannel) {
	log.Printf("Data Channel '%s' request received\n", dc.Label())

	dc.OnOpen(func() {
		log.Printf("Data Channel '%s' opened, Input HID Ready\n", dc.Label())
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		data := msg.Data
		if len(data) == 0 {
			return
		}
		// TODO decide whether to keep this
		if input.inputProcessor.IsStarted() {
			input.inputProcessor.Send(data)
		}

	})
}
