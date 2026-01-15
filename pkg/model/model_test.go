package model

import (
	"encoding/json"
	"testing"
)

func TestClientStatus(t *testing.T) {
	if ClientStatusOnline != "online" {
		t.Error("ClientStatusOnline should be 'online'")
	}
	if ClientStatusOffline != "offline" {
		t.Error("ClientStatusOffline should be 'offline'")
	}
}

func TestForwardType(t *testing.T) {
	if ForwardTypeDirect != "direct" {
		t.Error("ForwardTypeDirect should be 'direct'")
	}
	if ForwardTypeRelay != "relay" {
		t.Error("ForwardTypeRelay should be 'relay'")
	}
}

func TestLoadBalanceMethod(t *testing.T) {
	methods := []LoadBalanceMethod{
		LoadBalanceRoundRobin,
		LoadBalanceRandom,
		LoadBalanceLeastConn,
		LoadBalanceIPHash,
	}

	expected := []string{"round_robin", "random", "least_conn", "ip_hash"}

	for i, m := range methods {
		if string(m) != expected[i] {
			t.Errorf("LoadBalanceMethod %d: got %s, want %s", i, m, expected[i])
		}
	}
}

func TestNodeStatus(t *testing.T) {
	if NodeStatusHealthy != "healthy" {
		t.Error("NodeStatusHealthy should be 'healthy'")
	}
	if NodeStatusUnhealthy != "unhealthy" {
		t.Error("NodeStatusUnhealthy should be 'unhealthy'")
	}
	if NodeStatusUnknown != "unknown" {
		t.Error("NodeStatusUnknown should be 'unknown'")
	}
}

func TestRuleStatus(t *testing.T) {
	statuses := []RuleStatus{
		RuleStatusPending,
		RuleStatusRunning,
		RuleStatusError,
		RuleStatusStopped,
	}

	expected := []string{"pending", "running", "error", "stopped"}

	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("RuleStatus %d: got %s, want %s", i, s, expected[i])
		}
	}
}

func TestClientSetDefaults(t *testing.T) {
	c := &Client{}
	c.SetDefaults()

	if c.SSHPort != 22 {
		t.Errorf("expected SSHPort 22, got %d", c.SSHPort)
	}
	if c.Status != ClientStatusOffline {
		t.Errorf("expected Status 'offline', got '%s'", c.Status)
	}

	// 测试已设置值不被覆盖
	c2 := &Client{SSHPort: 2222, Status: ClientStatusOnline}
	c2.SetDefaults()

	if c2.SSHPort != 2222 {
		t.Errorf("expected SSHPort 2222, got %d", c2.SSHPort)
	}
	if c2.Status != ClientStatusOnline {
		t.Errorf("expected Status 'online', got '%s'", c2.Status)
	}
}

func TestForwardRuleSetDefaults(t *testing.T) {
	r := &ForwardRule{}
	r.SetDefaults()

	if r.Type != ForwardTypeDirect {
		t.Errorf("expected Type 'direct', got '%s'", r.Type)
	}

	// 测试已设置值不被覆盖
	r2 := &ForwardRule{Type: ForwardTypeRelay}
	r2.SetDefaults()

	if r2.Type != ForwardTypeRelay {
		t.Errorf("expected Type 'relay', got '%s'", r2.Type)
	}
}

func TestProxyGroupSetDefaults(t *testing.T) {
	g := &ProxyGroup{}
	g.SetDefaults()

	if g.LoadBalanceMethod != LoadBalanceRoundRobin {
		t.Errorf("expected LoadBalanceMethod 'round_robin', got '%s'", g.LoadBalanceMethod)
	}
	if g.HealthCheckInterval != 30 {
		t.Errorf("expected HealthCheckInterval 30, got %d", g.HealthCheckInterval)
	}
	if g.HealthCheckTimeout != 5 {
		t.Errorf("expected HealthCheckTimeout 5, got %d", g.HealthCheckTimeout)
	}
	if g.HealthCheckRetries != 3 {
		t.Errorf("expected HealthCheckRetries 3, got %d", g.HealthCheckRetries)
	}
}

func TestProxyGroupNodeSetDefaults(t *testing.T) {
	n := &ProxyGroupNode{}
	n.SetDefaults()

	if n.Priority != 100 {
		t.Errorf("expected Priority 100, got %d", n.Priority)
	}
	if n.Weight != 100 {
		t.Errorf("expected Weight 100, got %d", n.Weight)
	}
	if n.Status != NodeStatusUnknown {
		t.Errorf("expected Status 'unknown', got '%s'", n.Status)
	}
}

func TestForwardRuleWithStatus(t *testing.T) {
	rule := ForwardRule{
		ID:        "rule-id",
		Name:      "Test Rule",
		Type:      ForwardTypeDirect,
		Status:    RuleStatusRunning,
		LastError: "",
	}

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded ForwardRule
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Status != RuleStatusRunning {
		t.Errorf("expected Status 'running', got '%s'", decoded.Status)
	}
}

func TestForwardRuleWithError(t *testing.T) {
	rule := ForwardRule{
		ID:        "rule-id",
		Name:      "Test Rule",
		Type:      ForwardTypeDirect,
		Status:    RuleStatusError,
		LastError: "address already in use",
	}

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded ForwardRule
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Status != RuleStatusError {
		t.Errorf("expected Status 'error', got '%s'", decoded.Status)
	}
	if decoded.LastError != "address already in use" {
		t.Errorf("expected LastError 'address already in use', got '%s'", decoded.LastError)
	}
}

func TestStringSliceScan(t *testing.T) {
	var ss StringSlice

	// Test nil value
	err := ss.Scan(nil)
	if err != nil {
		t.Errorf("Scan(nil) error: %v", err)
	}
	if ss != nil {
		t.Error("Scan(nil) should result in nil slice")
	}

	// Test valid JSON array
	jsonData := []byte(`["a", "b", "c"]`)
	err = ss.Scan(jsonData)
	if err != nil {
		t.Errorf("Scan() error: %v", err)
	}
	if len(ss) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(ss))
	}
	if ss[0] != "a" || ss[1] != "b" || ss[2] != "c" {
		t.Error("Values don't match")
	}

	// Test invalid type
	err = ss.Scan(123)
	if err == nil {
		t.Error("Scan(int) should return error")
	}
}

func TestStringSliceValue(t *testing.T) {
	// Test nil slice - returns "[]" for database compatibility
	var nilSlice StringSlice
	val, err := nilSlice.Value()
	if err != nil {
		t.Errorf("Value() error: %v", err)
	}
	if val != "[]" {
		t.Errorf("nil slice should return '[]', got %v", val)
	}

	// Test non-nil slice
	ss := StringSlice{"a", "b", "c"}
	val, err = ss.Value()
	if err != nil {
		t.Errorf("Value() error: %v", err)
	}

	// Verify JSON
	var decoded []string
	err = json.Unmarshal(val.([]byte), &decoded)
	if err != nil {
		t.Errorf("Failed to unmarshal value: %v", err)
	}
	if len(decoded) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(decoded))
	}
}

func TestStringSliceGormDataType(t *testing.T) {
	var ss StringSlice
	dataType := ss.GormDataType()
	if dataType != "text" {
		t.Errorf("GormDataType() = %s, want text", dataType)
	}
}

func TestClientTableName(t *testing.T) {
	c := Client{}
	if c.TableName() != "mb_clients" {
		t.Errorf("TableName() = %s, want mb_clients", c.TableName())
	}
}

func TestForwardRuleTableName(t *testing.T) {
	r := ForwardRule{}
	if r.TableName() != "forward_rules" {
		t.Errorf("TableName() = %s, want forward_rules", r.TableName())
	}
}

func TestProxyGroupTableName(t *testing.T) {
	g := ProxyGroup{}
	if g.TableName() != "proxy_groups" {
		t.Errorf("TableName() = %s, want proxy_groups", g.TableName())
	}
}

func TestProxyGroupNodeTableName(t *testing.T) {
	n := ProxyGroupNode{}
	if n.TableName() != "proxy_group_nodes" {
		t.Errorf("TableName() = %s, want proxy_group_nodes", n.TableName())
	}
}

func TestClientJSONSerialization(t *testing.T) {
	client := Client{
		ID:     "test-id",
		Name:   "Test Client",
		Token:  "secret-token",
		Status: ClientStatusOnline,
	}

	data, err := json.Marshal(client)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Client
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ID != client.ID {
		t.Error("ID mismatch")
	}
	if decoded.Name != client.Name {
		t.Error("Name mismatch")
	}
	if decoded.Status != client.Status {
		t.Error("Status mismatch")
	}
}

func TestForwardRuleJSONSerialization(t *testing.T) {
	rule := ForwardRule{
		ID:           "rule-id",
		Name:         "Test Rule",
		Type:         ForwardTypeRelay,
		ListenAddr:   "0.0.0.0:8080",
		ListenClient: "client-id",
		RelayChain:   StringSlice{"client-a", "client-b"},
		ExitAddr:     "192.168.1.1:80",
	}

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded ForwardRule
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ID != rule.ID {
		t.Error("ID mismatch")
	}
	if decoded.Type != rule.Type {
		t.Error("Type mismatch")
	}
	if len(decoded.RelayChain) != 2 {
		t.Errorf("RelayChain length mismatch: got %d, want 2", len(decoded.RelayChain))
	}
}

func TestProxyGroupJSONSerialization(t *testing.T) {
	group := ProxyGroup{
		ID:                "group-id",
		Name:              "Test Group",
		LoadBalanceMethod: LoadBalanceRoundRobin,
		HealthCheckEnabled: true,
	}

	data, err := json.Marshal(group)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded ProxyGroup
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ID != group.ID {
		t.Error("ID mismatch")
	}
	if decoded.LoadBalanceMethod != group.LoadBalanceMethod {
		t.Error("LoadBalanceMethod mismatch")
	}
}
