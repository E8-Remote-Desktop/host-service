package helpers

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/e8-remote-desktop/host-service/pkg/rdp"
	windowsspecial "github.com/e8-remote-desktop/host-service/pkg/rdp/windowsSpecial"
)

// Standard messages for all clients
var closeACK = []byte{0, 0}
var startACK = []byte{0, 1}
var closeMsg = []byte{0, 3}

type WindowsHelperNamedPipedClient struct {
	conn            net.Conn
	helperFunctions WindowsHelperProcessor
	genericName     string
	pipeName        string
	config          *rdp.StreamConfig
}

func (client *WindowsHelperNamedPipedClient) Register(name string, helperFunctions WindowsHelperProcessor) {
	client.pipeName = fmt.Sprintf(`\\.\pipe\e8-%s`, name)
	client.genericName = name
	client.helperFunctions = helperFunctions
	configurator := &windowsspecial.WindowsConfigurator{}
	var err error
	client.config, err = configurator.GetConfig()
	if err != nil {
		log.Fatalf("ERROR: could not load config in helper %v", err)
	}
}

func (client *WindowsHelperNamedPipedClient) Run() {
	log.Printf("Starting %s handler client with named pipes...", client.genericName)
	client.conn = client.connectToCommunicator() // This function will handle connection and retries
	defer client.conn.Close()

	// Give the helper the connection to allow it to send messages back
	client.helperFunctions.RegisterMsgSender(client.conn)

	// Pre-Start the helper
	client.helperFunctions.Start(client.config)

	log.Println("Client connected and listening for commands.")

	// This loop reads from the pipe and processes commands until the connection closes
	for {
		if !client.listenForCommands(client.conn) {
			break
		}
	}

	log.Println("Connection closed. Client shutting down.")
}

func (client *WindowsHelperNamedPipedClient) connectToCommunicator() net.Conn {
	var conn net.Conn
	var err error
	for {
		// We want rapid restarts until we can connect
		timeout := 100 * time.Millisecond
		conn, err = winio.DialPipe(client.pipeName, &timeout)
		if err == nil {
			// Connection successful, send the start message

			if _, writeErr := conn.Write(startACK); writeErr != nil {
				log.Fatalf("Failed to send start message: %v", writeErr)
			}
			log.Println("Sent start message [0, 1] to server.")
			return conn
		}
		log.Printf("Failed to connect to server: %v. Retrying in 100 ms...", err)
	}
}

func (client *WindowsHelperNamedPipedClient) listenForCommands(conn net.Conn) bool {
	// Buffer to read data from the pipe
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		if err != io.EOF {
			log.Printf("Pipe read error: %v", err)
		}
		return false // Stop on error or EOF
	}
	msg := buf[:n]

	if n > 0 {
		// Pass the slice and check if the processor received a close command
		return client.msgRouter(conn, msg)
	}

	return true
}

func (client *WindowsHelperNamedPipedClient) msgRouter(conn net.Conn, msg []byte) bool {
	if bytes.Equal(msg, closeMsg) {

		// Send the close acknowledgment [0, 0] back to the server
		log.Println("Close request received. Sending acknowledgment.")
		if _, err := conn.Write(closeACK); err != nil {
			log.Printf("Failed to send close acknowledgment: %v", err)
		}
		client.helperFunctions.Close()
		return false
	}
	client.helperFunctions.MsgProcessor(msg)

	return true
}
