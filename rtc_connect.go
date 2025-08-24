package rdp

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"github.com/zieckey/goini"
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

func (connector *RDPWebRTCConnect) getParamsFromConfig() ([]string, error) {
	// TODO OS Selection
	ini := goini.New()
	err := ini.ParseFile("C:\\ProgramData\\e8rd\\config.ini")
	if err != nil {
		log.Printf("Config Parse Error")
		return []string{}, err
	}
	// todo error checking
	url, ok := ini.SectionGet("Server", "url")
	if !ok {
		log.Printf("Invalid API URL Parameter")
		return []string{}, fmt.Errorf("could not find api url in config")
	}
	hostname, ok := ini.SectionGet("Server", "name")
	if !ok {
		log.Printf("Invalid API URL Parameter")
		return []string{}, fmt.Errorf("could not find hostname in config")
	}
	token, ok := ini.SectionGet("Server", "token")
	if !ok {
		log.Printf("Invalid API token Parameter")
		return []string{}, fmt.Errorf("could not find hostname in config")
	}

	return []string{url, hostname, token}, nil
}

// Also handles the socket connection
func (connector *RDPWebRTCConnect) Start() {
	// Create the peer connection

	configOptions, err := connector.getParamsFromConfig()
	if err != nil {
		log.Printf("Could not find API Url")
		return
	}

	apiURL := configOptions[0]
	my_id := configOptions[1]
	token := configOptions[2]

	header := http.Header{}
	header.Set("Cookie", fmt.Sprintf("user-session=%s", token))
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	conn, _, err := dialer.Dial(fmt.Sprintf("wss://%s/ws?id=%s&type=machine", apiURL, my_id), header)
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

		_, msgBytes, err := conn.ReadMessage()
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
				log.Printf("Could not send SDP Answer %v", err)
				continue
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

			if signalmsg.Candidate != nil && signalmsg.Candidate.Candidate != "" {
				err := peerConnection.AddICECandidate(*signalmsg.Candidate)
				if err != nil {
					log.Println("Error adding ICE candidate:", err)
				}
			}
		}
	}

}
