package rdp

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

type RDPAudioVideo struct {
	cancelRTPTrackInjest context.CancelFunc
	streamWaitGroup      sync.WaitGroup
	streamsMutex         sync.Mutex
	isClosing            bool
}

func (video *RDPAudioVideo) StartStream() {
}

func (video *RDPAudioVideo) AttachMediaChannel(PeerConnection *webrtc.PeerConnection) {
	video.streamsMutex.Lock()
	defer video.streamsMutex.Unlock()

	if video.cancelRTPTrackInjest != nil {
		log.Printf("WARNING: Closing RTP Injest Loops before attaching to media, call close before this!\n")
		video.Close()
	}

	video.isClosing = false
	video.StartStream()

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
	go video.receiveRTPAndForward(ctx, "127.0.0.1:50045", audioTrack)
	go video.receiveRTPAndForward(ctx, "127.0.0.1:50055", videoTrack)
}

func (video *RDPAudioVideo) receiveRTPAndForward(ctx context.Context, listenAddr string, track *webrtc.TrackLocalStaticRTP) {
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
		udpConn.SetReadBuffer(1024 * 2048) // 2048kib buffer
	}

	// Goroutine to handle context cancellation
	go func() {
		<-ctx.Done()
		close(done)
		// Close the connection to unblock ReadFrom
		conn.Close()
	}()

	buf := make([]byte, 1500) // 1.5 kib buffer

	for {
		select {
		case <-done:
			log.Printf("Shutting down RTP ingest for %s\n", track.StreamID())
			return
		default:
			// Set a shorter read deadline for more responsive cancellation
			//deadline := time.Now().Add(100 * time.Millisecond)
			//conn.SetReadDeadline(deadline)
			conn.SetReadDeadline(time.Time{})

			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					// Check if we should exit on timeout
					select {
					case <-done:
						log.Printf("Shutting down RTP ingest for %s\n", track.StreamID())
						return
					default:
						continue
					}
				}

				// Check for context cancellation on any error
				select {
				case <-done:
					log.Printf("Shutting down RTP ingest for %s\n", track.StreamID())
					return
				default:
					// If connection was closed due to cancellation, exit
					if video.isClosing {
						log.Printf("Shutting down RTP ingest for %s (connection closed)\n", track.StreamID())
						return
					}
					log.Printf("Read error: %v", err)
					continue
				}
			}

			_, writeErr := track.Write(buf[:n])
			if writeErr != nil {
				log.Printf("Failed to write RTP to track: %v", writeErr)
			}
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

	video.cancelRTPTrackInjest = nil
	video.isClosing = false
}

// CloseWithTimeout provides more control over the closing timeout
func (video *RDPAudioVideo) CloseWithTimeout(timeout time.Duration) error {
	video.streamsMutex.Lock()
	defer video.streamsMutex.Unlock()

	if video.cancelRTPTrackInjest != nil {
		log.Printf("Closing RTP Injest Loops with timeout %v\n", timeout)
		video.isClosing = true
		video.cancelRTPTrackInjest()

		done := make(chan struct{})
		go func() {
			video.streamWaitGroup.Wait()
			close(done)
		}()

		select {
		case <-done:
			log.Printf("All RTP streams closed successfully\n")
		case <-time.After(timeout):
			log.Printf("Timeout waiting for RTP streams to close\n")
			return context.DeadlineExceeded
		}
	}

	video.cancelRTPTrackInjest = nil
	video.isClosing = false
	return nil
}
