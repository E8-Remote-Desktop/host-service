package windowsspecial

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/e8-remote-desktop/host-service/pkg/rdp"
	"golang.org/x/sys/windows"
)

type WindowsOSHelper struct {
	input          rdp.InputProcessor
	streamer       rdp.MediaStreamer
	desktopMonitor DesktopMonitor
	exePath        string
	runner         *DesktopRunner
	isRunning      bool
}

func (helper *WindowsOSHelper) Init(input rdp.InputProcessor, streamer rdp.MediaStreamer) error {
	helper.input = input
	helper.streamer = streamer
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		log.Fatal(err)
	}
	helper.exePath = exePath
	helper.runner = &DesktopRunner{}
	if err := helper.runner.Init(); err != nil {
		log.Fatalf("Could not init the windows OS connector %v", err)
	}
	return nil

}

func (helper *WindowsOSHelper) StartStreamAndInput() error {
	helper.isRunning = true
	go helper.MonitorDesktops()
	return nil

}

func (helper *WindowsOSHelper) Close() error {
	helper.isRunning = false
	helper.input.Close()
	helper.streamer.Close()
	return nil
}

// This is seperate because it only needs to restart on token change
func (helper *WindowsOSHelper) RestartDesktopMonitor(token windows.Token) {
	log.Printf("DEBUG: Restarting Desktop Monitoring Service")
	if err := helper.desktopMonitor.Close(); err != nil {
		log.Printf("WARNING, Desktop Monitor could not be closed: %v", err)
		helper.desktopMonitor.YouAreClosedTrustMe()
	}
	err := helper.runner.RunProcesses([]string{fmt.Sprintf("%s -desktopmonitorhelper", helper.exePath)}, token)
	if err != nil {
		log.Printf("ERROR: Could not start desktop monitor with correct permissions, %v", err)
		return
	}
	log.Printf("DEBUG: Requesting data start for the desktop monitor")

	if err := helper.desktopMonitor.Start(); err != nil {
		log.Printf("ERROR: Could not start desktop monitor connector: %v", err)
	}
	log.Printf("DEBUG: Started desktop monitoring helper")

}

func (helper *WindowsOSHelper) RestartInteractiveServices(token windows.Token) {

	// we should only need to check input (right?)
	log.Printf("DEBUG: Restarting on new desktop")
	if err := helper.input.Close(); err != nil {
		log.Printf("ERROR: FAILED TO RESTART INPUT: %v; this is expected when switching users", err)
		helper.input.YouAreClosedTrustMe()
	}
	if err := helper.streamer.Close(); err != nil {
		log.Printf("WARNING: FAILED TO RESTART VIDEO/STREAMER: %v; this is expected when switching users", err)
		helper.streamer.YouAreClosedTrustMe()
	}
	// Start helpers
	err := helper.runner.RunProcesses(
		[]string{
			fmt.Sprintf("%s -inputhelper", helper.exePath),
			fmt.Sprintf("%s -streamhelper", helper.exePath),
		},
		token,
	)
	if err != nil {
		log.Printf("ERROR: Could not start Stream and Input Processes with correct permissions, %v", err)
		return
	}
	log.Printf("DEBUG: Requesting data start for the helpers")

	if err := helper.streamer.Start(); err != nil {
		log.Printf("ERROR: Could not start Stream connector: %v", err)
	}
	if err := helper.input.Start(); err != nil {
		log.Printf("ERROR: Could not start Input connector: %v", err)
	}
	log.Printf("DEBUG: Request for start sent to  Input Handler")

}

// monitorDesktops loops every 50ms and prints the active desktop name when it changes
func (helper *WindowsOSHelper) MonitorDesktops() {
	// will be set on first run
	var token windows.Token = helper.runner.getActiveUserToken()
	var lastUser string
	log.Printf("Starting Desktop Monitor")
	helper.RestartDesktopMonitor(token)
	log.Printf("Started Desktop Monitor")
	// idk if this works when the program gets exit signal
	//defer token.Close()

	for {
		//log.Printf("DEBUG: Checking if we are currently displaying the right content")
		if !helper.isRunning {
			log.Printf("DEBUG: Shutting down because we are not supposed to be running")
			helper.desktopMonitor.Close()
			return
		}
		select {
		case msg := <-helper.desktopMonitor.GetRecieveChannel():
			//todo replace with switch
			if strings.Contains(string(msg), "restart-request") {
				log.Printf("DEBUG: Desktop Monitor Requested we restart it, obliging")
				helper.RestartDesktopMonitor(token)
				token.Close()
				continue
			}
			// desktop-change otherwise
			log.Printf("DEBUG: Got Desktop Change notification")
			log.Printf("DEBUG: Changing to desktop: %s", msg)
			log.Printf("DEBUG: Getting active user token")
			token = helper.runner.getActiveUserToken()

			currentUser, err := helper.runner.getCurrentIOSID()
			if err != nil {
				log.Printf("WARNING Could not get current user, reattempting")
				time.Sleep(500 * time.Millisecond)
				continue
			}
			lastUser = currentUser
			helper.RestartInteractiveServices(token)
			token.Close()
		default:
			//log.Printf("DEBUG: Checking if user account is current")
			currentUser, err := helper.runner.getCurrentIOSID()
			if err != nil {
				log.Printf("WARNING Could not get current user, reattempting")
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if currentUser != lastUser {
				log.Printf("DEBUG: Switching user because of differing token")
				token = helper.runner.getActiveUserToken()
				lastUser = currentUser
				helper.RestartDesktopMonitor(token)
				helper.RestartInteractiveServices(token)
				token.Close()
			}
			time.Sleep(200 * time.Millisecond)

		}

	}
}
