package rdp

// GetInput returns the Windows-specific input handler.
func GetInput() Input {
	return &RDPWindowsInput{}
}

func GetStreamer() Streamer {
	return &WindowsStreamer{}
}

func GetConfigurator() Configurator {
	return &WindowsConfigurator{}
}
