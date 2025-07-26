package rdp

type Input interface {
	StartDataChannel()
}

type Video interface {
	IngestWhip()
	StartMediaChannel()
}

type WhipEndpoint interface {
	StartWebServer()
}
