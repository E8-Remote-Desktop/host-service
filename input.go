package rdp

import (
	"log"

	"github.com/pion/webrtc/v3"
)

type RDPInput struct {
	PeerConnection *webrtc.PeerConnection
}

func (input *RDPInput) AcceptDataChannel() {
	input.PeerConnection.OnDataChannel(input.processor)
}

func (input *RDPInput) processor(dc *webrtc.DataChannel) {
	log.Printf("Data Channel %s request recieved\n", dc.Label())

	dc.OnOpen(func() {
		log.Printf("Data Channel %s opened, Input HID Ready\n", dc.Label())
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("Recieved HID Input: %s", string(msg.Data))
		// TODO: Actually do something with the data
	})

}
