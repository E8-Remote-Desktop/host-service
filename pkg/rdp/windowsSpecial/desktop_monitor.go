package windowsspecial

type DesktopMonitor struct {
	pipeHandler WindowsCommunicator
	isStarted   bool
	initalized  bool
}

func (processor *DesktopMonitor) IsStarted() bool {
	return processor.isStarted
}

func (processor *DesktopMonitor) Init() error {
	processor.pipeHandler = &WindowsNamedPipeCommunicator{GenericName: "desktop"}
	processor.pipeHandler.Init()
	processor.initalized = true
	return nil
}

func (processor *DesktopMonitor) Start() error {
	// this is non-standard so it has to handle it's own initalization
	if !processor.initalized {
		processor.Init()
	}

	if err := processor.pipeHandler.Start(); err != nil {
		return err
	}
	processor.isStarted = true
	return nil

}

func (processor *DesktopMonitor) GetRecieveChannel() <-chan []byte {
	return processor.pipeHandler.Recieve()
}

func (processor *DesktopMonitor) YouAreClosedTrustMe() {
	processor.isStarted = false
}
func (processor *DesktopMonitor) Close() error {
	if !processor.isStarted || !processor.initalized {
		return nil
	}
	processor.isStarted = false
	return processor.pipeHandler.Close()
}
