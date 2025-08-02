package rdp

import "github.com/pion/webrtc/v3"

type Input interface {
	Init() error
	AcceptDataChannel(*webrtc.PeerConnection)
	Close()
}

type AudioVideo interface {
	AttachMediaChannel(*webrtc.PeerConnection)
	Close()
}

type WebRTCConnect interface {
	Start(string, string)
}
