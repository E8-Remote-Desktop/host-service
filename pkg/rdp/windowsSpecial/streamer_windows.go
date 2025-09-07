//go:build windows
// +build windows

package windowsspecial

// Load libraries that syscall/windows don't expose
import (
	"github.com/e8-remote-desktop/host-service/pkg/rdp"
)

type WindowsMediaStreamer struct{}

func (w *WindowsMediaStreamer) Start(config *rdp.StreamConfig) error {
	return nil
}

func (w *WindowsMediaStreamer) Cancel() {

}
