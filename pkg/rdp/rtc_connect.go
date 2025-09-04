package rdp

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

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

type RDPWebRTCConnect struct {
	conn           *websocket.Conn
	peerConnection *webrtc.PeerConnection
}

// Also handles the socket connection
func (connector *RDPWebRTCConnect) Start() {
	// DI
	var captureStream AudioVideo = &RDPAudioVideo{}
	var input Input = GetInput()
	var streamer Streamer = GetStreamer()
	var config Configurator = GetConfigurator()
	// init stuff
	configOptions, err := config.GetConfig()
	captureStream.Init(configOptions, streamer)

	// Create the peer connection

	if err != nil {
		log.Printf("Could not find API Url")
		return
	}

	apiURL := configOptions.apiURL
	my_id := configOptions.hostname
	token := configOptions.token

	header := http.Header{}
	header.Set("Cookie", fmt.Sprintf("user-session=%s", token))
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	connector.conn, _, err = dialer.Dial(fmt.Sprintf("wss://%s/ws?id=%s&type=machine", apiURL, my_id), header)
	if err != nil {
		log.Fatalf("Error connecting to websocket server %v", err)
	}

	defer connector.conn.Close()

	log.Println("Host has connected to broker websocket, waiting for client connection...")
	// Setup stuff

	var pendingCandidates []*webrtc.ICECandidateInit
	//go func() {
	//ticker := time.NewTicker(30 * time.Second)
	//defer ticker.Stop()
	//for range ticker.C {
	//conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	//if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
	//log.Println("Ping failed, closing ping loop:", err)
	//return // Stops ping loop; consider reconnecting
	//}
	//}
	//}()
	for {

		_, msgBytes, err := connector.conn.ReadMessage()
		//log.Println("Recieved message?")
		if err != nil {
			log.Println("Error reading WebSocket:", err)
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

			if connector.peerConnection != nil {
				log.Println("Closing old PeerConnection before accepting new offer")
				connector.peerConnection.Close()
				connector.peerConnection = nil
			}

			connector.peerConnection, err = webrtc.NewPeerConnection(webrtc.Configuration{
				ICEServers: []webrtc.ICEServer{
					{
						URLs: []string{"stun:stun.l.google.com:19302"},
					},
				},
				ICETransportPolicy: webrtc.ICETransportPolicyAll,
				BundlePolicy:       webrtc.BundlePolicyMaxBundle,
				RTCPMuxPolicy:      webrtc.RTCPMuxPolicyRequire,
			})

			// Accept

			if err != nil {
				log.Fatal(err)
			}

			// Data channel accept (client opens the input data channel on the browser side)
			if err := input.Init(); err != nil {
				log.Fatalf("Could not init input %v\n", err)
			}
			input.AcceptDataChannel(connector.peerConnection)

			// Attach media channel
			captureStream.AttachMediaChannel(connector.peerConnection)

			// Set remote offer
			offer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  signalmsg.SDP,
			}

			err = connector.peerConnection.SetRemoteDescription(offer)
			if err != nil {
				log.Fatal(err)
			}

			for _, candidate := range pendingCandidates {
				if err := connector.peerConnection.AddICECandidate(*candidate); err != nil {
					log.Println("Error adding pending ICE candidate:", err)
				}
			}
			pendingCandidates = nil

			// Create answer
			answer, err := connector.peerConnection.CreateAnswer(nil)
			if err != nil {
				log.Fatal(err)
			}

			err = connector.peerConnection.SetLocalDescription(answer)
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
			err = connector.conn.WriteMessage(websocket.TextMessage, socketJSON)
			if err != nil {
				log.Printf("Could not send SDP Answer %v", err)
				continue
			}

			log.Printf("Sent SDP answer %s", string(socketJSON))

			// Handle ICE candidates from this peer
			connector.peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {

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
				connector.conn.WriteMessage(websocket.TextMessage, []byte(sendJSON))
			})
			// Logging
			//peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
			//if state == webrtc.ICEConnectionStateConnected {

			//log.Printf("Connection has been established, path: %s", peerConnection.)
			//}
			//})

		case "ice":
			log.Printf("Received ICE candidate %s\n", signalmsg.Candidate.Candidate)
			if connector.peerConnection == nil {
				log.Println("Missed packets?! ICE Candidates recieved before offer, queuing")
				pendingCandidates = append(pendingCandidates, signalmsg.Candidate)
				continue
			}

			if signalmsg.Candidate != nil && signalmsg.Candidate.Candidate != "" {
				err := connector.peerConnection.AddICECandidate(*signalmsg.Candidate)
				if err != nil {
					log.Println("Error adding ICE candidate:", err)
				}
			}
		}
	}

}

func (connector *RDPWebRTCConnect) Stop() {
	log.Println("Force stop signal detected, foricably closing")
	var err error
	if connector.conn != nil {
		err = connector.conn.Close()
	}
	if err != nil {
		log.Printf("ERROR Could not cleanly close websocket!")
	}
	if connector.peerConnection != nil {
		connector.peerConnection.Close()
	}
	if err != nil {
		log.Printf("ERROR Could not cleanly close WebRTC connection SOMEONE COULD STILL BE CONNECTED REBOOT NOW!!")
	}

}
