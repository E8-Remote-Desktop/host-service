package rdp

import (
	"fmt"

	"github.com/pion/webrtc/v3"
)

type RDPInput struct {
	PeerConnection *webrtc.PeerConnection
}

func (input *RDPInput) StartDataChannel() {

	dataChannel, err := input.PeerConnection.CreateDataChannel("input", nil)
	if err != nil {
		panic(err)
	}
	dataChannel.OnOpen(func() {
		fmt.Println("Data Channel Connection Established")
		dataChannel.SendText("Hello World")
	})
}
