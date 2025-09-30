package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/e8-remote-desktop/host-service/pkg/rdp"
	"github.com/e8-remote-desktop/host-service/pkg/rdp/e8protocol"
	winInputHelper "github.com/e8-remote-desktop/host-service/pkg/rdp/windowsSpecial/helpers/input"
	winStreamHelper "github.com/e8-remote-desktop/host-service/pkg/rdp/windowsSpecial/helpers/stream"
)

var rdpServer rdp.RTCInitalizer = &e8protocol.RDPWebRTCConnect{}

func redirectStdout(logName string) {
	logPath := filepath.Join(`C:\`, logName+".log")

	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}

	// Replace stdout with file
	os.Stdout = f
	os.Stderr = f
}

func main() {
	inputFlag := flag.Bool("inputhelper", false, "Run input helper")
	streamFlag := flag.Bool("streamhelper", false, "Run stream helper")
	flag.Parse()

	switch {
	case *inputFlag:
		redirectStdout("input")
		fmt.Println("Running Input Helper...")
		winInputHelper.Main()

	case *streamFlag:
		redirectStdout("stream")
		fmt.Println("Running Stream Helper...")
		winStreamHelper.Main()

	default:
		redirectStdout("full")
		fmt.Println("Running full RDP server...")
		rdpServer.Start()
	}
}
