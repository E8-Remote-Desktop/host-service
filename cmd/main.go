package main

import "github.com/e8-remote-desktop/host-service/pkg/rdp"

var rdpServer rdp.WebRTCConnect = &rdp.RDPWebRTCConnect{}

func main() {
	rdpServer.Start()

}
