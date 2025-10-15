package helpers

type WindowsExternalHelper interface {
	Init(string)
	Main()
	connectToCommunicator()
	listenForCommands()
	msgRouter()
}

type WindowsHelperProcessor interface (
	Start() error
	msgProcessor([]byte)
	Close() error
	RegisterMsgSender(func([]byte) error)
)
