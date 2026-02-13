package tests

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/dollarkillerx/MessageBoy/internal/client"
)

// allocatePort finds a free port and returns an address string
func allocatePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// TestE2E_DirectForwarding tests direct TCP forwarding with traffic counting
func TestE2E_DirectForwarding(t *testing.T) {
	// Start echo server
	echoAddr, echoCleanup := startEchoServer(t)
	defer echoCleanup()

	// Create traffic counter
	tc := client.NewTrafficCounter()

	// Allocate a port for the forwarder
	listenAddr := allocatePort(t)

	cfg := client.ForwarderSection{
		ConnectTimeout: 5,
	}
	fwd := client.NewForwarder("test-rule", listenAddr, echoAddr, cfg, tc, nil)

	started := make(chan struct{})
	go func() {
		close(started)
		fwd.Start()
	}()
	<-started
	defer fwd.Stop()

	// Wait for forwarder to bind
	time.Sleep(200 * time.Millisecond)

	// Connect to forwarder
	conn, err := net.DialTimeout("tcp", listenAddr, time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to forwarder: %v", err)
	}
	defer conn.Close()

	// Send data
	testData := []byte("Hello through direct forwarder!")
	_, err = conn.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read echoed data
	buf := make([]byte, len(testData))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = io.ReadFull(conn, buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if string(buf) != string(testData) {
		t.Errorf("echoed data mismatch: got %q, want %q", buf, testData)
	}

	// Close connection and wait for traffic to be counted
	conn.Close()
	time.Sleep(100 * time.Millisecond)

	// Verify traffic counter
	reports := tc.GetAndReset()
	if len(reports) == 0 {
		t.Fatal("expected traffic reports, got none")
	}

	var found bool
	for _, r := range reports {
		if r.RuleID == "test-rule" {
			found = true
			if r.BytesIn == 0 && r.BytesOut == 0 {
				t.Error("expected non-zero traffic bytes")
			}
			if r.Connections == 0 {
				t.Error("expected at least 1 connection count")
			}
			t.Logf("Traffic report: BytesIn=%d, BytesOut=%d, Connections=%d, ActiveConns=%d",
				r.BytesIn, r.BytesOut, r.Connections, r.ActiveConns)
		}
	}
	if !found {
		t.Error("traffic report for 'test-rule' not found")
	}
}

// TestE2E_DirectForwarding_MultipleConnections tests multiple simultaneous connections
func TestE2E_DirectForwarding_MultipleConnections(t *testing.T) {
	echoAddr, echoCleanup := startEchoServer(t)
	defer echoCleanup()

	tc := client.NewTrafficCounter()
	listenAddr := allocatePort(t)
	cfg := client.ForwarderSection{ConnectTimeout: 5}
	fwd := client.NewForwarder("multi-rule", listenAddr, echoAddr, cfg, tc, nil)

	started := make(chan struct{})
	go func() {
		close(started)
		fwd.Start()
	}()
	<-started
	defer fwd.Stop()
	time.Sleep(200 * time.Millisecond)

	numConns := 5
	conns := make([]net.Conn, numConns)

	// Open multiple connections
	for i := 0; i < numConns; i++ {
		conn, err := net.DialTimeout("tcp", listenAddr, time.Second)
		if err != nil {
			t.Fatalf("conn %d: dial failed: %v", i, err)
		}
		conns[i] = conn
	}

	// Send and receive on each connection
	for i, conn := range conns {
		data := []byte{byte(i), 0xAA, 0xBB, 0xCC}
		conn.Write(data)
		buf := make([]byte, len(data))
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err := io.ReadFull(conn, buf)
		if err != nil {
			t.Fatalf("conn %d: read failed: %v", i, err)
		}
		if buf[0] != byte(i) {
			t.Errorf("conn %d: got first byte %d, want %d", i, buf[0], i)
		}
	}

	// Close all
	for _, conn := range conns {
		conn.Close()
	}
	time.Sleep(100 * time.Millisecond)

	reports := tc.GetAndReset()
	var found bool
	for _, r := range reports {
		if r.RuleID == "multi-rule" {
			found = true
			if r.Connections != int64(numConns) {
				t.Errorf("expected %d connections, got %d", numConns, r.Connections)
			}
		}
	}
	if !found {
		t.Errorf("traffic report for 'multi-rule' not found")
	}
}
