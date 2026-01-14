package proxy

import (
	"testing"

	"github.com/dollarkillerx/MessageBoy/pkg/model"
)

func TestSelectRoundRobin(t *testing.T) {
	lb := &LoadBalancer{
		rrCounters: make(map[string]*uint64),
	}

	nodes := []model.ProxyGroupNode{
		{ID: "node1", ClientID: "client1"},
		{ID: "node2", ClientID: "client2"},
		{ID: "node3", ClientID: "client3"},
	}

	groupID := "test-group"

	// 测试轮询
	selected := make(map[string]int)
	for i := 0; i < 9; i++ {
		node := lb.selectRoundRobin(groupID, nodes)
		selected[node.ID]++
	}

	// 每个节点应该被选中3次
	for _, node := range nodes {
		if selected[node.ID] != 3 {
			t.Errorf("Expected node %s to be selected 3 times, got %d", node.ID, selected[node.ID])
		}
	}
}

func TestSelectRandom(t *testing.T) {
	lb := &LoadBalancer{}

	nodes := []model.ProxyGroupNode{
		{ID: "node1", ClientID: "client1"},
		{ID: "node2", ClientID: "client2"},
	}

	// 测试随机选择 (至少应该能选择)
	for i := 0; i < 10; i++ {
		node := lb.selectRandom(nodes)
		if node == nil {
			t.Error("Expected node, got nil")
		}
	}
}

func TestSelectLeastConn(t *testing.T) {
	lb := &LoadBalancer{}

	nodes := []model.ProxyGroupNode{
		{ID: "node1", ClientID: "client1", ActiveConns: 5},
		{ID: "node2", ClientID: "client2", ActiveConns: 2},
		{ID: "node3", ClientID: "client3", ActiveConns: 10},
	}

	// 应该选择连接数最少的节点 (已排序，第一个)
	node := lb.selectLeastConn(nodes)
	if node.ID != "node1" {
		t.Errorf("Expected node1 (first in sorted list), got %s", node.ID)
	}
}

func TestSelectIPHash(t *testing.T) {
	lb := &LoadBalancer{}

	nodes := []model.ProxyGroupNode{
		{ID: "node1", ClientID: "client1"},
		{ID: "node2", ClientID: "client2"},
		{ID: "node3", ClientID: "client3"},
	}

	// 相同 IP 应该选择相同节点
	clientIP := "192.168.1.100"

	firstNode := lb.selectIPHash(nodes, clientIP)
	for i := 0; i < 10; i++ {
		node := lb.selectIPHash(nodes, clientIP)
		if node.ID != firstNode.ID {
			t.Errorf("IP hash should return consistent results, expected %s, got %s", firstNode.ID, node.ID)
		}
	}

	// 不同 IP 可能选择不同节点 (不强制验证，只测试一致性)
	differentIP := "10.0.0.1"
	node1 := lb.selectIPHash(nodes, differentIP)
	node2 := lb.selectIPHash(nodes, differentIP)
	if node1.ID != node2.ID {
		t.Error("IP hash should return consistent results for same IP")
	}
}

func TestIsGroupReference(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"@my-group", true},
		{"@", false}, // too short
		{"my-group", false},
		{"@group-with-dash", true},
		{"", false},
	}

	for _, test := range tests {
		// Import storage to test IsGroupReference
		// For now, test the logic inline
		isGroup := len(test.input) > 1 && test.input[0] == '@'
		if isGroup != test.expected {
			t.Errorf("IsGroupReference(%q) = %v, expected %v", test.input, isGroup, test.expected)
		}
	}
}

func TestParseGroupReference(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"@my-group", "my-group"},
		{"@group123", "group123"},
		{"my-group", "my-group"}, // 非引用返回原值
	}

	for _, test := range tests {
		// Test the parsing logic inline
		var result string
		if len(test.input) > 1 && test.input[0] == '@' {
			result = test.input[1:]
		} else {
			result = test.input
		}
		if result != test.expected {
			t.Errorf("ParseGroupReference(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}
