package client

import (
	"net"
	"sync"
	"testing"

	"github.com/dollarkillerx/MessageBoy/internal/relay"
)

func TestRelayForwarderStatusCallback_Error(t *testing.T) {
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

	// Try to start relay forwarder on occupied port (nil wsConn is fine for this test)
	f := NewRelayForwarder("test-rule", occupiedAddr, "192.168.1.1:80", []string{}, cfg, nil, nil, callback)

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

func TestNewRelayForwarder(t *testing.T) {
	cfg := ForwarderSection{
		ConnectTimeout: 10,
		BufferSize:     65536,
	}

	callback := func(ruleID, status, errMsg string) {}
	relayChain := []string{"client-a", "client-b"}

	f := NewRelayForwarder("rule-456", "0.0.0.0:9090", "192.168.1.1:443", relayChain, cfg, nil, nil, callback)

	if f.id != "rule-456" {
		t.Errorf("Expected id 'rule-456', got '%s'", f.id)
	}

	if f.listenAddr != "0.0.0.0:9090" {
		t.Errorf("Expected listenAddr '0.0.0.0:9090', got '%s'", f.listenAddr)
	}

	if f.exitAddr != "192.168.1.1:443" {
		t.Errorf("Expected exitAddr '192.168.1.1:443', got '%s'", f.exitAddr)
	}

	if len(f.relayChain) != 2 {
		t.Errorf("Expected relayChain length 2, got %d", len(f.relayChain))
	}

	if f.relayChain[0] != "client-a" || f.relayChain[1] != "client-b" {
		t.Error("RelayChain values don't match")
	}

	if f.statusCallback == nil {
		t.Error("Expected statusCallback to be set")
	}
}

func TestRelayForwarderWithNilCallback(t *testing.T) {
	cfg := ForwarderSection{
		ConnectTimeout: 5,
		BufferSize:     32768,
	}

	// Should not panic with nil callback
	f := NewRelayForwarder("test-rule", "127.0.0.1:0", "192.168.1.1:80", []string{}, cfg, nil, nil, nil)

	if f.statusCallback != nil {
		t.Error("Expected statusCallback to be nil")
	}
}

func TestRelayForwarder_GetConfigHash(t *testing.T) {
	tests := []struct {
		name       string
		listenAddr string
		exitAddr   string
		chain      []string
		expected   string
	}{
		{
			name:       "empty chain",
			listenAddr: ":8080",
			exitAddr:   "10.0.0.1:80",
			chain:      []string{},
			expected:   "relay::8080:10.0.0.1:80:",
		},
		{
			name:       "single chain",
			listenAddr: ":8080",
			exitAddr:   "10.0.0.1:80",
			chain:      []string{"c1"},
			expected:   "relay::8080:10.0.0.1:80:c1,",
		},
		{
			name:       "two chain",
			listenAddr: ":8080",
			exitAddr:   "10.0.0.1:80",
			chain:      []string{"c1", "c2"},
			expected:   "relay::8080:10.0.0.1:80:c1,c2,",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := NewRelayForwarder("r1", tc.listenAddr, tc.exitAddr, tc.chain, ForwarderSection{}, nil, nil, nil)
			if got := f.GetConfigHash(); got != tc.expected {
				t.Errorf("GetConfigHash() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestRelayForwarder_GetListenAddr(t *testing.T) {
	f := NewRelayForwarder("r1", ":9090", "10.0.0.1:80", nil, ForwarderSection{}, nil, nil, nil)
	if got := f.GetListenAddr(); got != ":9090" {
		t.Errorf("GetListenAddr() = %q, want %q", got, ":9090")
	}
}

func TestRelayForwarder_StopBeforeStart(t *testing.T) {
	f := NewRelayForwarder("r1", "127.0.0.1:0", "10.0.0.1:80", nil, ForwarderSection{}, nil, nil, nil)
	// Should not panic
	f.Stop()
}

func TestRelayForwarder_WaitForConnAck_Timeout(t *testing.T) {
	cfg := ForwarderSection{ConnectTimeout: 1}
	f := NewRelayForwarder("r1", ":0", "10.0.0.1:80", nil, cfg, nil, nil, nil)

	stream := &relay.Stream{
		DataCh:  make(chan []byte, 1),
		CloseCh: make(chan struct{}),
	}

	result := f.waitForConnAck(stream)
	if result {
		t.Error("expected false (timeout), got true")
	}
}

func TestRelayForwarder_WaitForConnAck_Success(t *testing.T) {
	cfg := ForwarderSection{ConnectTimeout: 5}
	f := NewRelayForwarder("r1", ":0", "10.0.0.1:80", nil, cfg, nil, nil, nil)

	stream := &relay.Stream{
		DataCh:  make(chan []byte, 1),
		CloseCh: make(chan struct{}),
	}

	go func() {
		stream.DataCh <- []byte{relay.MsgTypeConnAck}
	}()

	result := f.waitForConnAck(stream)
	if !result {
		t.Error("expected true (ConnAck), got false")
	}
}

func TestRelayForwarder_WaitForConnAck_Error(t *testing.T) {
	cfg := ForwarderSection{ConnectTimeout: 5}
	f := NewRelayForwarder("r1", ":0", "10.0.0.1:80", nil, cfg, nil, nil, nil)

	stream := &relay.Stream{
		DataCh:  make(chan []byte, 1),
		CloseCh: make(chan struct{}),
	}

	go func() {
		stream.DataCh <- []byte{relay.MsgTypeError}
	}()

	result := f.waitForConnAck(stream)
	if result {
		t.Error("expected false (error), got true")
	}
}
