package rdp

import (
	"log"
	"net/http"
)

type RDPWhipEndpoint struct{}

func (endpoint *RDPWhipEndpoint) StartWebServer() {
	log.Fatal((http.ListenAndServe(":8080", nil)))
}
