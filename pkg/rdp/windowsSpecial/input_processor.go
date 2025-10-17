package windowsspecial

import "github.com/e8-remote-desktop/host-service/pkg/rdp"

type WindowsInputProcessor struct {
	pipeHandler WindowsCommunicator
	isStarted   bool
}

func (processor *WindowsInputProcessor) Init(config *rdp.StreamConfig) error {
	processor.pipeHandler = &WindowsNamedPipeCommunicator{GenericName: "input"}
	processor.pipeHandler.Init()
	return nil
}
func (processor *WindowsInputProcessor) Start() error {

	if err := processor.pipeHandler.Start(); err != nil {
		return err
	}
	processor.isStarted = true
	return nil

}
func (processor *WindowsInputProcessor) Send(data []byte) error {
	return processor.pipeHandler.Send(data)
}
func (processor *WindowsInputProcessor) Close() error {
	processor.isStarted = false
	return processor.pipeHandler.Close()
}
func (processor *WindowsInputProcessor) IsStarted() bool {
	return processor.isStarted
}

func (processor *WindowsInputProcessor) YouAreClosedTrustMe() {
	processor.isStarted = false
}
