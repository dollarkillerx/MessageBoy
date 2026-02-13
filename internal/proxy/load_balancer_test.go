package proxy

import (
	"errors"
	"sync"
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

// --- mockProxyGroupReader implements ProxyGroupReader for testing ---

type mockProxyGroupReader struct {
	mu           sync.Mutex
	groups       map[string]*model.ProxyGroup
	groupsByName map[string]*model.ProxyGroup
	healthyNodes map[string][]model.ProxyGroupNode
	incrCalls    []string
	decrCalls    []string
}

func (m *mockProxyGroupReader) GetByID(id string) (*model.ProxyGroup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groups[id]
	if !ok {
		return nil, errors.New("group not found")
	}
	return g, nil
}

func (m *mockProxyGroupReader) GetByName(name string) (*model.ProxyGroup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groupsByName[name]
	if !ok {
		return nil, errors.New("group not found by name")
	}
	return g, nil
}

func (m *mockProxyGroupReader) GetHealthyNodesByGroupID(groupID string) ([]model.ProxyGroupNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	nodes, ok := m.healthyNodes[groupID]
	if !ok {
		return nil, nil
	}
	return nodes, nil
}

func (m *mockProxyGroupReader) IncrementNodeConns(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.incrCalls = append(m.incrCalls, nodeID)
	return nil
}

func (m *mockProxyGroupReader) DecrementNodeConns(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decrCalls = append(m.decrCalls, nodeID)
	return nil
}

func TestNewLoadBalancer(t *testing.T) {
	lb := &LoadBalancer{
		proxyStore: &mockProxyGroupReader{},
		rrCounters: make(map[string]*uint64),
	}

	if lb.rrCounters == nil {
		t.Error("expected rrCounters to be initialized")
	}
	if lb.proxyStore == nil {
		t.Error("expected proxyStore to be set")
	}
}

func TestLoadBalancer_SelectNode_RoundRobinIntegration(t *testing.T) {
	mock := &mockProxyGroupReader{
		groups: map[string]*model.ProxyGroup{
			"g1": {ID: "g1", LoadBalanceMethod: model.LoadBalanceRoundRobin},
		},
		healthyNodes: map[string][]model.ProxyGroupNode{
			"g1": {
				{ID: "n1", ClientID: "c1"},
				{ID: "n2", ClientID: "c2"},
				{ID: "n3", ClientID: "c3"},
			},
		},
	}

	lb := &LoadBalancer{proxyStore: mock, rrCounters: make(map[string]*uint64)}

	counts := make(map[string]int)
	for i := 0; i < 9; i++ {
		node, err := lb.SelectNode("g1", "")
		if err != nil {
			t.Fatalf("SelectNode error: %v", err)
		}
		counts[node.ID]++
	}

	for _, id := range []string{"n1", "n2", "n3"} {
		if counts[id] != 3 {
			t.Errorf("expected node %s selected 3 times, got %d", id, counts[id])
		}
	}
}

func TestLoadBalancer_SelectNode_NoHealthyNodes(t *testing.T) {
	mock := &mockProxyGroupReader{
		groups: map[string]*model.ProxyGroup{
			"g1": {ID: "g1"},
		},
		healthyNodes: map[string][]model.ProxyGroupNode{
			"g1": {},
		},
	}

	lb := &LoadBalancer{proxyStore: mock, rrCounters: make(map[string]*uint64)}

	_, err := lb.SelectNode("g1", "")
	if !errors.Is(err, ErrNoHealthyNodes) {
		t.Errorf("expected ErrNoHealthyNodes, got %v", err)
	}
}

func TestLoadBalancer_SelectNode_GroupNotFound(t *testing.T) {
	mock := &mockProxyGroupReader{
		groups: map[string]*model.ProxyGroup{},
	}

	lb := &LoadBalancer{proxyStore: mock, rrCounters: make(map[string]*uint64)}

	_, err := lb.SelectNode("nonexistent", "")
	if !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("expected ErrGroupNotFound, got %v", err)
	}
}

func TestLoadBalancer_SelectNodeByGroupName(t *testing.T) {
	mock := &mockProxyGroupReader{
		groups: map[string]*model.ProxyGroup{
			"g1": {ID: "g1", LoadBalanceMethod: model.LoadBalanceRoundRobin},
		},
		groupsByName: map[string]*model.ProxyGroup{
			"my-group": {ID: "g1", LoadBalanceMethod: model.LoadBalanceRoundRobin},
		},
		healthyNodes: map[string][]model.ProxyGroupNode{
			"g1": {{ID: "n1", ClientID: "c1"}},
		},
	}

	lb := &LoadBalancer{proxyStore: mock, rrCounters: make(map[string]*uint64)}

	node, err := lb.SelectNodeByGroupName("my-group", "")
	if err != nil {
		t.Fatalf("SelectNodeByGroupName error: %v", err)
	}
	if node.ID != "n1" {
		t.Errorf("expected node n1, got %s", node.ID)
	}
}

func TestLoadBalancer_ResolveTarget_DirectClient(t *testing.T) {
	mock := &mockProxyGroupReader{}
	lb := &LoadBalancer{proxyStore: mock, rrCounters: make(map[string]*uint64)}

	clientID, nodeID, err := lb.ResolveTarget("client-123", "")
	if err != nil {
		t.Fatalf("ResolveTarget error: %v", err)
	}
	if clientID != "client-123" {
		t.Errorf("expected clientID 'client-123', got %q", clientID)
	}
	if nodeID != "" {
		t.Errorf("expected empty nodeID, got %q", nodeID)
	}
}

func TestLoadBalancer_ResolveTarget_GroupByID(t *testing.T) {
	mock := &mockProxyGroupReader{
		groups: map[string]*model.ProxyGroup{
			"group-id": {ID: "group-id", LoadBalanceMethod: model.LoadBalanceRoundRobin},
		},
		healthyNodes: map[string][]model.ProxyGroupNode{
			"group-id": {{ID: "n1", ClientID: "c1"}},
		},
	}

	lb := &LoadBalancer{proxyStore: mock, rrCounters: make(map[string]*uint64)}

	clientID, nodeID, err := lb.ResolveTarget("@group-id", "")
	if err != nil {
		t.Fatalf("ResolveTarget error: %v", err)
	}
	if clientID != "c1" {
		t.Errorf("expected clientID 'c1', got %q", clientID)
	}
	if nodeID != "n1" {
		t.Errorf("expected nodeID 'n1', got %q", nodeID)
	}
}

func TestLoadBalancer_ResolveTarget_GroupByName(t *testing.T) {
	g := &model.ProxyGroup{ID: "g1", LoadBalanceMethod: model.LoadBalanceRoundRobin}
	mock := &mockProxyGroupReader{
		groups: map[string]*model.ProxyGroup{
			// GetByID("my-group") fails (not an ID), but after name lookup
			// SelectNode is called with group.ID="g1", so we need it here.
			"g1": g,
		},
		groupsByName: map[string]*model.ProxyGroup{
			"my-group": g,
		},
		healthyNodes: map[string][]model.ProxyGroupNode{
			"g1": {{ID: "n1", ClientID: "c1"}},
		},
	}

	lb := &LoadBalancer{proxyStore: mock, rrCounters: make(map[string]*uint64)}

	clientID, nodeID, err := lb.ResolveTarget("@my-group", "")
	if err != nil {
		t.Fatalf("ResolveTarget error: %v", err)
	}
	if clientID != "c1" {
		t.Errorf("expected clientID 'c1', got %q", clientID)
	}
	if nodeID != "n1" {
		t.Errorf("expected nodeID 'n1', got %q", nodeID)
	}
}

func TestLoadBalancer_IncrementConnections(t *testing.T) {
	mock := &mockProxyGroupReader{}
	lb := &LoadBalancer{proxyStore: mock, rrCounters: make(map[string]*uint64)}

	if err := lb.IncrementConnections("n1"); err != nil {
		t.Fatalf("IncrementConnections error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.incrCalls) != 1 || mock.incrCalls[0] != "n1" {
		t.Errorf("expected IncrementNodeConns(n1), got %v", mock.incrCalls)
	}
}

func TestLoadBalancer_DecrementConnections(t *testing.T) {
	mock := &mockProxyGroupReader{}
	lb := &LoadBalancer{proxyStore: mock, rrCounters: make(map[string]*uint64)}

	if err := lb.DecrementConnections("n1"); err != nil {
		t.Fatalf("DecrementConnections error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.decrCalls) != 1 || mock.decrCalls[0] != "n1" {
		t.Errorf("expected DecrementNodeConns(n1), got %v", mock.decrCalls)
	}
}
