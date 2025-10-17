package winDesktopHelper

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"
	"unsafe"

	"github.com/e8-remote-desktop/host-service/pkg/rdp"
	windowsspecial "github.com/e8-remote-desktop/host-service/pkg/rdp/windowsSpecial"
	"github.com/winlabs/gowin32/wrappers"
	"golang.org/x/sys/windows"
)

type WindowsDesktopPoller struct {
	conn           net.Conn
	currentDesktop string
	running        bool
}

func (desktopPoller *WindowsDesktopPoller) poller() {
	for {
		//log.Printf("DEBUG: Polling for desktop change")
		if !desktopPoller.running {
			log.Printf("DEBUG: closing because not running anymore")
			return
		}
		var desktopName string
		hDesktop, err := wrappers.OpenInputDesktop(0, false,
			windowsspecial.DESKTOP_READOBJECTS)
		if strings.Contains(err.Error(), "Access is denied") {
			// we should restart
			log.Printf("INFO: Requesting Restart Because We Don't Have The Permission to View the Desktop")
			if _, err := desktopPoller.conn.Write([]byte(fmt.Sprintf("%s, %s", "restart-request: ", err.Error()))); err != nil {
				log.Printf("ERROR: Could not write to connection, reattempting")
			}
			return

		}
		if err != nil {
			log.Printf("ERROR: Could not open desktop %v", err)
			continue
		}
		defer wrappers.CloseDesktop(hDesktop)

		// Get the desktop's name.
		var desktopNameLength uint32
		if err := wrappers.GetUserObjectInformation(hDesktop, windowsspecial.UOI_NAME, uintptr(unsafe.Pointer(nil)), 0, &desktopNameLength); err != nil {
			log.Printf("DEBUG: Errors from desktop Length %v", err)
		}
		//log.Printf("DEBUG: Desktop Name Length: %v", desktopNameLength)
		if desktopNameLength == 0 {
			log.Printf("WARNING: DESKTOP LENGTH 0 RE-ATTEMPTING")
			continue
		}

		desktopNameUTF16 := make([]uint16, 256)
		var blankuint32 uint32

		err = wrappers.GetUserObjectInformation(hDesktop,
			windowsspecial.UOI_NAME,
			uintptr(
				unsafe.Pointer(&desktopNameUTF16[0]),
			),
			desktopNameLength,
			&blankuint32)

		if err != nil {
			log.Printf("WARNING: Could not get info about desktop")
			continue
		}
		desktopName = windows.UTF16ToString(desktopNameUTF16)
		if desktopName != desktopPoller.currentDesktop {
			log.Printf("DEBUG: Desktop Changed, %s", desktopName)
			if _, err := desktopPoller.conn.Write([]byte(fmt.Sprintf("%s, %s", "desktop-change: ", desktopName))); err != nil {
				log.Printf("ERROR: Could not write to connection, reattempting")
				continue
			}
			desktopPoller.currentDesktop = desktopName

		}

		time.Sleep(200 * time.Millisecond)
	}

}

func (desktopPoller *WindowsDesktopPoller) Start(config *rdp.StreamConfig) error {
	log.Printf("Running Desktop Monitoring Service (Started)")
	desktopPoller.running = true
	go desktopPoller.poller()
	return nil
}
func (desktopPoller *WindowsDesktopPoller) Close() error {

	desktopPoller.running = false
	return nil
}
func (desktopPoller *WindowsDesktopPoller) MsgProcessor([]byte) {
	// This is a push helper, is will send when it needs to no need to recieve anything
}
func (desktopPoller *WindowsDesktopPoller) RegisterMsgSender(conn net.Conn) {
	desktopPoller.conn = conn
}
