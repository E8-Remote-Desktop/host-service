//go:build windows
// +build windows

package helpers

import (
	"context"
	"fmt"
	"log"
	"runtime"

	"github.com/e8-remote-desktop/host-service/pkg/rdp"
	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
)

type WindowsStreamHelper struct {
	mainLoop       *glib.MainLoop
	mainLoopCancel context.CancelFunc
	pipelines      []*gst.Pipeline
	isClosing      bool
}

func (streamer *WindowsStreamHelper) buildGstVideoPipeline(config *rdp.StreamConfig) string {
	// TODO OS Based Pipeline Switching
	rtpCodec := config.Codec
	if config.Codec == "hevc" {
		rtpCodec = "h265"
	}
	pipeline := ""
	switch config.Encoder {
	case "intel":
		pipeline = fmt.Sprintf("d3d11screencapturesrc monitor-index=%d show-cursor=true ", config.Screen) +
			"! d3d11convert" +
			fmt.Sprintf("! video/x-raw(memory:D3D11Memory),framerate=%d/1,format=NV12 ", config.Framerate) +
			fmt.Sprintf("! qsv%senc rate-control=cbr bitrate=%d gop-size=%d low-latency=true target-usage=7 rc-lookahead=0 ", config.Codec, config.Bitrate, config.Gopsize) +
			"! queue max-size-buffers=2 max-size-time=0 max-size-bytes=0 leaky=downstream " +
			fmt.Sprintf("! video/x-%s,stream-format=byte-stream,alignment=au ", config.Codec) +
			fmt.Sprintf("! rtp%spay config-interval=0 pt=96 aggregate-mode=zero-latency mtu=%d", rtpCodec, config.MTU) +
			"! udpsink host=127.0.0.1 port=50055 sync=false async=false "

		log.Println(pipeline)

	case "amd":
		// usage=2 because in newer AMD drivers usage=1 for ultra low latency doesn't seem to function
		pipeline = fmt.Sprintf("d3d11screencapturesrc monitor-index=%d show-cursor=true ", config.Screen) +
			"! d3d11convert " +
			fmt.Sprintf("! video/x-raw(memory:D3D11Memory),framerate=%d/1,format=NV12 ", config.Framerate) +
			fmt.Sprintf("! amf%senc rate-control=cbr bitrate=%d gop-size=%d usage=2 ", config.Codec, config.Bitrate, config.Gopsize) +
			"! queue max-size-buffers=1 max-size-time=0 max-size-bytes=0 leaky=downstream " +
			fmt.Sprintf("! video/x-%s,stream-format=byte-stream,alignment=au ", config.Codec) +
			fmt.Sprintf("! rtp%spay config-interval=1 pt=96 aggregate-mode=zero-latency mtu=%d ", rtpCodec, config.MTU) +
			"! udpsink host=127.0.0.1 port=50055 sync=false async=false "
		log.Println(pipeline)
	}
	return pipeline
}

func (streamer *WindowsStreamHelper) buildGstAudioPipeline(config *rdp.StreamConfig) string {
	// TODO OS Based Selection
	return "wasapisrc loopback=true low-latency=true " +
		"! audioresample " +
		"! audio/x-raw,rate=48000,channels=2 ! audioconvert " +
		"! opusenc bitrate=128000 frame-size=10 " +
		"! rtpopuspay pt=97 " +
		"! udpsink host=127.0.0.1 port=50045 sync=false async=false "
}

func (streamer *WindowsStreamHelper) StartStream(config *rdp.StreamConfig, pipelineFactory func(*rdp.StreamConfig) string) error {

	pipelineString := pipelineFactory(config)

	pipeline, err := gst.NewPipelineFromString(pipelineString)
	if err != nil {
		return err
	}

	streamer.pipelines = append(streamer.pipelines, pipeline)

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
			streamer.mainLoop.Quit()
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

func (streamer *WindowsStreamHelper) Cancel() {
	if streamer.pipelines == nil {
		return
	}
	log.Printf("Closing GST Streams\n")
	streamer.isClosing = true
	streamer.mainLoopCancel()
	streamer.mainLoopCancel = nil

	for _, p := range streamer.pipelines {
		// the go gc doesn't seem to want to unref this itself, which makes gst mad
		// tell go that we'll handle this ourselves
		// todo maybe use AddCleanup to tell Go how to handle this?
		if err := p.SetState(gst.StateNull); err != nil {
			log.Printf("ERROR COULD NOT STOP PIPELINE FROM PLAYING STATE, %v!", err)
		}
		// stop before unref
		runtime.SetFinalizer(p, nil)
		p.Unref()
	}

	streamer.isClosing = false
}

func (streamer *WindowsStreamHelper) StartStreaming(config *rdp.StreamConfig) error {
	ctx, streamCancel := context.WithCancel(context.Background())

	streamer.mainLoopCancel = streamCancel
	streamer.mainLoop = glib.NewMainLoop(glib.MainContextDefault(), false)

	if err := streamer.StartStream(config, streamer.buildGstVideoPipeline); err != nil {
		log.Printf("Could not start video stream, %v", err)
		return err
	}

	// start audio
	if err := streamer.StartStream(config, streamer.buildGstAudioPipeline); err != nil {
		log.Printf("Could not start audio stream %v", err)
		return err
	}
	go func() {
		// Watch context cancellation
		go func() {
			<-ctx.Done()
			log.Println("Context canceled: stopping main loop")
			streamer.mainLoop.Quit()
		}()

		// Run the GLib main loop
		log.Println("Starting GStreamer main loop")
		streamer.mainLoop.Run()
		log.Println("GStreamer main loop stopped")
	}()

	return nil
}
