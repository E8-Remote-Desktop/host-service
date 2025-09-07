package rdp

import "github.com/pion/webrtc/v3"

type Input interface {
	Init() error
	AcceptDataChannel(*webrtc.PeerConnection)
	Close()
}

type StreamConnector interface {
	Init(*StreamConfig, MediaStreamer)
	AttachMediaChannel(*webrtc.PeerConnection)
	Close()
}

type WebRTCConnect interface {
	Start()
	Stop()
}

type StreamConfig struct {
	ApiURL    string
	Hostname  string
	Token     string
	Codec     string //h264 or hevc
	Encoder   string
	Bitrate   int
	Gopsize   int
	Screen    int
	Framerate int
	MTU       int
}

type MediaStreamer interface {
	Start(*StreamConfig) error
	Cancel()
}

type Configurator interface {
	GetConfig() (*StreamConfig, error)
}

// mainly used for authentication and os abstractions, should be it's own standalone thing
type OSHelper interface {
	Init() error
}
