package client

import (
	"net"
	"sync"
	"testing"
	"time"
)

func TestForwarderStatusCallback_Success(t *testing.T) {
	// Create a temporary listener to get an available port
	tempListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create temp listener: %v", err)
	}
	addr := tempListener.Addr().String()
	tempListener.Close()

	var mu sync.Mutex
	var receivedStatus string
	var receivedError string
	var callCount int

	ready := make(chan struct{}, 1)
	callback := func(ruleID, status, errMsg string) {
		mu.Lock()
		defer mu.Unlock()
		receivedStatus = status
		receivedError = errMsg
		callCount++
		select {
		case ready <- struct{}{}:
		default:
		}
	}

	cfg := ForwarderSection{
		ConnectTimeout: 5,
		BufferSize:     32768,
	}

	f := NewForwarder("test-rule", addr, "127.0.0.1:9999", cfg, nil, callback)

	// Start forwarder in goroutine
	go func() {
		f.Start()
	}()

	// Wait for callback to fire (listener is ready)
	<-ready

	// Stop the forwarder
	f.Stop()

	mu.Lock()
	defer mu.Unlock()

	if callCount == 0 {
		t.Error("Status callback was never called")
	}

	// When listener succeeds, status should be "running"
	if receivedStatus != "running" {
		t.Errorf("Expected status 'running', got '%s'", receivedStatus)
	}

	if receivedError != "" {
		t.Errorf("Expected empty error, got '%s'", receivedError)
	}
}

func TestForwarderStatusCallback_Error(t *testing.T) {
	// First, occupy a port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	occupiedAddr := listener.Addr().String()

	var mu sync.Mutex
	var receivedStatus string
	var receivedError string
	var callCount int

	callback := func(ruleID, status, errMsg string) {
		mu.Lock()
		defer mu.Unlock()
		receivedStatus = status
		receivedError = errMsg
		callCount++
	}

	cfg := ForwarderSection{
		ConnectTimeout: 5,
		BufferSize:     32768,
	}

	// Try to start forwarder on occupied port
	f := NewForwarder("test-rule", occupiedAddr, "127.0.0.1:9999", cfg, nil, callback)

	// Start should return error
	err = f.Start()
	if err == nil {
		f.Stop()
		t.Fatal("Expected error when starting on occupied port")
	}

	mu.Lock()
	defer mu.Unlock()

	if callCount == 0 {
		t.Error("Status callback was never called")
	}

	if receivedStatus != "error" {
		t.Errorf("Expected status 'error', got '%s'", receivedStatus)
	}

	if receivedError == "" {
		t.Error("Expected error message, got empty string")
	}
}

func TestForwarderWithNilCallback(t *testing.T) {
	// Create a temporary listener to get an available port
	tempListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create temp listener: %v", err)
	}
	addr := tempListener.Addr().String()
	tempListener.Close()

	cfg := ForwarderSection{
		ConnectTimeout: 5,
		BufferSize:     32768,
	}

	// Should not panic with nil callback
	f := NewForwarder("test-rule", addr, "127.0.0.1:9999", cfg, nil, nil)

	go func() {
		f.Start()
	}()

	time.Sleep(100 * time.Millisecond)
	f.Stop()
}

func TestNewForwarder(t *testing.T) {
	cfg := ForwarderSection{
		ConnectTimeout: 10,
		BufferSize:     65536,
	}

	callback := func(ruleID, status, errMsg string) {}

	f := NewForwarder("rule-123", "0.0.0.0:8080", "192.168.1.1:80", cfg, nil, callback)

	if f.id != "rule-123" {
		t.Errorf("Expected id 'rule-123', got '%s'", f.id)
	}

	if f.listenAddr != "0.0.0.0:8080" {
		t.Errorf("Expected listenAddr '0.0.0.0:8080', got '%s'", f.listenAddr)
	}

	if f.targetAddr != "192.168.1.1:80" {
		t.Errorf("Expected targetAddr '192.168.1.1:80', got '%s'", f.targetAddr)
	}

	if f.cfg.ConnectTimeout != 10 {
		t.Errorf("Expected ConnectTimeout 10, got %d", f.cfg.ConnectTimeout)
	}

	if f.statusCallback == nil {
		t.Error("Expected statusCallback to be set")
	}
}

func TestForwarder_GetConfigHash(t *testing.T) {
	cfg := ForwarderSection{}
	f := NewForwarder("r1", "0.0.0.0:8080", "localhost:80", cfg, nil, nil)

	expected := "direct:0.0.0.0:8080:localhost:80"
	if got := f.GetConfigHash(); got != expected {
		t.Errorf("GetConfigHash() = %q, want %q", got, expected)
	}
}

func TestForwarder_GetListenAddr(t *testing.T) {
	cfg := ForwarderSection{}
	f := NewForwarder("r1", "0.0.0.0:9090", "localhost:80", cfg, nil, nil)

	if got := f.GetListenAddr(); got != "0.0.0.0:9090" {
		t.Errorf("GetListenAddr() = %q, want %q", got, "0.0.0.0:9090")
	}
}

func TestForwarder_StopBeforeStart(t *testing.T) {
	cfg := ForwarderSection{}
	f := NewForwarder("r1", "127.0.0.1:0", "localhost:80", cfg, nil, nil)

	// Should not panic
	f.Stop()
}
