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
	// Test nil slice
	var nilSlice StringSlice
	val, err := nilSlice.Value()
	if err != nil {
		t.Errorf("Value() error: %v", err)
	}
	if val != nil {
		t.Error("nil slice should return nil value")
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
	if dataType != "jsonb" {
		t.Errorf("GormDataType() = %s, want jsonb", dataType)
	}
}

func TestClientTableName(t *testing.T) {
	c := Client{}
	if c.TableName() != "clients" {
		t.Errorf("TableName() = %s, want clients", c.TableName())
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
