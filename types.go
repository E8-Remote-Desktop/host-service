package rdp

type Input interface {
	AcceptDataChannel()
}

type AudioVideo interface {
	AttachMediaChannel()
}

type WebRTCConnect interface {
	Start()
}
