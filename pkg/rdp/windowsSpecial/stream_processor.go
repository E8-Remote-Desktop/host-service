package windowsspecial

import "github.com/e8-remote-desktop/host-service/pkg/rdp"

type WindowsStreamProcessor struct {
	pipeHandler WindowsCommunicator
	isStarted   bool
}

func (processor *WindowsStreamProcessor) Init(config *rdp.StreamConfig) error {
	processor.pipeHandler = &WindowsNamedPipeCommunicator{GenericName: "stream"}
	processor.pipeHandler.Init()
	return nil
}
func (processor *WindowsStreamProcessor) Start() error {

	if err := processor.pipeHandler.Start(); err != nil {
		return err
	}
	processor.isStarted = true
	return nil

}
func (processor *WindowsStreamProcessor) Close() error {
	processor.isStarted = false
	return processor.pipeHandler.Close()
}
func (processor *WindowsStreamProcessor) IsStarted() bool {
	return processor.isStarted
}

func (processor *WindowsStreamProcessor) YouAreClosedTrustMe() {
	processor.isStarted = false
}
