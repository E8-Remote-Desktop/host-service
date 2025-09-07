package e8protocol

import (
	rdp "github.com/e8-remote-desktop/host-service/pkg/rdp"
	windowsspecial "github.com/e8-remote-desktop/host-service/pkg/rdp/windowsSpecial"
)

// GetInput returns the Windows-specific input handler.
func GetInput() rdp.Input {
	return &windowsspecial.RDPWindowsInput{}
}

func GetStreamer() rdp.MediaStreamer {
	return &rdp.WindowsMediaStreamer{}
}

func GetConfigurator() rdp.Configurator {
	return &rdp.WindowsConfigurator{}
}

func GetOSHelper() rdp.OSHelper {
	return &rdp.OSHelper
}
