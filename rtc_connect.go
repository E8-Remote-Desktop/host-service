package rdp

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type SignalMessage struct {
	Type      string                   `json:"type"` // "offer", "answer", "ice"
	SDP       string                   `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
}

type SocketMessage struct {
	From    string          `json:"from"`
	To      string          `json:"to"`
	Content json.RawMessage `json:"content"`
}

type RDPWebRTCConnect struct{}

// Also handles the socket connection
func (connector *RDPWebRTCConnect) Start(id string, token string) {
	// Create the peer connection
	my_id := id
	header := http.Header{}
	header.Set("Cookie", fmt.Sprintf("user-session=%s", token))
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	conn, _, err := dialer.Dial(fmt.Sprintf("wss://%s/ws?id=%s&type=machine", os.Getenv("API_URL"), my_id), header)
	if err != nil {
		log.Fatalf("Error connecting to websocket server %v", err)
	}

	defer conn.Close()

	log.Println("Host has connected to broker websocket, waiting for client connection...")
	// Setup stuff

	var peerConnection *webrtc.PeerConnection

	var captureStream AudioVideo = &RDPAudioVideo{}
	var input Input = GetInput()
	var pendingCandidates []*webrtc.ICECandidateInit
	for {
		_, msgBytes, err := conn.ReadMessage()
		//log.Println("Recieved message?")
		if err != nil {
			log.Fatal("Error reading WebSocket:", err)
		}
		var msg SocketMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			log.Println("Invalid message:", err)
			continue
		}

		connecting_client := msg.From
		var signalmsg SignalMessage

		if err := json.Unmarshal([]byte(msg.Content), &signalmsg); err != nil {
			log.Println("Invalid message:", err)
			continue
		}

		switch signalmsg.Type {
		case "offer":
			// Create peer connection
			log.Println("Received SDP offer")

			// close will only actually do anything if anything can be closed
			captureStream.Close()
			input.Close()

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
			if err := input.Init(); err != nil {
				log.Fatalf("Could not init input %v\n", err)
			}
			input.AcceptDataChannel(peerConnection)

			// Attach media channel
			captureStream.AttachMediaChannel(peerConnection)

			// Set remote offer
			offer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  signalmsg.SDP,
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

			socketMsg := SocketMessage{
				From:    my_id,
				To:      connecting_client,
				Content: answerJSON,
			}
			socketJSON, _ := json.Marshal(socketMsg)
			err = conn.WriteMessage(websocket.TextMessage, socketJSON)
			if err != nil {
				log.Fatalf("Could not send SDP Answer %v", err)
			}

			log.Printf("Sent SDP answer %s", string(socketJSON))

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

				sendMessage := SocketMessage{
					From:    my_id,
					To:      connecting_client,
					Content: candidateJSON,
				}
				sendJSON, _ := json.Marshal(sendMessage)
				conn.WriteMessage(websocket.TextMessage, []byte(sendJSON))
			})
			// Logging
			//peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
			//if state == webrtc.ICEConnectionStateConnected {

			//log.Printf("Connection has been established, path: %s", peerConnection.)
			//}
			//})

		case "ice":
			log.Printf("Received ICE candidate %s\n", signalmsg.Candidate.Candidate)
			if peerConnection == nil {
				log.Println("Missed packets?! ICE Candidates recieved before offer, queuing")
				pendingCandidates = append(pendingCandidates, signalmsg.Candidate)
				continue
			}

			if signalmsg.Candidate != nil {
				err := peerConnection.AddICECandidate(*signalmsg.Candidate)
				if err != nil {
					log.Println("Error adding ICE candidate:", err)
				}
			}
		}
	}

}
