package tests

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dollarkillerx/MessageBoy/internal/relay"
	"github.com/gorilla/websocket"
)

// startEchoServer starts a TCP echo server on a random port.
// Returns the address and a cleanup function.
func startEchoServer(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

// startTestWSServer starts a test WebSocket relay server.
// Returns the ws URL and the WSServer instance.
func startTestWSServer(t *testing.T) (wsURL string, wsServer *relay.WSServer, cleanup func()) {
	t.Helper()
	wsServer = relay.NewWSServer()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsServer.HandleConnection)
	server := httptest.NewServer(mux)

	wsURL = "ws" + server.URL[4:] + "/ws"
	return wsURL, wsServer, func() { server.Close() }
}

// connectTestClient connects a WebSocket client to the relay server.
// Returns the connection.
func connectTestClient(t *testing.T, wsURL, clientID string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?client_id="+clientID, nil)
	if err != nil {
		t.Fatalf("Failed to connect client %s: %v", clientID, err)
	}
	return conn
}

// sendTunnelMsg marshals and sends a TunnelMessage over WebSocket.
func sendTunnelMsg(t *testing.T, conn *websocket.Conn, msg *relay.TunnelMessage) {
	t.Helper()
	data, err := msg.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}
}

// recvTunnelMsg reads and unmarshals a TunnelMessage from WebSocket.
func recvTunnelMsg(t *testing.T, conn *websocket.Conn, timeout time.Duration) *relay.TunnelMessage {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	defer conn.SetReadDeadline(time.Time{})
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	msg, err := relay.UnmarshalTunnelMessage(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	return msg
}

// waitForClient waits until a client is online on the server.
func waitForClient(t *testing.T, wsServer *relay.WSServer, clientID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if wsServer.IsClientOnline(clientID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("client %s did not come online within %v", clientID, timeout)
}
