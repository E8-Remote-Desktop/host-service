package rdp

import (
	"log"
	"net"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

type RDPAudioVideo struct {
	rtpStreamGoing bool
}

func (video *RDPAudioVideo) StartStream() {

}

func (video *RDPAudioVideo) AttachMediaChannel(PeerConnection *webrtc.PeerConnection) {
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

	videoTransceiver, err := PeerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != nil {
		panic(err)
	}
	audioTransceiver, err := PeerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	if err != nil {
		panic(err)
	}

	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"rdp-audio",
	)
	if err != nil {
		panic(err)
	}

	// Add tracks
	if videoTransceiver.Sender() != nil {
		videoTransceiver.Sender().ReplaceTrack(videoTrack)
	} else {
		log.Printf("Video transceiver does not have sender I hate you chatgpt")
	}

	if audioTransceiver.Sender() != nil {
		audioTransceiver.Sender().ReplaceTrack(audioTrack)
	} else {
		log.Printf("Audio transceiver does not have sender I hate you chatgpt")
	}

	// RTP Loop to injest the RTP frames
	log.Printf("Starting Media Stream")
	if !video.rtpStreamGoing {
		log.Println("RTP Stream Started, running fuse tripped")
		go video.receiveRTPAndForward("127.0.0.1:50045", audioTrack)
		go video.receiveRTPAndForward("127.0.0.1:50055", videoTrack)
		video.rtpStreamGoing = true
	} else {
		log.Println("\033[31mRTP Stream request IGNORED, already running for another track\033[0m")
	}

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
		_, writeErr := track.Write(raw)
		if writeErr != nil {
			log.Printf("Failed to write RTP to track: %v", writeErr)
		}
		//log.Printf("SENT RTP %d PACKETs TO CLIENT", n)
		//log.Printf("Received RTP Packet: SSRC=%d Seq=%d TS=%d", packet.SSRC, packet.SequenceNumber, packet.Timestamp)
	}
}
