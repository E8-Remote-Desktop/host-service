//go:build windows
// +build windows

package windowsstreamhelper

import (
	"context"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
)

type WindowsStreamHelper struct {
	mainLoop       *glib.MainLoop
	mainLoopCancel context.CancelFunc
	pipelines      []*gst.Pipeline
	isClosing      bool
}
