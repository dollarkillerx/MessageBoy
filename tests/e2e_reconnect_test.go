package tests

import (
	"testing"
	"time"

	"github.com/dollarkillerx/MessageBoy/internal/relay"
)

// TestE2E_ClientReconnection tests that after reconnecting,
// a client can still receive RuleUpdate notifications.
func TestE2E_ClientReconnection(t *testing.T) {
	wsURL, wsServer, cleanup := startTestWSServer(t)
	defer cleanup()

	// First connection
	conn1 := connectTestClient(t, wsURL, "reconnect-client")
	waitForClient(t, wsServer, "reconnect-client", time.Second)

	// Send a rule update - should succeed
	if !wsServer.NotifyRuleUpdate("reconnect-client") {
		t.Error("first NotifyRuleUpdate should succeed")
	}

	// Receive the notification
	msg1 := recvTunnelMsg(t, conn1, 2*time.Second)
	if msg1.Type != relay.MsgTypeRuleUpdate {
		t.Errorf("expected MsgTypeRuleUpdate, got %d", msg1.Type)
	}

	// Disconnect
	conn1.Close()
	time.Sleep(200 * time.Millisecond)

	// Client should be offline
	if wsServer.IsClientOnline("reconnect-client") {
		t.Error("client should be offline after disconnect")
	}

	// Reconnect
	conn2 := connectTestClient(t, wsURL, "reconnect-client")
	defer conn2.Close()
	waitForClient(t, wsServer, "reconnect-client", time.Second)

	// Send another rule update - should succeed on new connection
	if !wsServer.NotifyRuleUpdate("reconnect-client") {
		t.Error("second NotifyRuleUpdate should succeed")
	}

	msg2 := recvTunnelMsg(t, conn2, 2*time.Second)
	if msg2.Type != relay.MsgTypeRuleUpdate {
		t.Errorf("expected MsgTypeRuleUpdate after reconnect, got %d", msg2.Type)
	}
}

// TestE2E_ReconnectionOverridesOldConnection tests that a new connection
// from the same client_id properly replaces the old one.
func TestE2E_ReconnectionOverridesOldConnection(t *testing.T) {
	wsURL, wsServer, cleanup := startTestWSServer(t)
	defer cleanup()

	conn1 := connectTestClient(t, wsURL, "override-client")
	waitForClient(t, wsServer, "override-client", time.Second)

	// Connect again with same client_id (without closing conn1 first)
	conn2 := connectTestClient(t, wsURL, "override-client")
	defer conn2.Close()

	// Old connection should be broken
	conn1.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err := conn1.ReadMessage()
	if err == nil {
		t.Error("old connection should be closed by server")
	}
	conn1.Close()

	// Wait for the old readPump cleanup to finish and new one to be established
	// The old readPump will delete the client, but the new connection's
	// HandleConnection already registered it. We need to wait for stabilization.
	waitForClient(t, wsServer, "override-client", 2*time.Second)

	// New connection should work
	if !wsServer.NotifyRuleUpdate("override-client") {
		t.Error("NotifyRuleUpdate to new connection should succeed")
	}

	msg := recvTunnelMsg(t, conn2, 2*time.Second)
	if msg.Type != relay.MsgTypeRuleUpdate {
		t.Errorf("expected MsgTypeRuleUpdate, got %d", msg.Type)
	}
}

// TestE2E_RouteCleanupAfterReconnect verifies that routes from
// the old connection are cleaned up when a client reconnects.
func TestE2E_RouteCleanupAfterReconnect(t *testing.T) {
	wsURL, wsServer, cleanup := startTestWSServer(t)
	defer cleanup()

	connA := connectTestClient(t, wsURL, "routeA")
	defer connA.Close()
	connB := connectTestClient(t, wsURL, "routeB")

	waitForClient(t, wsServer, "routeA", time.Second)
	waitForClient(t, wsServer, "routeB", time.Second)

	// Setup a route from A to B
	sendTunnelMsg(t, connA, &relay.TunnelMessage{
		Type:     relay.MsgTypeConnect,
		StreamID: 42,
		Target:   "127.0.0.1:9999",
		Payload:  []byte("routeB"),
	})
	recvTunnelMsg(t, connB, 2*time.Second) // drain connect on B

	// Disconnect B
	connB.Close()
	time.Sleep(200 * time.Millisecond)

	// Reconnect B
	connB2 := connectTestClient(t, wsURL, "routeB")
	defer connB2.Close()
	waitForClient(t, wsServer, "routeB", time.Second)

	// Send data on the old route - should fail silently (route cleaned up)
	sendTunnelMsg(t, connA, &relay.TunnelMessage{
		Type:     relay.MsgTypeData,
		StreamID: 42,
		Payload:  []byte("old route data"),
	})

	// B2 should NOT receive this data (route was cleaned up on disconnect)
	connB2.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err := connB2.ReadMessage()
	if err == nil {
		t.Error("reconnected client should not receive data on old routes")
	}
}
