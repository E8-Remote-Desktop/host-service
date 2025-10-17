package windowsspecial

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
)

// pipeNameInput is the static name for the Windows named pipe.

type WindowsCommunicator interface {
	Init() error
	Send([]byte) error
	Recieve() <-chan []byte
	Close() error
	Start() error
	IsConnected() bool
	GetGenericName() string
}

// WindowsInputpipeController manages a Windows named pipe connection for raw byte communication.
type WindowsNamedPipeCommunicator struct {
	pipeName    string
	GenericName string
	isStarted   bool
	mu          sync.Mutex // Protects all fields below

	listener     net.Listener
	conn         net.Conn
	sendChan     chan []byte
	recvChan     chan []byte
	quitChan     chan struct{}
	wg           sync.WaitGroup
	closeAckChan chan struct{} // Used exclusively for the Close() handshake
}

func (pipeController *WindowsNamedPipeCommunicator) GetGenericName() string {
	return pipeController.GenericName
}

func (pipeController *WindowsNamedPipeCommunicator) Init() error {
	if strings.Contains(pipeController.GenericName, " ") {
		return fmt.Errorf("name not allowed to have spaces")
	}
	pipeController.pipeName = fmt.Sprintf(`\\.\pipe\e8-%s`, pipeController.GenericName)
	return nil
}

func (pipeController *WindowsNamedPipeCommunicator) IsConnected() bool {
	return pipeController.isStarted
}

// Start creates a named pipe, waits for a client, and performs a start handshake.
// It waits for a 2-byte message [0, 1] from the client. If this message is not
// received within 5 seconds of the client connecting, it returns an error
func (pipeController *WindowsNamedPipeCommunicator) Start() error {
	if pipeController.pipeName == "" {
		return fmt.Errorf("pipe name not set")
	}
	// TODO: security allow only 1 client on the pipe
	pipeController.mu.Lock()
	if pipeController.isStarted {
		pipeController.mu.Unlock()
		return fmt.Errorf("an %s pipeController is still running", pipeController.GenericName)
	}
	// TODO FIXME THIS ALLOWS EVERYONE
	sddl := "D:P(A;;GA;;;WD)(A;;GA;;;AN)"

	pipeConfig := &winio.PipeConfig{
		SecurityDescriptor: sddl,
	}

	// Initialize channels and state for this session
	pipeController.sendChan = make(chan []byte, 100) // Buffered for non-blocking sends
	pipeController.recvChan = make(chan []byte, 100)
	pipeController.quitChan = make(chan struct{})

	listener, err := winio.ListenPipe(pipeController.pipeName, pipeConfig)
	if err != nil {
		pipeController.mu.Unlock()
		return fmt.Errorf("failed to listen on named pipe: %v", err)
	}
	pipeController.listener = listener
	pipeController.mu.Unlock()

	// Accept one connection. This will block until a client connects.
	conn, err := pipeController.listener.Accept()
	if err != nil {
		// Clean up the listener if accept fails (e.g., listener was closed)
		pipeController.listener.Close()
		return fmt.Errorf("failed to accept client connection: %w", err)
	}

	// --- Start Handshake ---
	// Set a deadline for receiving the start message.
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		conn.Close()
		pipeController.listener.Close()
		return fmt.Errorf("failed to set read deadline: %w", err)
	}

	handshakeBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, handshakeBuf); err != nil {
		conn.Close()
		pipeController.listener.Close()
		return fmt.Errorf("failed to read start handshake: %w", err)
	}

	// Expected message is [0, 1]
	expectedHandshake := []byte{0, 1}
	if !bytes.Equal(handshakeBuf, expectedHandshake) {
		conn.Close()
		pipeController.listener.Close()
		return fmt.Errorf("failed to start pipe %s: invalid handshake received", pipeController.GenericName)
	}
	log.Printf("Recieved Start Handshake from %s helper", pipeController.GenericName)
	// Handshake successful, remove the deadline.
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		conn.Close()
		pipeController.listener.Close()
		return fmt.Errorf("failed to clear read deadline: %w", err)
	}
	// --- End Handshake ---

	pipeController.mu.Lock()
	pipeController.conn = conn
	pipeController.isStarted = true
	pipeController.wg.Add(2)
	go pipeController.readLoop()
	go pipeController.writeLoop()
	pipeController.mu.Unlock()

	return nil
}

// Send provides a non-blocking way to send a byte slice to the client.
func (pipeController *WindowsNamedPipeCommunicator) Send(data []byte) error {
	pipeController.mu.Lock()
	defer pipeController.mu.Unlock()

	if !pipeController.isStarted {
		log.Printf("Send attempted to send to a non-started process, data: %v", data)
		return nil
	}

	// Use a select to prevent blocking if the send channel is full.
	select {
	case pipeController.sendChan <- data:
	default:

		log.Printf("WARNING: Channel for %s Buffer full", pipeController.GenericName)
	}
	return nil
}

// Receive returns a read-only channel that broadcasts all messages received from the client.
// Any function can call this to get access to the stream of incoming data.
func (pipeController *WindowsNamedPipeCommunicator) Recieve() <-chan []byte {
	return pipeController.recvChan
}

// Close sends a shutdown message and waits for a specific acknowledgment.
// It sends [0, 3] and waits for [0, 0]. If the ack is not received within 10 seconds,
// it returns an error but still proceeds with cleanup.
func (pipeController *WindowsNamedPipeCommunicator) Close() error {
	pipeController.mu.Lock()
	if !pipeController.isStarted {
		pipeController.mu.Unlock()
		return nil // Already closed
	}

	// Create a temporary channel to receive the specific close acknowledgment.
	pipeController.closeAckChan = make(chan struct{}, 1)
	pipeController.mu.Unlock()

	// Send the shutdown message [0, 3]
	shutdownMsg := []byte{0, 3}
	_, err := pipeController.conn.Write(shutdownMsg)
	if err != nil {
		// If we can't even write the close message, proceed to force-close.
		pipeController.cleanup()
		return fmt.Errorf("failed to send close message: %w", err)
	}

	// Wait for the acknowledgment [0, 0] with a timeout.
	var returnErr error
	select {
	// this should already be 00
	case <-pipeController.closeAckChan:
		// Acknowledgment received successfully.
	case <-time.After(500 * time.Millisecond):
		returnErr = fmt.Errorf("failed to stop input: acknowledgment not received in time")
	}

	// Regardless of timeout, clean up all resources.
	pipeController.cleanup()
	return returnErr
}

// cleanup handles the graceful shutdown of goroutines and network resources.
func (pipeController *WindowsNamedPipeCommunicator) cleanup() {
	pipeController.mu.Lock()
	defer pipeController.mu.Unlock()

	// Signal goroutines to exit.
	close(pipeController.quitChan)

	// Close network resources to unblock any pending I/O operations.
	if pipeController.conn != nil {
		pipeController.conn.Close()
	}
	if pipeController.listener != nil {
		pipeController.listener.Close()
	}

	// Wait for goroutines to finish.
	pipeController.mu.Unlock()
	pipeController.wg.Wait()
	pipeController.mu.Lock()

	// Reset state
	pipeController.isStarted = false
	pipeController.conn = nil
	pipeController.listener = nil
	pipeController.closeAckChan = nil

	// Close channels to unblock any waiting receivers.
	close(pipeController.sendChan)
	close(pipeController.recvChan)
}

// readLoop continuously reads from the connection and forwards messages.
func (pipeController *WindowsNamedPipeCommunicator) readLoop() {
	defer pipeController.wg.Done()
	buf := make([]byte, 4096)
	closeAck := []byte{0, 0}

	for {
		n, err := pipeController.conn.Read(buf)
		if err != nil {
			// Connection closed or error, time to exit.
			return
		}

		msg := make([]byte, n)
		copy(msg, buf[:n])

		// Check if this is the special close acknowledgment.
		pipeController.mu.Lock()
		ackChan := pipeController.closeAckChan
		pipeController.mu.Unlock()

		if ackChan != nil && bytes.Equal(msg, closeAck) {
			// It's the ack for Close(). Signal the waiting Close() call.
			ackChan <- struct{}{}
		} else {
			// It's a regular message. Send it to the public receive channel.
			pipeController.recvChan <- msg
		}
	}
}

// writeLoop continuously reads from the send channel and writes to the connection.
func (pipeController *WindowsNamedPipeCommunicator) writeLoop() {
	defer pipeController.wg.Done()
	for {
		select {
		case data := <-pipeController.sendChan:
			if _, err := pipeController.conn.Write(data); err != nil {
				// Error on write, likely connection is dead.
				return
			}
		case <-pipeController.quitChan:
			// The Close() method has been called.
			return
		}
	}
}
