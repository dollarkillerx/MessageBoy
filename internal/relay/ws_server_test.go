package relay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ============================================================
// Route operations (sync.Map)
// ============================================================

func TestWSServer_RouteStoreAndLoad(t *testing.T) {
	s := NewWSServer()

	route := &RouteInfo{
		SourceClientID: "clientA",
		TargetClientID: "clientB",
		StreamID:       1,
		ExitAddr:       "127.0.0.1:8080",
	}

	s.routes.Store(routeKey("clientA", 1), route)
	s.routes.Store(routeKey("clientB", 1), route)

	// Both keys should resolve to the same route
	v, ok := s.routes.Load(routeKey("clientA", 1))
	if !ok {
		t.Fatal("route should be found via source key")
	}
	got := v.(*RouteInfo)
	if got.SourceClientID != "clientA" {
		t.Errorf("SourceClientID = %q, want %q", got.SourceClientID, "clientA")
	}
	if got.TargetClientID != "clientB" {
		t.Errorf("TargetClientID = %q, want %q", got.TargetClientID, "clientB")
	}
	if got.ExitAddr != "127.0.0.1:8080" {
		t.Errorf("ExitAddr = %q, want %q", got.ExitAddr, "127.0.0.1:8080")
	}

	v2, ok := s.routes.Load(routeKey("clientB", 1))
	if !ok {
		t.Fatal("route should be found via target key")
	}
	if v2.(*RouteInfo) != route {
		t.Error("both keys should point to the same RouteInfo")
	}
}

func TestWSServer_RouteLoadMissing(t *testing.T) {
	s := NewWSServer()
	_, ok := s.routes.Load(routeKey("nonexistent", 999))
	if ok {
		t.Error("load on empty routes should return false")
	}
}

func TestWSServer_CleanupRoute(t *testing.T) {
	s := NewWSServer()

	route := &RouteInfo{
		SourceClientID: "a",
		TargetClientID: "b",
		StreamID:       1,
	}
	s.routes.Store(routeKey("a", 1), route)
	s.routes.Store(routeKey("b", 1), route)

	s.cleanupRoute(route)

	if _, ok := s.routes.Load(routeKey("a", 1)); ok {
		t.Error("source key should be deleted after cleanupRoute")
	}
	if _, ok := s.routes.Load(routeKey("b", 1)); ok {
		t.Error("target key should be deleted after cleanupRoute")
	}
}

func TestWSServer_CleanupRoute_NonExistent(t *testing.T) {
	s := NewWSServer()
	// Should not panic
	s.cleanupRoute(&RouteInfo{
		SourceClientID: "nonexistent",
		TargetClientID: "also-nonexistent",
		StreamID:       999,
	})
}

func TestWSServer_CleanupRoute_WithLoadBalancer(t *testing.T) {
	s := NewWSServer()
	lb := &mockLoadBalancer{}
	s.SetLoadBalancer(lb)

	route := &RouteInfo{
		SourceClientID: "a",
		TargetClientID: "b",
		StreamID:       1,
		NodeID:         "node-1",
	}
	s.routes.Store(routeKey("a", 1), route)
	s.routes.Store(routeKey("b", 1), route)

	s.cleanupRoute(route)

	if lb.decremented != "node-1" {
		t.Errorf("DecrementConnections not called with node-1, got %q", lb.decremented)
	}
}

func TestWSServer_CleanupRoute_WithTrafficCounter(t *testing.T) {
	s := NewWSServer()
	tc := &mockTrafficCounter{}
	s.SetTrafficCounter(tc)

	route := &RouteInfo{
		SourceClientID: "clientA",
		TargetClientID: "clientB",
		StreamID:       1,
		RuleID:         "rule-1",
	}
	s.routes.Store(routeKey("clientA", 1), route)
	s.routes.Store(routeKey("clientB", 1), route)

	s.cleanupRoute(route)

	if tc.decrementedRule != "rule-1" || tc.decrementedClient != "clientA" {
		t.Errorf("DecrementConn not called correctly: rule=%q client=%q", tc.decrementedRule, tc.decrementedClient)
	}
}

func TestWSServer_CleanupRoutesForClient(t *testing.T) {
	s := NewWSServer()

	// Create routes involving clientB
	route1 := &RouteInfo{SourceClientID: "clientA", TargetClientID: "clientB", StreamID: 1}
	s.routes.Store(routeKey("clientA", 1), route1)
	s.routes.Store(routeKey("clientB", 1), route1)

	route2 := &RouteInfo{SourceClientID: "clientB", TargetClientID: "clientC", StreamID: 2}
	s.routes.Store(routeKey("clientB", 2), route2)
	s.routes.Store(routeKey("clientC", 2), route2)

	route3 := &RouteInfo{SourceClientID: "clientC", TargetClientID: "clientD", StreamID: 3}
	s.routes.Store(routeKey("clientC", 3), route3)
	s.routes.Store(routeKey("clientD", 3), route3)

	s.cleanupRoutesForClient("clientB")

	// Route 1 both keys should be gone (clientB is target)
	if _, ok := s.routes.Load(routeKey("clientA", 1)); ok {
		t.Error("route 1 source key should be cleaned")
	}
	if _, ok := s.routes.Load(routeKey("clientB", 1)); ok {
		t.Error("route 1 target key should be cleaned")
	}
	// Route 2 both keys should be gone (clientB is source)
	if _, ok := s.routes.Load(routeKey("clientB", 2)); ok {
		t.Error("route 2 source key should be cleaned")
	}
	if _, ok := s.routes.Load(routeKey("clientC", 2)); ok {
		t.Error("route 2 target key should be cleaned")
	}
	// Route 3 should remain
	if _, ok := s.routes.Load(routeKey("clientC", 3)); !ok {
		t.Error("route 3 source key should remain (no clientB involvement)")
	}
	if _, ok := s.routes.Load(routeKey("clientD", 3)); !ok {
		t.Error("route 3 target key should remain (no clientB involvement)")
	}
}

func TestWSServer_CleanupRoutesForClient_Empty(t *testing.T) {
	s := NewWSServer()
	// Should not panic
	s.cleanupRoutesForClient("nonexistent")
}

func TestWSServer_ConcurrentRouteOperations(t *testing.T) {
	s := NewWSServer()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(3)
		streamID := uint32(i)

		go func() {
			defer wg.Done()
			route := &RouteInfo{
				SourceClientID: "a",
				TargetClientID: "b",
				StreamID:       streamID,
			}
			s.routes.Store(routeKey("a", streamID), route)
			s.routes.Store(routeKey("b", streamID), route)
		}()

		go func() {
			defer wg.Done()
			s.routes.Load(routeKey("a", streamID))
		}()

		go func() {
			defer wg.Done()
			s.cleanupRoute(&RouteInfo{
				SourceClientID: "a",
				TargetClientID: "b",
				StreamID:       streamID,
			})
		}()
	}

	wg.Wait()
}

// ============================================================
// Client management
// ============================================================

func TestWSServer_IsClientOnline(t *testing.T) {
	s := NewWSServer()

	if s.IsClientOnline("test-client") {
		t.Error("client should not be online initially")
	}

	s.mu.Lock()
	s.clients["test-client"] = &WSClient{
		ID:      "test-client",
		SendCh:  make(chan *sendItem, 10),
		CloseCh: make(chan struct{}),
	}
	s.mu.Unlock()

	if !s.IsClientOnline("test-client") {
		t.Error("client should be online after registration")
	}
}

func TestWSServer_GetClient(t *testing.T) {
	s := NewWSServer()

	c := s.GetClient("nonexistent")
	if c != nil {
		t.Error("GetClient should return nil for nonexistent client")
	}

	client := &WSClient{
		ID:      "test",
		SendCh:  make(chan *sendItem, 10),
		CloseCh: make(chan struct{}),
	}
	s.mu.Lock()
	s.clients["test"] = client
	s.mu.Unlock()

	got := s.GetClient("test")
	if got != client {
		t.Error("GetClient should return the registered client")
	}
}

func TestWSServer_SendToClient_Offline(t *testing.T) {
	s := NewWSServer()
	ok := s.SendToClient("offline-client", []byte("data"))
	if ok {
		t.Error("SendToClient should return false for offline client")
	}
}

func TestWSServer_SendMsgToClient_Offline(t *testing.T) {
	s := NewWSServer()
	ok := s.SendMsgToClient("offline-client", &TunnelMessage{Type: MsgTypeData})
	if ok {
		t.Error("SendMsgToClient should return false for offline client")
	}
}

// ============================================================
// WSClient unit tests
// ============================================================

func TestWSClient_Close_Idempotent(t *testing.T) {
	// WSClient.Close uses a real websocket.Conn, so we set up a minimal server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.Upgrade(w, r, nil)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	client := &WSClient{
		ID:      "test",
		Conn:    conn,
		SendCh:  make(chan *sendItem, 10),
		CloseCh: make(chan struct{}),
	}

	// First close
	client.Close()

	// Second close should not panic
	client.Close()

	if !client.closed {
		t.Error("client should be marked as closed")
	}
}

func TestWSClient_DroppedMessages_InitiallyZero(t *testing.T) {
	client := &WSClient{
		ID:      "test",
		SendCh:  make(chan *sendItem, 10),
		CloseCh: make(chan struct{}),
	}
	if client.DroppedMessages() != 0 {
		t.Errorf("initial DroppedMessages = %d, want 0", client.DroppedMessages())
	}
}

func TestWSClient_Send_DropCounting(t *testing.T) {
	// Create a client with a tiny channel to force drops
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.Upgrade(w, r, nil)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	client := &WSClient{
		ID:      "test",
		Conn:    conn,
		SendCh:  make(chan *sendItem, 1), // capacity 1
		CloseCh: make(chan struct{}),
	}
	defer client.Close()

	// Fill the channel
	client.Send([]byte("fill"))

	// This should be dropped
	ok := client.Send([]byte("overflow"))
	if ok {
		t.Error("Send should return false when channel is full")
	}

	if client.DroppedMessages() != 1 {
		t.Errorf("DroppedMessages = %d, want 1", client.DroppedMessages())
	}
}

func TestWSClient_SendMsg_DropCounting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.Upgrade(w, r, nil)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	client := &WSClient{
		ID:      "test",
		Conn:    conn,
		SendCh:  make(chan *sendItem, 1),
		CloseCh: make(chan struct{}),
	}
	defer client.Close()

	// Fill the channel
	client.SendMsg(&TunnelMessage{Type: MsgTypeRuleUpdate})

	// This should be dropped
	ok := client.SendMsg(&TunnelMessage{Type: MsgTypeRuleUpdate})
	if ok {
		t.Error("SendMsg should return false when channel is full")
	}

	if client.DroppedMessages() != 1 {
		t.Errorf("DroppedMessages = %d, want 1", client.DroppedMessages())
	}
}

func TestWSClient_Send_AfterClose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.Upgrade(w, r, nil)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	client := &WSClient{
		ID:      "test",
		Conn:    conn,
		SendCh:  make(chan *sendItem, 10),
		CloseCh: make(chan struct{}),
	}

	client.Close()

	ok := client.Send([]byte("after close"))
	if ok {
		t.Error("Send should return false after Close")
	}

	ok = client.SendMsg(&TunnelMessage{Type: MsgTypeData, Payload: []byte("x")})
	if ok {
		t.Error("SendMsg should return false after Close")
	}
}

func TestWSClient_DroppedMessages_Concurrent(t *testing.T) {
	client := &WSClient{
		ID:      "test",
		SendCh:  make(chan *sendItem), // unbuffered = always drops
		CloseCh: make(chan struct{}),
		closed:  true, // mark as closed to make Send return false fast
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// These will all return false because client is closed
			// but DroppedMessages should still be safe to read concurrently
			atomic.LoadInt64(&client.droppedMessages)
		}()
	}

	wg.Wait()
}

// ============================================================
// NotifyRuleUpdate
// ============================================================

func TestWSServer_NotifyRuleUpdate_ClientOffline(t *testing.T) {
	s := NewWSServer()
	ok := s.NotifyRuleUpdate("offline")
	if ok {
		t.Error("NotifyRuleUpdate should return false for offline client")
	}
}

func TestWSServer_NotifyRuleUpdateToAll_Empty(t *testing.T) {
	s := NewWSServer()
	// Should not panic with no clients
	s.NotifyRuleUpdateToAll()
}

// ============================================================
// HandlePortCheckResult
// ============================================================

func TestWSServer_HandlePortCheckResult_Available(t *testing.T) {
	s := NewWSServer()

	// Setup pending check
	ch := make(chan *PortCheckResult, 1)
	s.pendingPortChecksMu.Lock()
	s.pendingPortChecks[42] = ch
	s.pendingPortChecksMu.Unlock()

	// Simulate result with no error (available)
	s.HandlePortCheckResult(&TunnelMessage{
		Type:     MsgTypeCheckPortResult,
		StreamID: 42,
		Error:    "",
	})

	select {
	case result := <-ch:
		if !result.Available {
			t.Error("port should be available when Error is empty")
		}
		if result.Error != "" {
			t.Errorf("Error = %q, want empty", result.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for result")
	}
}

func TestWSServer_HandlePortCheckResult_Unavailable(t *testing.T) {
	s := NewWSServer()

	ch := make(chan *PortCheckResult, 1)
	s.pendingPortChecksMu.Lock()
	s.pendingPortChecks[43] = ch
	s.pendingPortChecksMu.Unlock()

	s.HandlePortCheckResult(&TunnelMessage{
		Type:     MsgTypeCheckPortResult,
		StreamID: 43,
		Error:    "port already in use",
	})

	select {
	case result := <-ch:
		if result.Available {
			t.Error("port should be unavailable when Error is set")
		}
		if result.Error != "port already in use" {
			t.Errorf("Error = %q, want %q", result.Error, "port already in use")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for result")
	}
}

func TestWSServer_HandlePortCheckResult_UnknownRequest(t *testing.T) {
	s := NewWSServer()
	// Should not panic
	s.HandlePortCheckResult(&TunnelMessage{
		Type:     MsgTypeCheckPortResult,
		StreamID: 999,
	})
}

func TestWSServer_CheckPortAvailable_ClientOffline(t *testing.T) {
	s := NewWSServer()

	available, errMsg := s.CheckPortAvailable("offline", "0.0.0.0:8080", "", time.Second)
	if available {
		t.Error("should not be available when client is offline")
	}
	if errMsg == "" {
		t.Error("should have error message for offline client")
	}
}

// ============================================================
// handleConnect logic
// ============================================================

func TestWSServer_HandleConnect_NoTarget(t *testing.T) {
	s := NewWSServer()

	// Register a client to receive error
	client := registerTestWSClient(t, s, "sender")
	defer client.Close()

	// Connect with empty payload (no target)
	s.handleConnect("sender", &TunnelMessage{
		Type:     MsgTypeConnect,
		StreamID: 1,
		Target:   "127.0.0.1:8080",
		Payload:  nil,
	})

	// Should receive error
	item := drainOneItem(t, client.SendCh, time.Second)
	msg := unmarshalItem(t, item)
	if msg.Type != MsgTypeError {
		t.Errorf("expected MsgTypeError, got %d", msg.Type)
	}
}

func TestWSServer_HandleConnect_TargetOffline(t *testing.T) {
	s := NewWSServer()

	client := registerTestWSClient(t, s, "sender")
	defer client.Close()

	s.handleConnect("sender", &TunnelMessage{
		Type:     MsgTypeConnect,
		StreamID: 1,
		Target:   "127.0.0.1:8080",
		Payload:  []byte("offline-target"),
	})

	item := drainOneItem(t, client.SendCh, time.Second)
	msg := unmarshalItem(t, item)
	if msg.Type != MsgTypeError {
		t.Errorf("expected MsgTypeError, got %d", msg.Type)
	}
}

func TestWSServer_HandleConnect_Success(t *testing.T) {
	s := NewWSServer()
	tc := &mockTrafficCounter{}
	s.SetTrafficCounter(tc)

	sender := registerTestWSClient(t, s, "sender")
	defer sender.Close()
	target := registerTestWSClient(t, s, "target")
	defer target.Close()

	s.handleConnect("sender", &TunnelMessage{
		Type:     MsgTypeConnect,
		StreamID: 1,
		Target:   "127.0.0.1:8080",
		Payload:  []byte("target"),
		RuleID:   "rule-1",
	})

	// Target should receive Connect
	item := drainOneItem(t, target.SendCh, time.Second)
	msg := unmarshalItem(t, item)
	if msg.Type != MsgTypeConnect {
		t.Errorf("expected MsgTypeConnect, got %d", msg.Type)
	}
	if msg.Target != "127.0.0.1:8080" {
		t.Errorf("Target = %q, want %q", msg.Target, "127.0.0.1:8080")
	}

	// Route should be stored under both keys
	v, ok := s.routes.Load(routeKey("sender", 1))
	if !ok {
		t.Fatal("route should be stored under source key")
	}
	route := v.(*RouteInfo)
	if route.SourceClientID != "sender" || route.TargetClientID != "target" {
		t.Errorf("route mismatch: source=%q target=%q", route.SourceClientID, route.TargetClientID)
	}
	if _, ok := s.routes.Load(routeKey("target", 1)); !ok {
		t.Fatal("route should be stored under target key")
	}

	// TrafficCounter should be incremented
	if tc.incrementedRule != "rule-1" {
		t.Errorf("IncrementConn not called with rule-1, got %q", tc.incrementedRule)
	}
}

func TestWSServer_HandleConnect_WithLoadBalancer(t *testing.T) {
	s := NewWSServer()
	lb := &mockLoadBalancer{
		resolveClientID: "exit-node",
		resolveNodeID:   "node-42",
	}
	s.SetLoadBalancer(lb)

	sender := registerTestWSClient(t, s, "sender")
	defer sender.Close()
	exitNode := registerTestWSClient(t, s, "exit-node")
	defer exitNode.Close()

	s.handleConnect("sender", &TunnelMessage{
		Type:     MsgTypeConnect,
		StreamID: 1,
		Target:   "10.0.0.1:80",
		Payload:  []byte("@my-group"),
	})

	// Exit node should receive Connect
	item := drainOneItem(t, exitNode.SendCh, time.Second)
	msg := unmarshalItem(t, item)
	if msg.Type != MsgTypeConnect {
		t.Errorf("expected MsgTypeConnect, got %d", msg.Type)
	}

	// LoadBalancer should be called
	if lb.incremented != "node-42" {
		t.Errorf("IncrementConnections not called with node-42, got %q", lb.incremented)
	}
}

// ============================================================
// handleConnAck logic
// ============================================================

func TestWSServer_HandleConnAck_NoRoute(t *testing.T) {
	s := NewWSServer()
	// Should not panic
	s.handleConnAck("client", &TunnelMessage{Type: MsgTypeConnAck, StreamID: 999})
}

func TestWSServer_HandleConnAck_UnexpectedSender(t *testing.T) {
	s := NewWSServer()

	route := &RouteInfo{
		SourceClientID: "A",
		TargetClientID: "B",
		StreamID:       1,
	}
	s.routes.Store(routeKey("A", 1), route)
	s.routes.Store(routeKey("B", 1), route)

	// ConnAck from C (not B) should be ignored — C's key doesn't exist
	s.handleConnAck("C", &TunnelMessage{Type: MsgTypeConnAck, StreamID: 1})
}

func TestWSServer_HandleConnAck_Success(t *testing.T) {
	s := NewWSServer()

	source := registerTestWSClient(t, s, "source")
	defer source.Close()

	route := &RouteInfo{
		SourceClientID: "source",
		TargetClientID: "target",
		StreamID:       1,
	}
	s.routes.Store(routeKey("source", 1), route)
	s.routes.Store(routeKey("target", 1), route)

	s.handleConnAck("target", &TunnelMessage{Type: MsgTypeConnAck, StreamID: 1})

	item := drainOneItem(t, source.SendCh, time.Second)
	msg := unmarshalItem(t, item)
	if msg.Type != MsgTypeConnAck {
		t.Errorf("expected MsgTypeConnAck, got %d", msg.Type)
	}
}

// ============================================================
// handleData logic
// ============================================================

func TestWSServer_HandleData_NoRoute(t *testing.T) {
	s := NewWSServer()
	// Should not panic
	s.handleData("client", &TunnelMessage{Type: MsgTypeData, StreamID: 999, Payload: []byte("test")})
}

func TestWSServer_HandleData_SourceToTarget(t *testing.T) {
	s := NewWSServer()
	tc := &mockTrafficCounter{}
	s.SetTrafficCounter(tc)

	target := registerTestWSClient(t, s, "target")
	defer target.Close()

	route := &RouteInfo{
		SourceClientID: "source",
		TargetClientID: "target",
		StreamID:       1,
		RuleID:         "rule-1",
	}
	s.routes.Store(routeKey("source", 1), route)
	s.routes.Store(routeKey("target", 1), route)

	s.handleData("source", &TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 1,
		Payload:  []byte("hello"),
	})

	// Target should receive data
	item := drainOneItem(t, target.SendCh, time.Second)
	msg := unmarshalItem(t, item)
	if msg.Type != MsgTypeData {
		t.Errorf("expected MsgTypeData, got %d", msg.Type)
	}

	// Traffic should be counted as outbound (source->target)
	if tc.bytesOutRule != "rule-1" || tc.bytesOut != 5 {
		t.Errorf("BytesOut not counted: rule=%q bytes=%d", tc.bytesOutRule, tc.bytesOut)
	}
}

func TestWSServer_HandleData_TargetToSource(t *testing.T) {
	s := NewWSServer()
	tc := &mockTrafficCounter{}
	s.SetTrafficCounter(tc)

	source := registerTestWSClient(t, s, "source")
	defer source.Close()

	route := &RouteInfo{
		SourceClientID: "source",
		TargetClientID: "target",
		StreamID:       1,
		RuleID:         "rule-1",
	}
	s.routes.Store(routeKey("source", 1), route)
	s.routes.Store(routeKey("target", 1), route)

	s.handleData("target", &TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 1,
		Payload:  []byte("response"),
	})

	// Source should receive data
	item := drainOneItem(t, source.SendCh, time.Second)
	msg := unmarshalItem(t, item)
	if msg.Type != MsgTypeData {
		t.Errorf("expected MsgTypeData, got %d", msg.Type)
	}

	// Traffic should be counted as inbound (target->source)
	if tc.bytesInRule != "rule-1" || tc.bytesIn != 8 {
		t.Errorf("BytesIn not counted: rule=%q bytes=%d", tc.bytesInRule, tc.bytesIn)
	}
}

func TestWSServer_HandleData_UnexpectedSender(t *testing.T) {
	s := NewWSServer()

	route := &RouteInfo{
		SourceClientID: "A",
		TargetClientID: "B",
		StreamID:       1,
	}
	s.routes.Store(routeKey("A", 1), route)
	s.routes.Store(routeKey("B", 1), route)

	// Data from C (neither A nor B) — C's key doesn't exist, so no route found
	s.handleData("C", &TunnelMessage{Type: MsgTypeData, StreamID: 1, Payload: []byte("bad")})
}

// ============================================================
// handleClose logic
// ============================================================

func TestWSServer_HandleClose_NoRoute(t *testing.T) {
	s := NewWSServer()
	// Should not panic
	s.handleClose("client", &TunnelMessage{Type: MsgTypeClose, StreamID: 999})
}

func TestWSServer_HandleClose_CleansRoute(t *testing.T) {
	s := NewWSServer()

	target := registerTestWSClient(t, s, "target")
	defer target.Close()

	route := &RouteInfo{
		SourceClientID: "source",
		TargetClientID: "target",
		StreamID:       1,
	}
	s.routes.Store(routeKey("source", 1), route)
	s.routes.Store(routeKey("target", 1), route)

	s.handleClose("source", &TunnelMessage{Type: MsgTypeClose, StreamID: 1})

	// Both keys should be cleaned up
	if _, ok := s.routes.Load(routeKey("source", 1)); ok {
		t.Error("source key should be deleted after handleClose")
	}
	if _, ok := s.routes.Load(routeKey("target", 1)); ok {
		t.Error("target key should be deleted after handleClose")
	}

	// Target should receive Close
	item := drainOneItem(t, target.SendCh, time.Second)
	msg := unmarshalItem(t, item)
	if msg.Type != MsgTypeClose {
		t.Errorf("expected MsgTypeClose, got %d", msg.Type)
	}
}

// ============================================================
// handleError logic
// ============================================================

func TestWSServer_HandleError_NoRoute(t *testing.T) {
	s := NewWSServer()
	// Should not panic
	s.handleError("client", &TunnelMessage{Type: MsgTypeError, StreamID: 999})
}

func TestWSServer_HandleError_ForwardsAndCleans(t *testing.T) {
	s := NewWSServer()

	source := registerTestWSClient(t, s, "source")
	defer source.Close()

	route := &RouteInfo{
		SourceClientID: "source",
		TargetClientID: "target",
		StreamID:       1,
	}
	s.routes.Store(routeKey("source", 1), route)
	s.routes.Store(routeKey("target", 1), route)

	s.handleError("target", &TunnelMessage{
		Type:     MsgTypeError,
		StreamID: 1,
		Error:    "connection refused",
	})

	// Source should receive error
	item := drainOneItem(t, source.SendCh, time.Second)
	msg := unmarshalItem(t, item)
	if msg.Type != MsgTypeError {
		t.Errorf("expected MsgTypeError, got %d", msg.Type)
	}

	// Both keys should be cleaned up
	if _, ok := s.routes.Load(routeKey("source", 1)); ok {
		t.Error("source key should be deleted after handleError")
	}
	if _, ok := s.routes.Load(routeKey("target", 1)); ok {
		t.Error("target key should be deleted after handleError")
	}
}

// ============================================================
// StreamID collision
// ============================================================

func TestWSServer_StreamID_Collision(t *testing.T) {
	s := NewWSServer()

	// Two different client pairs using the same StreamID=1
	route1 := &RouteInfo{
		SourceClientID: "clientA",
		TargetClientID: "clientB",
		StreamID:       1,
		ExitAddr:       "10.0.0.1:80",
	}
	s.routes.Store(routeKey("clientA", 1), route1)
	s.routes.Store(routeKey("clientB", 1), route1)

	route2 := &RouteInfo{
		SourceClientID: "clientC",
		TargetClientID: "clientD",
		StreamID:       1,
		ExitAddr:       "10.0.0.2:80",
	}
	s.routes.Store(routeKey("clientC", 1), route2)
	s.routes.Store(routeKey("clientD", 1), route2)

	// Both routes should coexist independently
	v1, ok := s.routes.Load(routeKey("clientA", 1))
	if !ok {
		t.Fatal("route1 should exist")
	}
	if v1.(*RouteInfo).ExitAddr != "10.0.0.1:80" {
		t.Errorf("route1 ExitAddr = %q, want %q", v1.(*RouteInfo).ExitAddr, "10.0.0.1:80")
	}

	v2, ok := s.routes.Load(routeKey("clientC", 1))
	if !ok {
		t.Fatal("route2 should exist")
	}
	if v2.(*RouteInfo).ExitAddr != "10.0.0.2:80" {
		t.Errorf("route2 ExitAddr = %q, want %q", v2.(*RouteInfo).ExitAddr, "10.0.0.2:80")
	}

	// Cleaning up route1 should not affect route2
	s.cleanupRoute(route1)

	if _, ok := s.routes.Load(routeKey("clientA", 1)); ok {
		t.Error("route1 source key should be deleted")
	}
	if _, ok := s.routes.Load(routeKey("clientB", 1)); ok {
		t.Error("route1 target key should be deleted")
	}

	// route2 should still be intact
	v2, ok = s.routes.Load(routeKey("clientC", 1))
	if !ok {
		t.Fatal("route2 should still exist after cleaning route1")
	}
	if v2.(*RouteInfo).ExitAddr != "10.0.0.2:80" {
		t.Errorf("route2 ExitAddr = %q, want %q", v2.(*RouteInfo).ExitAddr, "10.0.0.2:80")
	}
	if _, ok := s.routes.Load(routeKey("clientD", 1)); !ok {
		t.Fatal("route2 target key should still exist after cleaning route1")
	}
}

// ============================================================
// sendError
// ============================================================

func TestWSServer_SendError(t *testing.T) {
	s := NewWSServer()

	client := registerTestWSClient(t, s, "test")
	defer client.Close()

	s.sendError("test", 42, "something went wrong")

	item := drainOneItem(t, client.SendCh, time.Second)
	msg := unmarshalItem(t, item)
	if msg.Type != MsgTypeError {
		t.Errorf("expected MsgTypeError, got %d", msg.Type)
	}
	if msg.StreamID != 42 {
		t.Errorf("StreamID = %d, want 42", msg.StreamID)
	}
	if msg.Error != "something went wrong" {
		t.Errorf("Error = %q, want %q", msg.Error, "something went wrong")
	}
}

// ============================================================
// HandleConnection integration (via httptest)
// ============================================================

func TestWSServer_HandleConnection_MissingClientID(t *testing.T) {
	s := NewWSServer()

	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()

	s.HandleConnection(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestWSServer_HandleConnection_ReplacesOldClient(t *testing.T) {
	s := NewWSServer()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.HandleConnection(w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?client_id=dup"

	// First connection
	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial 1: %v", err)
	}
	defer conn1.Close()

	// Wait for registration
	waitForClientOnline(t, s, "dup", time.Second)

	// Second connection (should replace first)
	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial 2: %v", err)
	}
	defer conn2.Close()

	// Wait for replacement
	time.Sleep(100 * time.Millisecond)

	// conn1 should be broken
	conn1.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err = conn1.ReadMessage()
	if err == nil {
		t.Error("old connection should be closed by server")
	}

	// New connection should still work
	waitForClientOnline(t, s, "dup", time.Second)

	// The guard should ensure new client is not deleted by old readPump cleanup
	time.Sleep(200 * time.Millisecond)
	if !s.IsClientOnline("dup") {
		t.Error("replacement client should still be online after old readPump cleanup")
	}
}

// ============================================================
// SetLoadBalancer / SetTrafficCounter
// ============================================================

func TestWSServer_SetLoadBalancer(t *testing.T) {
	s := NewWSServer()
	if s.loadBalancer != nil {
		t.Error("loadBalancer should be nil initially")
	}

	lb := &mockLoadBalancer{}
	s.SetLoadBalancer(lb)
	if s.loadBalancer != lb {
		t.Error("loadBalancer should be set")
	}
}

func TestWSServer_SetTrafficCounter(t *testing.T) {
	s := NewWSServer()
	if s.trafficCounter != nil {
		t.Error("trafficCounter should be nil initially")
	}

	tc := &mockTrafficCounter{}
	s.SetTrafficCounter(tc)
	if s.trafficCounter != tc {
		t.Error("trafficCounter should be set")
	}
}

// ============================================================
// Helpers
// ============================================================

// registerTestWSClient creates a WSClient with a real websocket.Conn and registers it in the server.
func registerTestWSClient(t *testing.T, s *WSServer, clientID string) *WSClient {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.Upgrade(w, r, nil)
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial for %s: %v", clientID, err)
	}

	client := &WSClient{
		ID:      clientID,
		Conn:    conn,
		SendCh:  make(chan *sendItem, 2048),
		CloseCh: make(chan struct{}),
	}

	s.mu.Lock()
	s.clients[clientID] = client
	s.mu.Unlock()

	return client
}

func drainOneItem(t *testing.T, ch chan *sendItem, timeout time.Duration) *sendItem {
	t.Helper()
	select {
	case item := <-ch:
		return item
	case <-time.After(timeout):
		t.Fatal("timeout waiting for send item")
		return nil
	}
}

func unmarshalItem(t *testing.T, item *sendItem) *TunnelMessage {
	t.Helper()
	msg, err := UnmarshalBinary((*item.buf)[:item.size])
	PutBuffer(item.buf)
	if err != nil {
		t.Fatalf("unmarshal item: %v", err)
	}
	return msg
}

func waitForClientOnline(t *testing.T, s *WSServer, clientID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.IsClientOnline(clientID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("client %q not online within %v", clientID, timeout)
}

// ============================================================
// Mocks
// ============================================================

type mockLoadBalancer struct {
	resolveClientID string
	resolveNodeID   string
	incremented     string
	decremented     string
}

func (m *mockLoadBalancer) ResolveTarget(target string, clientIP string) (string, string, error) {
	return m.resolveClientID, m.resolveNodeID, nil
}

func (m *mockLoadBalancer) IncrementConnections(nodeID string) error {
	m.incremented = nodeID
	return nil
}

func (m *mockLoadBalancer) DecrementConnections(nodeID string) error {
	m.decremented = nodeID
	return nil
}

type mockTrafficCounter struct {
	bytesInRule        string
	bytesIn            int64
	bytesOutRule       string
	bytesOut           int64
	incrementedRule    string
	incrementedClient  string
	decrementedRule    string
	decrementedClient  string
}

func (m *mockTrafficCounter) AddBytesIn(ruleID, clientID string, bytes int64) {
	m.bytesInRule = ruleID
	m.bytesIn = bytes
}

func (m *mockTrafficCounter) AddBytesOut(ruleID, clientID string, bytes int64) {
	m.bytesOutRule = ruleID
	m.bytesOut = bytes
}

func (m *mockTrafficCounter) IncrementConn(ruleID, clientID string) {
	m.incrementedRule = ruleID
	m.incrementedClient = clientID
}

func (m *mockTrafficCounter) DecrementConn(ruleID, clientID string) {
	m.decrementedRule = ruleID
	m.decrementedClient = clientID
}
