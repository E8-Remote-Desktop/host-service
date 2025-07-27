package rdp

import (
	"log"
	"net"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

type RDPAudioVideo struct {
	PeerConnection *webrtc.PeerConnection
}

func (video *RDPAudioVideo) StartStream() {

}

func (video *RDPAudioVideo) AttachMediaChannel() {
	video.StartStream()
	// Create tracks
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"rdp-video",
	)

	if err != nil {
		log.Fatal("Failed to create video track")
		panic(err)
	}

	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"video",
		"rdp-video",
	)
	if err != nil {
		log.Fatal("Failed to create video track")
		panic(err)
	}

	// Add tracks
	_, err = video.PeerConnection.AddTrack(videoTrack)
	if err != nil {
		panic(err)
	}
	_, err = video.PeerConnection.AddTrack(audioTrack)
	if err != nil {
		panic(err)
	}

	// RTP Loop to injest the RTP frames
	go video.receiveRTPAndForward("127.0.0.1:5004", audioTrack)
	go video.receiveRTPAndForward("127.0.0.1:5005", videoTrack)
}

func (video *RDPAudioVideo) receiveRTPAndForward(listenAddr string, track *webrtc.TrackLocalStaticRTP) {
	conn, err := net.ListenPacket("udp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to bind to UDP: %v", err)
	}
	defer conn.Close()
	log.Printf("Listening for RTP on %s\n", listenAddr)

	buf := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			log.Printf("Read error: %v", err)
			continue
		}

		packet := &rtp.Packet{}
		if err := packet.Unmarshal(buf[:n]); err != nil {
			log.Printf("Failed to parse RTP packet: %v", err)
			continue
		}

		raw, err := packet.Marshal()
		if err != nil {
			log.Printf("Failed to marshal RTP packet: %v", err)
			continue
		}

		// Write the RTP packet to the WebRTC track
		if _, writeErr := track.Write(raw); writeErr != nil {
			log.Printf("Failed to write RTP to track: %v", writeErr)
		}
		log.Println("SENT RTP PACKET TO CLIENT")
	}
}
