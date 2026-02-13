package tests

import (
	"bytes"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/dollarkillerx/MessageBoy/internal/relay"
	"github.com/gorilla/websocket"
)

// TestE2E_FullRelayTunnel tests a full relay tunnel:
// Client A -> Server -> Client B -> Echo Server -> back
func TestE2E_FullRelayTunnel(t *testing.T) {
	// Start echo server
	echoAddr, echoCleanup := startEchoServer(t)
	defer echoCleanup()

	// Start relay server
	wsURL, wsServer, serverCleanup := startTestWSServer(t)
	defer serverCleanup()

	// Connect Client A (entry)
	connA := connectTestClient(t, wsURL, "clientA")
	defer connA.Close()

	// Connect Client B (exit)
	connB := connectTestClient(t, wsURL, "clientB")
	defer connB.Close()

	waitForClient(t, wsServer, "clientA", time.Second)
	waitForClient(t, wsServer, "clientB", time.Second)

	// Client A sends Connect request, targeting clientB
	streamID := uint32(100)
	sendTunnelMsg(t, connA, &relay.TunnelMessage{
		Type:     relay.MsgTypeConnect,
		StreamID: streamID,
		Target:   echoAddr,
		Payload:  []byte("clientB"),
	})

	// Client B receives the Connect
	msgB := recvTunnelMsg(t, connB, 2*time.Second)
	if msgB.Type != relay.MsgTypeConnect {
		t.Fatalf("expected MsgTypeConnect, got %d", msgB.Type)
	}
	if msgB.Target != echoAddr {
		t.Fatalf("expected target %s, got %s", echoAddr, msgB.Target)
	}

	// Client B connects to echo server
	echoConn, err := net.DialTimeout("tcp", echoAddr, time.Second)
	if err != nil {
		t.Fatalf("Client B dial echo failed: %v", err)
	}
	defer echoConn.Close()

	// Client B sends ConnAck
	sendTunnelMsg(t, connB, &relay.TunnelMessage{
		Type:     relay.MsgTypeConnAck,
		StreamID: streamID,
	})

	// Client A receives ConnAck
	ack := recvTunnelMsg(t, connA, 2*time.Second)
	if ack.Type != relay.MsgTypeConnAck {
		t.Fatalf("expected MsgTypeConnAck, got %d", ack.Type)
	}

	// Client A sends data
	testData := []byte("Hello from Client A!")
	sendTunnelMsg(t, connA, &relay.TunnelMessage{
		Type:     relay.MsgTypeData,
		StreamID: streamID,
		Payload:  testData,
	})

	// Client B receives the data and writes to echo server
	dataMsg := recvTunnelMsg(t, connB, 2*time.Second)
	if dataMsg.Type != relay.MsgTypeData {
		t.Fatalf("expected MsgTypeData, got %d", dataMsg.Type)
	}
	if !bytes.Equal(dataMsg.Payload, testData) {
		t.Fatalf("payload mismatch: got %q, want %q", dataMsg.Payload, testData)
	}

	// Write to echo server and read the echo back
	echoConn.Write(dataMsg.Payload)
	echoBuf := make([]byte, len(testData))
	echoConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = io.ReadFull(echoConn, echoBuf)
	if err != nil {
		t.Fatalf("echo read failed: %v", err)
	}

	// Client B sends echoed data back
	sendTunnelMsg(t, connB, &relay.TunnelMessage{
		Type:     relay.MsgTypeData,
		StreamID: streamID,
		Payload:  echoBuf,
	})

	// Client A receives the echoed data
	echoedMsg := recvTunnelMsg(t, connA, 2*time.Second)
	if echoedMsg.Type != relay.MsgTypeData {
		t.Fatalf("expected MsgTypeData, got %d", echoedMsg.Type)
	}
	if !bytes.Equal(echoedMsg.Payload, testData) {
		t.Fatalf("echoed data mismatch: got %q, want %q", echoedMsg.Payload, testData)
	}

	// Close
	sendTunnelMsg(t, connA, &relay.TunnelMessage{
		Type:     relay.MsgTypeClose,
		StreamID: streamID,
	})

	closeMsg := recvTunnelMsg(t, connB, 2*time.Second)
	if closeMsg.Type != relay.MsgTypeClose {
		t.Fatalf("expected MsgTypeClose, got %d", closeMsg.Type)
	}
}

// TestE2E_ConcurrentStreams tests 10 concurrent streams transmitting simultaneously
func TestE2E_ConcurrentStreams(t *testing.T) {
	wsURL, wsServer, cleanup := startTestWSServer(t)
	defer cleanup()

	connA := connectTestClient(t, wsURL, "muxA")
	defer connA.Close()
	connB := connectTestClient(t, wsURL, "muxB")
	defer connB.Close()

	waitForClient(t, wsServer, "muxA", time.Second)
	waitForClient(t, wsServer, "muxB", time.Second)

	numStreams := 10

	// Setup routes for all streams
	for i := 0; i < numStreams; i++ {
		sendTunnelMsg(t, connA, &relay.TunnelMessage{
			Type:     relay.MsgTypeConnect,
			StreamID: uint32(i + 1),
			Target:   "127.0.0.1:9999",
			Payload:  []byte("muxB"),
		})
	}

	// Client B receives all connect messages
	for i := 0; i < numStreams; i++ {
		msg := recvTunnelMsg(t, connB, 2*time.Second)
		if msg.Type != relay.MsgTypeConnect {
			t.Fatalf("stream %d: expected Connect, got %d", i, msg.Type)
		}
		// Send ConnAck
		sendTunnelMsg(t, connB, &relay.TunnelMessage{
			Type:     relay.MsgTypeConnAck,
			StreamID: msg.StreamID,
		})
	}

	// Drain ConnAcks on A side
	for i := 0; i < numStreams; i++ {
		msg := recvTunnelMsg(t, connA, 2*time.Second)
		if msg.Type != relay.MsgTypeConnAck {
			t.Fatalf("expected ConnAck, got %d", msg.Type)
		}
	}

	// Send data on all streams concurrently from A
	var wg sync.WaitGroup
	var muA sync.Mutex
	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(streamID uint32) {
			defer wg.Done()
			data := []byte{byte(streamID), 0xAA, 0xBB}
			msg := &relay.TunnelMessage{
				Type:     relay.MsgTypeData,
				StreamID: streamID,
				Payload:  data,
			}
			msgBytes, _ := msg.Marshal()
			muA.Lock()
			connA.WriteMessage(websocket.BinaryMessage, msgBytes)
			muA.Unlock()
		}(uint32(i + 1))
	}
	wg.Wait()

	// Client B should receive all data messages
	received := make(map[uint32]bool)
	for i := 0; i < numStreams; i++ {
		msg := recvTunnelMsg(t, connB, 2*time.Second)
		if msg.Type != relay.MsgTypeData {
			t.Fatalf("expected Data, got %d", msg.Type)
		}
		received[msg.StreamID] = true
	}

	for i := 1; i <= numStreams; i++ {
		if !received[uint32(i)] {
			t.Errorf("stream %d data not received", i)
		}
	}
}

// TestE2E_LargeDataTransfer tests 1MB data transfer through relay in 32KB chunks
func TestE2E_LargeDataTransfer(t *testing.T) {
	echoAddr, echoCleanup := startEchoServer(t)
	defer echoCleanup()

	wsURL, wsServer, cleanup := startTestWSServer(t)
	defer cleanup()

	connA := connectTestClient(t, wsURL, "largeA")
	defer connA.Close()
	connB := connectTestClient(t, wsURL, "largeB")
	defer connB.Close()

	waitForClient(t, wsServer, "largeA", time.Second)
	waitForClient(t, wsServer, "largeB", time.Second)

	streamID := uint32(1)

	// Setup route
	sendTunnelMsg(t, connA, &relay.TunnelMessage{
		Type:     relay.MsgTypeConnect,
		StreamID: streamID,
		Target:   echoAddr,
		Payload:  []byte("largeB"),
	})

	// B receives Connect
	recvTunnelMsg(t, connB, 2*time.Second)

	// B connects to echo server
	echoConn, err := net.DialTimeout("tcp", echoAddr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer echoConn.Close()

	// B sends ConnAck
	sendTunnelMsg(t, connB, &relay.TunnelMessage{
		Type:     relay.MsgTypeConnAck,
		StreamID: streamID,
	})
	recvTunnelMsg(t, connA, 2*time.Second) // drain ConnAck on A

	// Generate 1MB of random data
	totalSize := 1024 * 1024
	chunkSize := 32 * 1024
	fullData := make([]byte, totalSize)
	rand.Read(fullData)

	// Send all chunks from A
	var receivedData bytes.Buffer
	var echoedData bytes.Buffer
	done := make(chan error, 1)

	// B side: receive data, write to echo, read echo back
	go func() {
		for receivedData.Len() < totalSize {
			connB.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, raw, err := connB.ReadMessage()
			if err != nil {
				done <- err
				return
			}
			msg, err := relay.UnmarshalTunnelMessage(raw)
			if err != nil {
				done <- err
				return
			}
			if msg.Type == relay.MsgTypeData {
				receivedData.Write(msg.Payload)
				// Write to echo server
				echoConn.Write(msg.Payload)
			}
		}
		done <- nil
	}()

	// A sends chunks
	for offset := 0; offset < totalSize; offset += chunkSize {
		end := offset + chunkSize
		if end > totalSize {
			end = totalSize
		}
		sendTunnelMsg(t, connA, &relay.TunnelMessage{
			Type:     relay.MsgTypeData,
			StreamID: streamID,
			Payload:  fullData[offset:end],
		})
	}

	// Wait for B to receive all data
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("B receiver error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for B to receive all data")
	}

	// Read echoed data from echo server
	echoReadDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		for echoedData.Len() < totalSize {
			echoConn.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, err := echoConn.Read(buf)
			if err != nil {
				echoReadDone <- err
				return
			}
			echoedData.Write(buf[:n])
		}
		echoReadDone <- nil
	}()

	select {
	case err := <-echoReadDone:
		if err != nil {
			t.Fatalf("echo read error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("timeout reading echo data, got %d/%d bytes", echoedData.Len(), totalSize)
	}

	// Verify
	if !bytes.Equal(receivedData.Bytes(), fullData) {
		t.Error("received data does not match sent data")
	}
	if !bytes.Equal(echoedData.Bytes(), fullData) {
		t.Error("echoed data does not match sent data")
	}
}

// TestE2E_RouteCleanupOnDisconnect tests that routes are cleaned up when a client disconnects
func TestE2E_RouteCleanupOnDisconnect(t *testing.T) {
	wsURL, wsServer, cleanup := startTestWSServer(t)
	defer cleanup()

	connA := connectTestClient(t, wsURL, "cleanA")
	defer connA.Close()
	connB := connectTestClient(t, wsURL, "cleanB")

	waitForClient(t, wsServer, "cleanA", time.Second)
	waitForClient(t, wsServer, "cleanB", time.Second)

	// Setup several routes
	for i := uint32(1); i <= 5; i++ {
		sendTunnelMsg(t, connA, &relay.TunnelMessage{
			Type:     relay.MsgTypeConnect,
			StreamID: i,
			Target:   "127.0.0.1:9999",
			Payload:  []byte("cleanB"),
		})
	}

	// Drain connect messages on B
	for i := 0; i < 5; i++ {
		recvTunnelMsg(t, connB, 2*time.Second)
	}

	// Disconnect Client B
	connB.Close()
	time.Sleep(200 * time.Millisecond)

	// Client B should no longer be online
	if wsServer.IsClientOnline("cleanB") {
		t.Error("cleanB should be offline after disconnect")
	}

	// Try to send data on a route that was connected to cleanB
	// The server should not be able to forward it (no error to A, just logged)
	sendTunnelMsg(t, connA, &relay.TunnelMessage{
		Type:     relay.MsgTypeData,
		StreamID: 1,
		Payload:  []byte("test"),
	})

	// The data should be silently dropped (route cleaned up)
	// Verify by checking that no error is sent back within a short timeout
	connA.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := connA.ReadMessage()
	if err == nil {
		t.Error("expected timeout, but received a message")
	}
}
