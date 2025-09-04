package rdp

import "github.com/pion/webrtc/v3"

type Input interface {
	Init() error
	AcceptDataChannel(*webrtc.PeerConnection)
	Close()
}

type AudioVideo interface {
	Init(*StreamConfig, Streamer)
	AttachMediaChannel(*webrtc.PeerConnection)
	Close()
}

type WebRTCConnect interface {
	Start()
	Stop()
}

type StreamConfig struct {
	apiURL    string
	hostname  string
	token     string
	os        string
	codec     string //h264 or hevc
	encoder   string
	bitrate   int
	gopsize   int
	screen    int
	framerate int
	mtu       int
}

type Streamer interface {
	Start(*StreamConfig) error
	Cancel()
}

type Configurator interface {
	GetConfig() (*StreamConfig, error)
}
