package rdp

import (
	"encoding/json"
	"log"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type SignalMessage struct {
	Type      string                   `json:"type"` // "offer", "answer", "ice"
	SDP       string                   `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
}

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

	log.Println("Host has connected to broker websocket, waiting for client connection...")

	var peerConnection *webrtc.PeerConnection
	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			log.Fatal("Error reading WebSocket:", err)
		}

		var msg SignalMessage

		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			log.Println("Invalid message:", err)
			continue
		}

		switch msg.Type {
		case "offer":
			// Create peer connection
			peerConnection, err = webrtc.NewPeerConnection(webrtc.Configuration{
				ICEServers: []webrtc.ICEServer{
					{
						URLs: []string{"stun:stun.l.google.com:19302"},
					},
				},
			})

			// Accept

			if err != nil {
				log.Fatal(err)
			}

			log.Println("Received SDP offer")

			// Handle ICE candidates from this peer
			peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {

				if c == nil {
					return
				}

				candidate := c.ToJSON()

				candidateMsg := SignalMessage{
					Type:      "ice",
					Candidate: &candidate,
				}
				candidateJSON, _ := json.Marshal(candidateMsg)
				conn.WriteMessage(websocket.TextMessage, candidateJSON)
			})
			// Data channel accept (client opens the input data channel on the browser side)
			var input Input = &RDPInput{PeerConnection: peerConnection}
			input.AcceptDataChannel()

			// Attach media channel
			var captureStream AudioVideo = &RDPAudioVideo{PeerConnection: peerConnection}
			captureStream.AttachMediaChannel()

			// Set remote offer
			offer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  msg.SDP,
			}

			err = peerConnection.SetRemoteDescription(offer)
			if err != nil {
				log.Fatal(err)
			}

			// Create answer
			answer, err := peerConnection.CreateAnswer(nil)
			if err != nil {
				log.Fatal(err)
			}

			err = peerConnection.SetLocalDescription(answer)
			if err != nil {
				log.Fatal(err)
			}

			// Send answer via WebSocket
			answerMsg := SignalMessage{
				Type: "answer",
				SDP:  answer.SDP,
			}

			answerJSON, _ := json.Marshal(answerMsg)
			conn.WriteMessage(websocket.TextMessage, answerJSON)

			log.Println("Sent SDP answer")

		case "ice":
			log.Println("Received ICE candidate")
			if peerConnection == nil {
				log.Println("Missed packets?! ICE Candidates recieved before offer.")
				continue
			}

			if msg.Candidate != nil {
				err := peerConnection.AddICECandidate(*msg.Candidate)
				if err != nil {
					log.Println("Error adding ICE candidate:", err)
				}
			}
		}
	}

}
