package rdp

import (
	"log"
	"net/url"

	"github.com/gorilla/websocket"
)

type RDPWebRTCConnect struct{}

// Also handles the socket connection
func (connector *RDPWebRTCConnect) Start() {
	// Create the peer connection
	u := url.URL{Scheme: "ws", Host: "127.0.0.1:8080", Path: "/"}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("Error connecting to websocket server")
	}
	defer conn.Close()

}
