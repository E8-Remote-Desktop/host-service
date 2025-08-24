package rdp

import (
	"context"
	"fmt"
	"log"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	"github.com/pion/webrtc/v3"
	"github.com/zieckey/goini"
)

type RDPAudioVideo struct {
	mainLoop       *glib.MainLoop
	mainLoopCancel context.CancelFunc

	pipelines            []*gst.Pipeline
	cancelRTPTrackInjest context.CancelFunc
	streamWaitGroup      sync.WaitGroup
	streamsMutex         sync.Mutex
	isClosing            bool
}

type StreamConfig struct {
	os        string
	codec     string //h264 or hevc
	encoder   string
	bitrate   int
	gopsize   int
	screen    int
	framerate int
	mtu       int
}

func (video *RDPAudioVideo) buildGstVideoPipeline(config *StreamConfig) string {
	// TODO OS Based Pipeline Switching
	rtpCodec := config.codec
	if config.codec == "hevc" {
		rtpCodec = "h265"
	}
	pipeline := ""
	switch config.encoder {
	case "intel":
		pipeline = fmt.Sprintf("d3d11screencapturesrc monitor-index=%d show-cursor=true ", config.screen) +
			"! d3d11convert" +
			fmt.Sprintf("! video/x-raw(memory:D3D11Memory),framerate=%d/1,format=NV12 ", config.framerate) +
			fmt.Sprintf("! qsv%senc rate-control=cbr bitrate=%d gop-size=%d low-latency=true target-usage=7 rc-lookahead=0 ", config.codec, config.bitrate, config.gopsize) +
			"! queue max-size-buffers=1 max-size-time=0 max-size-bytes=0 leaky=downstream " +
			fmt.Sprintf("! video/x-%s,stream-format=byte-stream,alignment=au ", config.codec) +
			fmt.Sprintf("! rtp%spay config-interval=0 pt=96 aggregate-mode=zero-latency mtu=%d", rtpCodec, config.mtu) +
			"! udpsink host=127.0.0.1 port=50055 sync=false async=false "

		log.Println(pipeline)

	case "amd":
		// usage=2 because in newer AMD drivers usage=1 for ultra low latency doesn't seem to function
		pipeline = fmt.Sprintf("d3d11screencapturesrc monitor-index=%d show-cursor=true ", config.screen) +
			"! d3d11convert " +
			fmt.Sprintf("! video/x-raw(memory:D3D11Memory),framerate=%d/1,format=NV12 ", config.framerate) +
			fmt.Sprintf("! amf%senc rate-control=cbr bitrate=%d gop-size=%d usage=2 ", config.codec, config.bitrate, config.gopsize) +
			"! queue max-size-buffers=1 max-size-time=0 max-size-bytes=0 leaky=downstream " +
			fmt.Sprintf("! video/x-%s,stream-format=byte-stream,alignment=au ", config.codec) +
			fmt.Sprintf("! rtp%spay config-interval=0 pt=96 aggregate-mode=zero-latency mtu=%d ", rtpCodec, config.mtu) +
			"! udpsink host=127.0.0.1 port=50055 sync=false async=false "
		log.Println(pipeline)
	}
	return pipeline
}

func (video *RDPAudioVideo) buildGstAudioPipeline(config *StreamConfig) string {
	// TODO OS Based Selection
	return "wasapisrc loopback=true low-latency=true " +
		"! audioresample " +
		"! audio/x-raw,rate=48000,channels=2 ! audioconvert " +
		"! opusenc bitrate=128000 frame-size=10 " +
		"! rtpopuspay pt=97 " +
		"! udpsink host=127.0.0.1 port=50045 sync=false async=false "
}

func (video *RDPAudioVideo) StartStream(config *StreamConfig, pipelineFactory func(*StreamConfig) string) error {

	pipelineString := pipelineFactory(config)

	pipeline, err := gst.NewPipelineFromString(pipelineString)
	if err != nil {
		return err
	}

	video.pipelines = append(video.pipelines, pipeline)

	pipeline.GetPipelineBus().AddWatch(func(msg *gst.Message) bool {
		switch msg.Type() {
		// why are both -34? bug?
		//case gst.MessageEOS:
		//pipeline.BlockSetState(gst.StateNull)
		//mainLoop.Quit()
		case gst.MessageError:
			err := msg.ParseError()
			fmt.Println("ERROR:", err.Error())
			if debug := err.DebugString(); debug != "" {
				fmt.Println("DEBUG:", debug)
			}
			video.mainLoop.Quit()
		default:
			// All messages implement a Stringer. However, this is
			// typically an expensive thing to do and should be avoided.
			fmt.Println(msg)
		}
		return true
	})

	if err := pipeline.SetState(gst.StatePlaying); err != nil {
		log.Printf("Could not start stream for %s, err: %v", pipelineString, err)
	}
	log.Printf("Readied pipeline %s", pipelineString)
	return nil
}

func (video *RDPAudioVideo) parseConfig() (*StreamConfig, error) {
	// TODO OS Selection
	ini := goini.New()
	err := ini.ParseFile("C:\\ProgramData\\e8rd\\config.ini")
	if err != nil {
		log.Printf("Config Parse Error")
		return &StreamConfig{}, err
	}
	// todo error checking
	config := &StreamConfig{}
	ok := false
	config.os, ok = ini.SectionGet("OS", "os")
	if !ok {
		log.Printf("Invalid OS Parameter")
	}
	config.codec, ok = ini.SectionGet("Encoding", "codec")
	if !ok {
		log.Printf("Invalid Codec Parameter")
	}
	config.encoder, ok = ini.SectionGet("Encoding", "encoder")
	if !ok {
		log.Printf("Invalid Encoder Parameter")
	}
	config.bitrate, ok = ini.SectionGetInt("Encoding", "bitrate")
	if !ok {
		log.Printf("Invalid Bitrate Parameter")
	}
	config.gopsize, ok = ini.SectionGetInt("Encoding", "gopsize")
	if !ok {
		log.Printf("Invalid GOP Size Parameter")
	}
	config.screen, ok = ini.SectionGetInt("Capture", "screen")
	if !ok {
		log.Printf("Invalid Screen Index Parameter")
	}
	config.framerate, ok = ini.SectionGetInt("Capture", "framerate")
	if !ok {
		log.Printf("Invalid Framerate Parameter")
	}
	config.mtu, ok = ini.SectionGetInt("Stream", "mtu")
	if !ok {
		log.Printf("Invalid MTU Parameter")
	}
	return config, nil
}

func (video *RDPAudioVideo) AttachMediaChannel(PeerConnection *webrtc.PeerConnection) {
	video.streamsMutex.Lock()
	defer video.streamsMutex.Unlock()

	if video.cancelRTPTrackInjest != nil {
		log.Printf("WARNING: Closing RTP Injest Loops before attaching to media, call close before this!\n")
		video.Close()
	}

	video.isClosing = false

	config, err := video.parseConfig()
	if err != nil {
		log.Printf("Error parsing config")
		return
	}

	gst.Init(nil)
	ctx, streamCancel := context.WithCancel(context.Background())
	video.mainLoopCancel = streamCancel
	video.mainLoop = glib.NewMainLoop(glib.MainContextDefault(), false)
	// start video
	if err := video.StartStream(config, video.buildGstVideoPipeline); err != nil {
		log.Printf("Could not start video stream, %v", err)
	}

	// start audio
	if err := video.StartStream(config, video.buildGstAudioPipeline); err != nil {
		log.Printf("Could not start audio stream %v", err)
	}

	go func() {
		// Watch context cancellation
		go func() {
			<-ctx.Done()
			log.Println("Context canceled: stopping main loop")
			video.mainLoop.Quit()
		}()

		// Run the GLib main loop
		log.Println("Starting GStreamer main loop")
		video.mainLoop.Run()
		log.Println("GStreamer main loop stopped")
	}()

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
		/*
		   seriously don't touch the buffer size, any lower and Windows
		   connections will start dropping almost every frame, any higher and
		   latency jumps by 200ms+, and it's impossible to tell that it's this
		   var
		*/
		udpConn.SetReadBuffer(1024 * 1024 * 4)
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
	buf := make([]byte, 1500) // 1.5 kib buffer

	for {
		select {
		case <-done:
			log.Printf("Shutting down RTP ingest for %s\n", track.StreamID())
			return
		default:
			// no more deadlines increases latency by 5-6ms
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

	if video.mainLoopCancel != nil {
		for _, p := range video.pipelines {
			if err := p.SetState(gst.StateNull); err != nil {
				log.Printf("ERROR COULD NOT STOP PIPELINE FROM PLAYING STATE, %v!", err)
			} // stop before unref
		}
		video.pipelines = nil
		log.Printf("Closing GST Streams\n")
		video.isClosing = true
		video.mainLoopCancel()

		for _, p := range video.pipelines {
			// the go gc doesn't seem to want to unref this itself, which makes gst mad
			// tell go that we'll handle this ourselves
			// todo maybe use AddCleanup to tell Go how to handle this?
			runtime.SetFinalizer(p, nil)
			p.Unref()
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

	}

	video.cancelRTPTrackInjest = nil
	video.isClosing = false
	video.mainLoopCancel = nil
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

	if video.mainLoopCancel != nil {
		for _, p := range video.pipelines {
			if err := p.SetState(gst.StateNull); err != nil {
				log.Printf("ERROR COULD NOT STOP PIPELINE FROM PLAYING STATE, %v!", err)
			} // stop before unref
		}
		video.pipelines = nil
		log.Printf("Closing GST Streams\n")
		video.isClosing = true
		video.mainLoopCancel()

		for _, p := range video.pipelines {
			// the go gc doesn't seem to want to unref this itself, which makes gst mad
			// tell go that we'll handle this ourselves
			// todo maybe use AddCleanup to tell Go how to handle this?
			runtime.SetFinalizer(p, nil)
			p.Unref()
		}

		done := make(chan struct{})
		go func() {
			video.streamWaitGroup.Wait()
			close(done)
		}()

		select {
		case <-done:
			log.Printf("All GST streams closed successfully\n")
		case <-time.After(timeout):
			log.Printf("Timeout waiting for GST streams to close\n")
			return context.DeadlineExceeded
		}
	}

	video.cancelRTPTrackInjest = nil
	video.mainLoopCancel = nil
	video.isClosing = false
	return nil
}
