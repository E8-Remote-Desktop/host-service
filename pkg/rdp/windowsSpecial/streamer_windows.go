//go:build windows
// +build windows

package windowsspecial

// Load libraries that syscall/windows don't expose
import (
	"github.com/e8-remote-desktop/host-service/pkg/rdp"
)

type WindowsMediaStreamer struct {
	config *rdp.StreamConfig
}

func (w *WindowsMediaStreamer) Init(config *rdp.StreamConfig) error {
	w.config = config
	return nil
}

func (w *WindowsMediaStreamer) Start() error {
	return nil
}

func (w *WindowsMediaStreamer) Restart() {
	if w.config == nil {
		return
	}
	w.Start()

}

func (w *WindowsMediaStreamer) Cancel() {

}
