package rdp

import "net/http"

type RDPWebServer struct{}

func (server *RDPWebServer) StartWebRTC(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

}

func (server *RDPWebServer) Start() {
	http.HandleFunc("/")
}
