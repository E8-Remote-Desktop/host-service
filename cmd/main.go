package main

import "github.com/rahulc07/rdp/host-service/pkg/rdp"

var rdpServer rdp.WebRTCConnect = &rdp.RDPWebRTCConnect{}

func main() {
	rdpServer.Start()

}
