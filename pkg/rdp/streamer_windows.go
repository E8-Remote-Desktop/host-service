//go:build windows
// +build windows

package rdp

// Load libraries that syscall/windows don't expose

type WindowsStreamer struct{}

func (w *WindowsStreamer) Start(config *StreamConfig) error {
	return nil
}

func (w *WindowsStreamer) Cancel() {

}
