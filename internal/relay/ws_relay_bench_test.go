package relay

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// BenchmarkRelayThroughput_64B benchmarks relay throughput with 64B messages
func BenchmarkRelayThroughput_64B(b *testing.B) {
	benchmarkRelayThroughput(b, 64)
}

// BenchmarkRelayThroughput_1KB benchmarks relay throughput with 1KB messages
func BenchmarkRelayThroughput_1KB(b *testing.B) {
	benchmarkRelayThroughput(b, 1024)
}

// BenchmarkRelayThroughput_32KB benchmarks relay throughput with 32KB messages
func BenchmarkRelayThroughput_32KB(b *testing.B) {
	benchmarkRelayThroughput(b, 32*1024)
}

func benchmarkRelayThroughput(b *testing.B, payloadSize int) {
	wsServer := NewWSServer()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsServer.HandleConnection)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + server.URL[4:] + "/ws"

	// Client 1 (source)
	conn1, _, err := websocket.DefaultDialer.Dial(wsURL+"?client_id=bench-source", nil)
	if err != nil {
		b.Fatal(err)
	}
	defer conn1.Close()

	// Client 2 (target)
	conn2, _, err := websocket.DefaultDialer.Dial(wsURL+"?client_id=bench-target", nil)
	if err != nil {
		b.Fatal(err)
	}
	defer conn2.Close()

	time.Sleep(50 * time.Millisecond)

	// Setup route
	streamID := uint32(1)
	connectMsg := &TunnelMessage{
		Type:     MsgTypeConnect,
		StreamID: streamID,
		Target:   "127.0.0.1:9999",
		Payload:  []byte("bench-target"),
	}
	data, _ := connectMsg.Marshal()
	conn1.WriteMessage(websocket.BinaryMessage, data)

	// Wait for route to be established + drain connect message from conn2
	time.Sleep(50 * time.Millisecond)
	conn2.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	conn2.ReadMessage()
	conn2.SetReadDeadline(time.Time{})

	// Read goroutine for conn2 (drain messages)
	go func() {
		for {
			_, _, err := conn2.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Prepare data message
	payload := make([]byte, payloadSize)
	dataMsg := &TunnelMessage{
		Type:     MsgTypeData,
		StreamID: streamID,
		Payload:  payload,
	}
	msgData, _ := dataMsg.Marshal()

	b.SetBytes(int64(payloadSize))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := conn1.WriteMessage(websocket.BinaryMessage, msgData); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkConcurrentStreams benchmarks 100 concurrent streams on a single connection
func BenchmarkConcurrentStreams(b *testing.B) {
	wsServer := NewWSServer()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsServer.HandleConnection)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + server.URL[4:] + "/ws"

	conn1, _, err := websocket.DefaultDialer.Dial(wsURL+"?client_id=stream-source", nil)
	if err != nil {
		b.Fatal(err)
	}
	defer conn1.Close()

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL+"?client_id=stream-target", nil)
	if err != nil {
		b.Fatal(err)
	}
	defer conn2.Close()

	time.Sleep(50 * time.Millisecond)

	// Setup 100 routes
	numStreams := 100
	for i := 0; i < numStreams; i++ {
		streamID := uint32(i + 1)
		connectMsg := &TunnelMessage{
			Type:     MsgTypeConnect,
			StreamID: streamID,
			Target:   "127.0.0.1:9999",
			Payload:  []byte("stream-target"),
		}
		data, _ := connectMsg.Marshal()
		conn1.WriteMessage(websocket.BinaryMessage, data)
	}

	time.Sleep(100 * time.Millisecond)

	// Drain connect messages from conn2
	conn2.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	for i := 0; i < numStreams; i++ {
		conn2.ReadMessage()
	}
	conn2.SetReadDeadline(time.Time{})

	// Reader goroutine
	go func() {
		for {
			_, _, err := conn2.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	payload := make([]byte, 1024)

	b.ResetTimer()
	b.SetBytes(int64(1024 * numStreams))

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		for s := 0; s < numStreams; s++ {
			wg.Add(1)
			go func(streamID uint32) {
				defer wg.Done()
				msg := &TunnelMessage{
					Type:     MsgTypeData,
					StreamID: streamID,
					Payload:  payload,
				}
				data, _ := msg.Marshal()
				conn1.WriteMessage(websocket.BinaryMessage, data)
			}(uint32(s + 1))
		}
		wg.Wait()
	}
}

// BenchmarkRelayOverhead measures overhead of marshal vs marshal+encrypt
func BenchmarkRelayOverhead_MarshalOnly(b *testing.B) {
	payload := make([]byte, 1024)
	msg := &TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 1,
		Payload:  payload,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, _, err := msg.MarshalBinary()
		if err != nil {
			b.Fatal(err)
		}
		PutBuffer(buf)
	}
}

// BenchmarkRelayOverhead_MarshalAndEncrypt measures marshal + encrypt overhead
func BenchmarkRelayOverhead_MarshalAndEncrypt(b *testing.B) {
	payload := make([]byte, 1024)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Encrypt
		ciphertext, nonce, err := sharedCrypto.Encrypt(payload)
		if err != nil {
			b.Fatal(err)
		}
		encPayload := make([]byte, nonceSize+len(ciphertext))
		copy(encPayload[:nonceSize], nonce)
		copy(encPayload[nonceSize:], ciphertext)

		msg := &TunnelMessage{
			Type:     MsgTypeData,
			StreamID: 1,
			Payload:  encPayload,
		}
		buf, _, err := msg.MarshalBinary()
		if err != nil {
			b.Fatal(err)
		}
		PutBuffer(buf)
	}
}
