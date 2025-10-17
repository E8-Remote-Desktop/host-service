package helpers

import (
	"net"

	"github.com/e8-remote-desktop/host-service/pkg/rdp"
)

type WindowsExternalHelper interface {
	Register(string, WindowsHelperProcessor)
	Run()
	connectToCommunicator()
	listenForCommands()
	msgRouter([]byte)
}

type WindowsHelperProcessor interface {
	Start(*rdp.StreamConfig) error
	MsgProcessor([]byte)
	Close() error
	RegisterMsgSender(net.Conn)
}
