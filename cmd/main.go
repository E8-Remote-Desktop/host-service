package main

import (
	"github.com/e8-remote-desktop/host-service/pkg/rdp"
	"github.com/e8-remote-desktop/host-service/pkg/rdp/e8protocol"
)

var rdpServer rdp.RTCInitalizer = &e8protocol.RDPWebRTCConnect{}

func main() {
	rdpServer.Start()

}
