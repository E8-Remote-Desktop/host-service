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

	var captureStream AudioVideo = &RDPAudioVideo{}
	var pendingCandidates []*webrtc.ICECandidateInit
	for {
		_, msgBytes, err := conn.ReadMessage()
		//log.Println("Recieved message?")
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
			log.Println("Received SDP offer")
			if peerConnection != nil {
				log.Println("Closing old PeerConnection before accepting new offer")
				peerConnection.Close()
				peerConnection = nil
			}

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

			// Data channel accept (client opens the input data channel on the browser side)
			var input Input = &RDPInput{PeerConnection: peerConnection}
			input.AcceptDataChannel()

			// Attach media channel
			captureStream.AttachMediaChannel(peerConnection)

			// Set remote offer
			offer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  msg.SDP,
			}

			err = peerConnection.SetRemoteDescription(offer)
			if err != nil {
				log.Fatal(err)
			}

			for _, candidate := range pendingCandidates {
				if err := peerConnection.AddICECandidate(*candidate); err != nil {
					log.Println("Error adding pending ICE candidate:", err)
				}
			}
			pendingCandidates = nil

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
			err = conn.WriteMessage(websocket.TextMessage, answerJSON)
			if err != nil {
				log.Fatalf("Could not send SDP Answer %v", err)
			}

			log.Printf("Sent SDP answer %s", string(answerJSON))

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

		case "ice":
			log.Printf("Received ICE candidate %s\n", msg.Candidate.Candidate)
			if peerConnection == nil {
				log.Println("Missed packets?! ICE Candidates recieved before offer, queuing")
				pendingCandidates = append(pendingCandidates, msg.Candidate)
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
