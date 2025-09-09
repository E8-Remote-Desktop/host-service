//go:build windows
// +build windows

package e8protocol

import (
	rdp "github.com/e8-remote-desktop/host-service/pkg/rdp"
	windowsspecial "github.com/e8-remote-desktop/host-service/pkg/rdp/windowsSpecial"
)

// GetInput returns the Windows-specific input handler.
func GetInputProcessor() rdp.InputProcessor {
	return &windowsspecial.WindowsInputProcessor{}
}

func GetStreamer() rdp.MediaStreamer {
	return &windowsspecial.WindowsMediaStreamer{}
}

func GetConfigurator() rdp.Configurator {
	return &windowsspecial.WindowsConfigurator{}
}

func GetOSHelper() rdp.OSHelper {
	return &windowsspecial.WindowsOSHelper{}
}
