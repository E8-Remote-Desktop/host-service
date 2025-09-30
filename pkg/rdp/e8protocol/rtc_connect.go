package e8protocol

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	rdp "github.com/e8-remote-desktop/host-service/pkg/rdp"
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
	captureStream  rdp.RTCStreamConnector
	input          rdp.RTCInputConnector
	oshelper       rdp.OSHelper
}

// Also handles the socket connection
func (rtcInitalizer *RDPWebRTCConnect) Start() {

	// DI
	rtcInitalizer.captureStream = &RDPStreamConnector{}
	rtcInitalizer.input = &RDPInputConnector{}
	// OS Specific Factories
	rtcInitalizer.oshelper = GetOSHelper()
	var config rdp.Configurator = GetConfigurator()
	// these 2 can be local
	inputProcessor := GetInputProcessor()
	streamProcessor := GetStreamer()

	// force port
	rtcSettings := webrtc.SettingEngine{}
	rtcSettings.SetEphemeralUDPPortRange(50000, 50001)

	rtcMediaEngine := webrtc.MediaEngine{}
	if err := rtcMediaEngine.RegisterDefaultCodecs(); err != nil {
		log.Fatalf("Could not register media codecs on the engine? %v", err)
	}

	rtcAPI := webrtc.NewAPI(webrtc.WithSettingEngine(rtcSettings), webrtc.WithMediaEngine(&rtcMediaEngine))

	// init stuff
	configOptions, err := config.GetConfig()
	if err != nil {
		log.Fatalf("Could not read config")
	}
	rtcInitalizer.captureStream.Init(configOptions)
	if err := rtcInitalizer.input.Init(inputProcessor); err != nil {
		log.Fatalf("Could not init input %v\n", err)
	}
	if err := rtcInitalizer.oshelper.Init(inputProcessor, streamProcessor); err != nil {
		log.Fatalf("Could not init the oshelper")
	}

	// Create the peer connection

	apiURL := configOptions.ApiURL
	my_id := configOptions.Hostname
	token := configOptions.Token

	header := http.Header{}
	header.Set("Cookie", fmt.Sprintf("user-session=%s", token))
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	rtcInitalizer.conn, _, err = dialer.Dial(fmt.Sprintf("wss://%s/ws?id=%s&type=machine", apiURL, my_id), header)
	if err != nil {
		log.Fatalf("Error connecting to websocket server %v", err)
	}

	defer rtcInitalizer.conn.Close()

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

		_, msgBytes, err := rtcInitalizer.conn.ReadMessage()
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
			rtcInitalizer.oshelper.Close()
			rtcInitalizer.captureStream.Close()
			//rtcInitalizer.input.Close() // this handeled by oshelper now

			if rtcInitalizer.peerConnection != nil {
				log.Println("Closing old PeerConnection before accepting new offer")
				rtcInitalizer.peerConnection.Close()
				rtcInitalizer.peerConnection = nil
			}

			rtcInitalizer.peerConnection, err = rtcAPI.NewPeerConnection(webrtc.Configuration{
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

			// Attach media channel
			rtcInitalizer.oshelper.StartStreamAndInput()
			log.Printf("Media-Stream and Input Started")

			rtcInitalizer.captureStream.AttachMediaChannel(rtcInitalizer.peerConnection)
			rtcInitalizer.input.AcceptDataChannel(rtcInitalizer.peerConnection)
			log.Printf("Media/Data Channels Initalized")

			// Set remote offer
			offer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  signalmsg.SDP,
			}
			log.Printf("Trying to set remote description")
			err = rtcInitalizer.peerConnection.SetRemoteDescription(offer)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("Remote Description Set")
			log.Printf("Attempting to add Ice Candidates")
			for _, candidate := range pendingCandidates {
				if err := rtcInitalizer.peerConnection.AddICECandidate(*candidate); err != nil {
					log.Println("Error adding pending ICE candidate:", err)
				}
			}
			log.Printf("ICE Candidates Set")
			pendingCandidates = nil

			// Create answer
			answer, err := rtcInitalizer.peerConnection.CreateAnswer(nil)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("Answer Created, %s", answer.SDP)

			err = rtcInitalizer.peerConnection.SetLocalDescription(answer)
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
			err = rtcInitalizer.conn.WriteMessage(websocket.TextMessage, socketJSON)
			if err != nil {
				log.Printf("Could not send SDP Answer %v", err)
				continue
			}

			log.Printf("Sent SDP answer %s", string(socketJSON))

			// Handle ICE candidates from this peer
			rtcInitalizer.peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {

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
				rtcInitalizer.conn.WriteMessage(websocket.TextMessage, []byte(sendJSON))
			})
			// Logging
			//peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
			//if state == webrtc.ICEConnectionStateConnected {

			//log.Printf("Connection has been established, path: %s", peerConnection.)
			//}
			//})

		case "ice":
			log.Printf("Received ICE candidate %s\n", signalmsg.Candidate.Candidate)
			if rtcInitalizer.peerConnection == nil {
				log.Println("Missed packets?! ICE Candidates recieved before offer, queuing")
				pendingCandidates = append(pendingCandidates, signalmsg.Candidate)
				continue
			}

			if signalmsg.Candidate != nil && signalmsg.Candidate.Candidate != "" {
				err := rtcInitalizer.peerConnection.AddICECandidate(*signalmsg.Candidate)
				if err != nil {
					log.Println("Error adding ICE candidate:", err)
				}
			}
		}
	}

}

func (rtcInitalizer *RDPWebRTCConnect) Stop() {
	log.Println("Force stop signal detected, foricably closing")
	var err error

	// shutdown os specific things
	rtcInitalizer.oshelper.Close()
	rtcInitalizer.captureStream.Close()

	if rtcInitalizer.conn != nil {
		err = rtcInitalizer.conn.Close()
	}
	if err != nil {
		log.Printf("ERROR Could not cleanly close websocket!")
	}
	if rtcInitalizer.peerConnection != nil {
		rtcInitalizer.peerConnection.Close()
	}
	if err != nil {
		log.Printf("ERROR Could not cleanly close WebRTC connection SOMEONE COULD STILL BE CONNECTED REBOOT NOW!!")
	}

}
