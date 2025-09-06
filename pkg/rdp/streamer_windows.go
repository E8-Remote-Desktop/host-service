//go:build windows
// +build windows

package rdp

type WindowsStreamer struct{}

func (w *WindowsStreamer) Start(config *StreamConfig) error {
	return nil
}

func (w *WindowsStreamer) Cancel() {

}
