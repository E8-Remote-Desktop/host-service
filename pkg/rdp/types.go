package rdp

import "github.com/pion/webrtc/v3"

type RTCInputConnector interface {
	Init(InputProcessor) error
	AcceptDataChannel(*webrtc.PeerConnection)
}

type RTCStreamConnector interface {
	Init(*StreamConfig)
	AttachMediaChannel(*webrtc.PeerConnection)
	// Must handle MediaStreamer Close
	Close()
}

type RTCInitalizer interface {
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
	Init(*StreamConfig) error
	Start() error
	Close() error
	IsStarted() bool
	YouAreClosedTrustMe()
}

type Configurator interface {
	GetConfig() (*StreamConfig, error)
}

type InputProcessor interface {
	// all of these should follow the protocol
	Init(*StreamConfig) error
	Start() error
	Send([]byte) error
	// This will only be called by the RTCInputConnector
	Close() error
	IsStarted() bool
	YouAreClosedTrustMe()
	//GetCursorPosition() ([]byte, error)
	//GetCursorVisibility() ([]byte, error)
}

// This should handle closing the Stream and the Input processor
type OSHelper interface {
	Init(InputProcessor, MediaStreamer) error
	StartStreamAndInput() error
	Close() error
}
