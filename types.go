package rdp

import "github.com/pion/webrtc/v3"

type Input interface {
	AcceptDataChannel()
}

type AudioVideo interface {
	AttachMediaChannel(*webrtc.PeerConnection)
}

type WebRTCConnect interface {
	Start()
}
