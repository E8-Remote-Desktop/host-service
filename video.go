package rdp

import (
	"github.com/pion/webrtc/v3"
)

type RDPVideo struct {
	PeerConnection *webrtc.PeerConnection
}

func (video *RDPVideo) IngestWhip() {

}

func (video *RDPVideo) StartMediaChannel() {

}
