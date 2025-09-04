package rdp

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/pion/webrtc/v3"
)

type RDPAudioVideo struct {
	cancelRTPTrackInjest context.CancelFunc
	streamWaitGroup      sync.WaitGroup
	streamsMutex         sync.Mutex
	isClosing            bool
	config               *StreamConfig
	streamer             Streamer
}

func (video *RDPAudioVideo) Init(config *StreamConfig, streamer Streamer) {
	video.config = config
	video.streamer = streamer

}

func (video *RDPAudioVideo) AttachMediaChannel(PeerConnection *webrtc.PeerConnection) {
	video.streamsMutex.Lock()
	defer video.streamsMutex.Unlock()

	if video.cancelRTPTrackInjest != nil {
		log.Printf("WARNING: Closing RTP Injest Loops before attaching to media, call close before this!\n")
		video.Close()
	}

	video.isClosing = false

	gst.Init(nil)
	// start video
	video.streamer.Start(video.config)

	// Create tracks
	videoTransceiver, err := PeerConnection.AddTransceiverFromKind(
		webrtc.RTPCodecTypeVideo,
	)
	if err != nil {
		panic(err)
	}
	audioTransceiver, err := PeerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	if err != nil {
		panic(err)
	}

	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, Channels: 0, SDPFmtpLine: "packetization-mode=1"},
		"video",
		"rdp-video",
	)
	if err != nil {
		log.Fatal("Failed to create video track")
		panic(err)
	}

	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"audio",
		"rdp-audio",
	)
	if err != nil {
		panic(err)
	}

	// Add tracks
	if videoTransceiver.Sender() != nil {
		videoTransceiver.Sender().ReplaceTrack(videoTrack)
	}
	if audioTransceiver.Sender() != nil {
		audioTransceiver.Sender().ReplaceTrack(audioTrack)
	}

	// RTP Loop to injest the RTP frames
	ctx, cancel := context.WithCancel(context.Background())
	video.cancelRTPTrackInjest = cancel

	log.Printf("Starting Media Stream")
	log.Println("RTP Stream Started")

	video.streamWaitGroup.Add(2)
	go video.receiveRTPAndForward(ctx, "127.0.0.1:50045", audioTrack, video.config)
	go video.receiveRTPAndForward(ctx, "127.0.0.1:50055", videoTrack, video.config)
}

func (video *RDPAudioVideo) receiveRTPAndForward(ctx context.Context, listenAddr string, track *webrtc.TrackLocalStaticRTP, config *StreamConfig) {
	defer video.streamWaitGroup.Done()

	conn, err := net.ListenPacket("udp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to bind to UDP: %v", err)
	}
	defer conn.Close()

	log.Printf("Listening for RTP on %s\n", listenAddr)

	// Create a channel to signal when we should stop
	done := make(chan struct{})
	if udpConn, ok := conn.(*net.UDPConn); ok {
		/*
		   seriously don't touch the buffer size, any lower and Windows
		   connections will start dropping almost every frame, any higher and
		   latency jumps by 200ms+, and it's impossible to tell that it's this
		   var
		*/
		udpConn.SetReadBuffer(1024 * 1024 * 1.5)
		udpConn.SetWriteBuffer(1024 * 1024 * 1.5)
	}

	// Goroutine to handle context cancellation
	go func() {
		<-ctx.Done()
		close(done)
		// Close the connection to unblock ReadFrom
		conn.Close()
	}()
	/* THIS IS NOT A BUG, the server really only uses a 1.5 kib buffer for
	everything but for some reason without the UDP buffer being much bigger it
	starts dropping packets, zero clue why this doesn't happen on Linux */
	//buf := make([]byte, 1500) // 1.5 kib buffer
	//var nextSendTime time.Time
	// optimize for around 1ms (1000 microSecond) latency
	//minPacketInterval := time.Duration(1000/((config.bitrate)/((config.mtu*8)/1000))) * time.Microsecond
	// set packet smoothing 1 microsecond for every 5 mbit
	//minPacketInterval := time.Duration(100*(config.bitrate/5000)) * time.Microsecond
	//log.Printf("Selected pacing of %v\n", minPacketInterval)
	// User-space jitter buffer
	const queueSize = 512
	rtpQueue := make(chan []byte, queueSize)

	// Goroutine: read from UDP and push into queue
	go func() {
		buf := make([]byte, 1500)
		for {
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("UDP read error: %v", err)
					continue
				}
			}

			pktCopy := make([]byte, n)
			copy(pktCopy, buf[:n])

			select {
			case rtpQueue <- pktCopy:
			default:
				// Queue full: drop oldest packet to avoid blocking
				<-rtpQueue
				rtpQueue <- pktCopy
			}
		}
	}()

	// Forward loop: batch packets every 1?2ms
	ticker := time.NewTicker(time.Duration(1000/config.framerate) * time.Millisecond)
	defer ticker.Stop()

	var batch [][]byte

	for {
		select {
		case <-ctx.Done():
			log.Printf("Shutting down RTP ingest for %s\n", track.StreamID())
			return
		case pkt := <-rtpQueue:
			batch = append(batch, pkt)
		case <-ticker.C:
			if len(batch) == 0 {
				continue
			}

			for _, pkt := range batch {
				if _, err := track.Write(pkt); err != nil {
					log.Printf("Failed to write RTP to track: %v", err)
				}
			}
			batch = batch[:0] // clear batch
		}
	}

}

func (video *RDPAudioVideo) Close() {
	video.streamsMutex.Lock()
	defer video.streamsMutex.Unlock()

	if video.cancelRTPTrackInjest != nil {
		log.Printf("Closing RTP Injest Loops\n")
		video.isClosing = true
		video.cancelRTPTrackInjest()

		// Wait for all goroutines to finish with a timeout
		done := make(chan struct{})
		go func() {
			video.streamWaitGroup.Wait()
			close(done)
		}()

		select {
		case <-done:
			log.Printf("All RTP streams closed successfully\n")
		case <-time.After(2 * time.Second):
			log.Printf("Timeout waiting for RTP streams to close\n")
		}
	}

	// Wait for all goroutines to finish with a timeout
	done := make(chan struct{})
	go func() {
		video.streamWaitGroup.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("All Video streams closed successfully\n")
	case <-time.After(2 * time.Second):
		log.Printf("Timeout waiting for Video streams to close\n")
	}

	video.cancelRTPTrackInjest = nil
	video.isClosing = false
}
