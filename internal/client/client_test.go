package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockForwarder implements ForwarderInterface for testing.
type mockForwarder struct {
	configHash string
	listenAddr string
	started    bool
	stopped    bool
}

func (m *mockForwarder) Start() error {
	m.started = true
	return nil
}

func (m *mockForwarder) Stop() {
	m.stopped = true
}

func (m *mockForwarder) GetConfigHash() string {
	return m.configHash
}

func (m *mockForwarder) GetListenAddr() string {
	return m.listenAddr
}

func TestNew(t *testing.T) {
	cfg := &ClientConfig{}
	c := New(cfg)

	if c.forwarders == nil {
		t.Error("expected forwarders map to be non-nil")
	}
	if c.trafficCounter == nil {
		t.Error("expected trafficCounter to be non-nil")
	}
	if c.stopCh == nil {
		t.Error("expected stopCh to be non-nil")
	}
	if c.reconnectCh == nil {
		t.Error("expected reconnectCh to be non-nil")
	}
}

func TestComputeRuleConfigHash_Direct(t *testing.T) {
	rule := map[string]interface{}{
		"type":        "direct",
		"listen_addr": ":8080",
		"target_addr": "localhost:80",
	}

	expected := "direct::8080:localhost:80"
	if got := computeRuleConfigHash(rule); got != expected {
		t.Errorf("computeRuleConfigHash() = %q, want %q", got, expected)
	}
}

func TestComputeRuleConfigHash_Relay(t *testing.T) {
	rule := map[string]interface{}{
		"type":        "relay",
		"listen_addr": ":8080",
		"exit_addr":   "10.0.0.1:80",
		"relay_chain": []interface{}{"c1", "c2"},
	}

	expected := "relay::8080:10.0.0.1:80:c1,c2,"
	if got := computeRuleConfigHash(rule); got != expected {
		t.Errorf("computeRuleConfigHash() = %q, want %q", got, expected)
	}
}

func TestRpcCall_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      "1",
			"result":  map[string]interface{}{"ok": true},
		})
	}))
	defer server.Close()

	c := &Client{cfg: &ClientConfig{Client: ClientSection{ServerURL: server.URL}}}

	resp, err := c.rpcCall(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "test",
	})
	if err != nil {
		t.Fatalf("rpcCall error: %v", err)
	}
	if resp["id"] != "1" {
		t.Errorf("expected id '1', got %v", resp["id"])
	}
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result to be a map")
	}
	if result["ok"] != true {
		t.Errorf("expected result.ok=true, got %v", result["ok"])
	}
}

func TestRpcCall_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"jsonrpc":"2.0","id":"1","error":{"code":-32000,"message":"server error"}}`))
	}))
	defer server.Close()

	c := &Client{cfg: &ClientConfig{Client: ClientSection{ServerURL: server.URL}}}

	resp, err := c.rpcCall(map[string]interface{}{"jsonrpc": "2.0", "id": "1", "method": "test"})
	if err != nil {
		t.Fatalf("rpcCall error: %v", err)
	}
	// rpcCall does not check HTTP status, so response should still be parseable
	if resp["id"] != "1" {
		t.Errorf("expected id '1', got %v", resp["id"])
	}
}

func TestRpcCall_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-json"))
	}))
	defer server.Close()

	c := &Client{cfg: &ClientConfig{Client: ClientSection{ServerURL: server.URL}}}

	_, err := c.rpcCall(map[string]interface{}{"jsonrpc": "2.0", "id": "1", "method": "test"})
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestRpcCall_NetworkError(t *testing.T) {
	c := &Client{cfg: &ClientConfig{Client: ClientSection{ServerURL: "http://127.0.0.1:1"}}}

	_, err := c.rpcCall(map[string]interface{}{"jsonrpc": "2.0", "id": "1", "method": "test"})
	if err == nil {
		t.Error("expected error for network failure")
	}
}

func TestApplyRules_NewDirectRule(t *testing.T) {
	cfg := &ClientConfig{
		Forwarder: ForwarderSection{ConnectTimeout: 5, BufferSize: 32768},
	}
	c := New(cfg)

	rules := []interface{}{
		map[string]interface{}{
			"id":          "rule-1",
			"type":        "direct",
			"listen_addr": "127.0.0.1:0",
			"target_addr": "127.0.0.1:9999",
		},
	}

	c.applyRules(rules)

	c.mu.RLock()
	defer c.mu.RUnlock()

	if _, exists := c.forwarders["rule-1"]; !exists {
		t.Error("expected forwarder 'rule-1' to be created")
	}

	// Clean up
	for _, f := range c.forwarders {
		f.Stop()
	}
}

func TestApplyRules_RuleRemoved(t *testing.T) {
	cfg := &ClientConfig{}
	c := New(cfg)

	mock := &mockForwarder{configHash: "direct::8080:localhost:80"}
	c.forwarders["old-rule"] = mock

	// Apply empty rules
	c.applyRules([]interface{}{})

	c.mu.RLock()
	defer c.mu.RUnlock()

	if !mock.stopped {
		t.Error("expected old forwarder to be stopped")
	}
	if _, exists := c.forwarders["old-rule"]; exists {
		t.Error("expected old forwarder to be removed from map")
	}
}

func TestApplyRules_ConfigUnchanged(t *testing.T) {
	cfg := &ClientConfig{
		Forwarder: ForwarderSection{ConnectTimeout: 5, BufferSize: 32768},
	}
	c := New(cfg)

	mock := &mockForwarder{
		configHash: "direct::8080:localhost:80",
		listenAddr: ":8080",
		started:    true,
	}
	c.forwarders["rule-1"] = mock

	rules := []interface{}{
		map[string]interface{}{
			"id":          "rule-1",
			"type":        "direct",
			"listen_addr": ":8080",
			"target_addr": "localhost:80",
		},
	}

	c.applyRules(rules)

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Mock should not be stopped — config hash matches
	if mock.stopped {
		t.Error("expected forwarder NOT to be stopped (config unchanged)")
	}
	// Same mock should still be in the map
	if c.forwarders["rule-1"] != mock {
		t.Error("expected same forwarder instance to remain in map")
	}
}
