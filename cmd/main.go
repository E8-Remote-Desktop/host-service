package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/e8-remote-desktop/host-service/pkg/rdp"
	"github.com/e8-remote-desktop/host-service/pkg/rdp/e8protocol"
	winInputHelper "github.com/e8-remote-desktop/host-service/pkg/rdp/windowsSpecial/helpers/input"
	winStreamHelper "github.com/e8-remote-desktop/host-service/pkg/rdp/windowsSpecial/helpers/stream"
)

var rdpServer rdp.RTCInitalizer = &e8protocol.RDPWebRTCConnect{}

func redirectStdout(fileOut string) func() {
	// from https://gist.github.com/jerblack/4b98ba48ed3fb1d9f7544d2b1a1be287
	logfile := fmt.Sprintf(`C:\%s.log`, fileOut)

	// open file read/write | create if not exist | clear file at open if exists
	f, _ := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)

	// save existing stdout | MultiWriter writes to saved stdout and file
	out := os.Stdout
	mw := io.MultiWriter(out, f)

	// get pipe reader and writer | writes to pipe writer come out pipe reader
	r, w, _ := os.Pipe()

	// replace stdout,stderr with pipe writer | all writes to stdout, stderr will go through pipe instead (fmt.print, log)
	os.Stdout = w
	os.Stderr = w

	// writes with log.Print should also write to mw
	log.SetOutput(mw)

	//create channel to control exit | will block until all copies are finished
	exit := make(chan bool)

	go func() {
		// copy all reads from pipe to multiwriter, which writes to stdout and file
		_, _ = io.Copy(mw, r)
		// when r or w is closed copy will finish and true will be sent to channel
		exit <- true
	}()

	// function to be deferred in main until program exits
	return func() {
		// close writer then block on exit channel | this will let mw finish writing before the program exits
		_ = w.Close()
		<-exit
		// close file after all writes have finished
		_ = f.Close()
	}

}

func main() {
	inputFlag := flag.Bool("inputhelper", false, "Run input helper")
	streamFlag := flag.Bool("streamhelper", false, "Run stream helper")
	flag.Parse()

	switch {
	case *inputFlag:
		closeFunc := redirectStdout("input")
		defer closeFunc()
		fmt.Println("Running Input Helper...")
		winInputHelper.Main()

	case *streamFlag:
		closeFunc := redirectStdout("stream")
		defer closeFunc()
		fmt.Println("Running Stream Helper...")
		winStreamHelper.Main()

	default:
		closeFunc := redirectStdout("full")
		defer closeFunc()
		fmt.Println("Running full RDP server...")
		rdpServer.Start()
	}
}
