package windowsspecial

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/e8-remote-desktop/host-service/pkg/rdp"
)

// pipeName is the static name for the Windows named pipe.
const pipeNameStream = `\\.\pipe\e8-stream`

// WindowsStreamProcessor manages a Windows named pipe connection for raw byte communication.
type WindowsStreamProcessor struct {
	isStarted bool
	mu        sync.Mutex // Protects all fields below

	listener     net.Listener
	conn         net.Conn
	sendChan     chan []byte
	recvChan     chan []byte
	quitChan     chan struct{}
	wg           sync.WaitGroup
	closeAckChan chan struct{} // Used exclusively for the Close() handshake
}

func (processor *WindowsStreamProcessor) Init(config *rdp.StreamConfig) error {
	// ignored the helpers load the config themselves
	return nil

}

// Start creates a named pipe, waits for a client, and performs a start handshake.
// It waits for a 2-byte message [0, 1] from the client. If this message is not
// received within 5 seconds of the client connecting, it returns an error.
func (processor *WindowsStreamProcessor) IsStarted() bool {
	return processor.isStarted
}
func (processor *WindowsStreamProcessor) YouAreClosedTrustMe() {
	// this is for the instance when swithcing user accounts and it force crashes the streamer/input, they are closed
	processor.isStarted = false
}
func (processor *WindowsStreamProcessor) Start() error {
	// TODO: security allow only 1 client on the pipe
	processor.mu.Lock()
	if processor.isStarted {
		processor.mu.Unlock()
		return fmt.Errorf("an input processor is still running")
	}

	// Initialize channels and state for this session
	processor.sendChan = make(chan []byte, 100) // Buffered for non-blocking sends
	processor.recvChan = make(chan []byte, 100)
	processor.quitChan = make(chan struct{})

	listener, err := winio.ListenPipe(pipeNameStream, nil)
	if err != nil {
		processor.mu.Unlock()
		return fmt.Errorf("failed to listen on named pipe: %w", err)
	}
	processor.listener = listener
	processor.mu.Unlock()

	// Accept one connection. This will block until a client connects.
	conn, err := processor.listener.Accept()
	if err != nil {
		// Clean up the listener if accept fails (e.g., listener was closed)
		processor.listener.Close()
		return fmt.Errorf("failed to accept client connection: %w", err)
	}

	// --- Start Handshake ---
	// Set a deadline for receiving the start message.
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		conn.Close()
		processor.listener.Close()
		return fmt.Errorf("failed to set read deadline: %w", err)
	}

	handshakeBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, handshakeBuf); err != nil {
		conn.Close()
		processor.listener.Close()
		return fmt.Errorf("failed to read start handshake: %w", err)
	}

	// Expected message is [0, 1]
	expectedHandshake := []byte{0, 1}
	if !bytes.Equal(handshakeBuf, expectedHandshake) {
		conn.Close()
		processor.listener.Close()
		return fmt.Errorf("failed to start input: invalid handshake received")
	}
	log.Printf("Recieved Start Handshake from stream helper")
	// Handshake successful, remove the deadline.
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		conn.Close()
		processor.listener.Close()
		return fmt.Errorf("failed to clear read deadline: %w", err)
	}
	// --- End Handshake ---

	processor.mu.Lock()
	processor.conn = conn
	processor.isStarted = true
	processor.wg.Add(2)
	go processor.readLoop()
	go processor.writeLoop()
	processor.mu.Unlock()

	return nil
}

// Send provides a non-blocking way to send a byte slice to the client.
func (processor *WindowsStreamProcessor) Send(data []byte) {

	if !processor.isStarted {
		log.Printf("Send attempted to send to a non-started process, data: %v", data)
		return
	}

	select {
	case processor.sendChan <- data:
	default:
		log.Printf("WARNING: Channel Buffer fool")
		// This case is hit if the channel buffer is full.
	}
}

// Receive returns a read-only channel that broadcasts all messages received from the client.
// Any function can call this to get access to the stream of incoming data.
func (processor *WindowsStreamProcessor) Receive() <-chan []byte {
	return processor.recvChan
}

// Close sends a shutdown message and waits for a specific acknowledgment.
// It sends [0, 3] and waits for [0, 0]. If the ack is not received within 10 seconds,
// it returns an error but still proceeds with cleanup.
func (processor *WindowsStreamProcessor) Close() error {
	processor.mu.Lock()
	if !processor.isStarted {
		processor.mu.Unlock()
		return nil // Already closed
	}
	log.Printf("Sending Close Message to Stream")

	// Create a temporary channel to receive the specific close acknowledgment.
	processor.closeAckChan = make(chan struct{}, 1)
	processor.mu.Unlock()

	// Send the shutdown message
	shutdownMsg := []byte("close")
	_, err := processor.conn.Write(shutdownMsg)
	if err != nil {
		// If we can't even write the close message, proceed to force-close.
		processor.cleanup()
		return fmt.Errorf("failed to send close message: %w", err)
	}

	// Wait for the acknowledgment closeACK with a timeout.
	var returnErr error
	select {
	// this should already be 00
	case <-processor.closeAckChan:
		// Acknowledgment received successfully.
	// we can't predict a session change/shutdown before a session change, this scenario allows it to actually cleanup
	case <-time.After(500 * time.Millisecond):
		returnErr = fmt.Errorf("failed to stop stream: acknowledgment not received in time")
	}

	// Regardless of timeout, clean up all resources.
	processor.cleanup()
	return returnErr
}

// cleanup handles the graceful shutdown of goroutines and network resources.
func (processor *WindowsStreamProcessor) cleanup() {
	processor.mu.Lock()
	defer processor.mu.Unlock()

	// Signal goroutines to exit.
	close(processor.quitChan)

	// Close network resources to unblock any pending I/O operations.
	if processor.conn != nil {
		processor.conn.Close()
	}
	if processor.listener != nil {
		processor.listener.Close()
	}

	// Wait for goroutines to finish.
	processor.mu.Unlock()
	processor.wg.Wait()
	processor.mu.Lock()

	// Reset state
	processor.isStarted = false
	processor.conn = nil
	processor.listener = nil
	processor.closeAckChan = nil

	// Close channels to unblock any waiting receivers.
	close(processor.sendChan)
	close(processor.recvChan)
}

// readLoop continuously reads from the connection and forwards messages.
func (processor *WindowsStreamProcessor) readLoop() {
	defer processor.wg.Done()
	buf := make([]byte, 4096)
	closeAck := []byte("closeACK")

	for {
		n, err := processor.conn.Read(buf)
		if err != nil {
			// Connection closed or error, time to exit.
			return
		}

		msg := make([]byte, n)
		copy(msg, buf[:n])

		// Check if this is the special close acknowledgment.
		processor.mu.Lock()
		ackChan := processor.closeAckChan
		processor.mu.Unlock()

		if ackChan != nil && bytes.Equal(msg, closeAck) {
			// It's the ack for Close(). Signal the waiting Close() call.
			ackChan <- struct{}{}
		} else {
			// It's a regular message. Send it to the public receive channel.
			processor.recvChan <- msg
		}
	}
}

// writeLoop continuously reads from the send channel and writes to the connection.
func (processor *WindowsStreamProcessor) writeLoop() {
	defer processor.wg.Done()
	for {
		select {
		case data := <-processor.sendChan:
			if _, err := processor.conn.Write(data); err != nil {
				// Error on write, likely connection is dead.
				return
			}
		case <-processor.quitChan:
			// The Close() method has been called.
			return
		}
	}
}
