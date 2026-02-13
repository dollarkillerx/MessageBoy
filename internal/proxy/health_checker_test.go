package proxy

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dollarkillerx/MessageBoy/internal/storage"
	"github.com/dollarkillerx/MessageBoy/pkg/model"
)

// mockProxyGroupStore implements ProxyGroupStore for testing.
type mockProxyGroupStore struct {
	mu              sync.Mutex
	groups          []model.ProxyGroup
	nodes           map[string][]model.ProxyGroupNode // groupID -> nodes
	nodeByID        map[string]*model.ProxyGroupNode
	groupByID       map[string]*model.ProxyGroup
	healthUpdates   []struct{ nodeID string; healthy bool }
	markedUnhealthy []string
}

func (m *mockProxyGroupStore) List(params storage.ProxyGroupListParams) ([]model.ProxyGroup, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.groups, int64(len(m.groups)), nil
}

func (m *mockProxyGroupStore) GetNodesByGroupID(groupID string) ([]model.ProxyGroupNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	nodes, ok := m.nodes[groupID]
	if !ok {
		return nil, nil
	}
	return nodes, nil
}

func (m *mockProxyGroupStore) GetNode(id string) (*model.ProxyGroupNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, ok := m.nodeByID[id]
	if !ok {
		return nil, errors.New("node not found")
	}
	return node, nil
}

func (m *mockProxyGroupStore) GetByID(id string) (*model.ProxyGroup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	group, ok := m.groupByID[id]
	if !ok {
		return nil, errors.New("group not found")
	}
	return group, nil
}

func (m *mockProxyGroupStore) UpdateNodeHealth(nodeID string, healthy bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthUpdates = append(m.healthUpdates, struct{ nodeID string; healthy bool }{nodeID, healthy})
	return nil
}

func (m *mockProxyGroupStore) MarkNodeUnhealthy(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markedUnhealthy = append(m.markedUnhealthy, nodeID)
	return nil
}

// mockClientChecker implements ClientChecker for testing.
type mockClientChecker struct {
	online map[string]bool
}

func (m *mockClientChecker) IsClientOnline(clientID string) bool {
	return m.online[clientID]
}

func TestHealthChecker_New(t *testing.T) {
	store := &mockProxyGroupStore{}
	checker := &mockClientChecker{}

	hc := &HealthChecker{
		proxyStore:  store,
		clientCheck: checker,
		stopCh:      make(chan struct{}),
		interval:    10 * time.Second,
	}

	if hc.interval != 10*time.Second {
		t.Errorf("expected interval 10s, got %v", hc.interval)
	}
	if hc.proxyStore == nil {
		t.Error("expected proxyStore to be set")
	}
	if hc.clientCheck == nil {
		t.Error("expected clientCheck to be set")
	}
}

func TestHealthChecker_CheckNode_Online(t *testing.T) {
	store := &mockProxyGroupStore{
		nodeByID: map[string]*model.ProxyGroupNode{},
	}
	checker := &mockClientChecker{online: map[string]bool{"client-1": true}}

	hc := &HealthChecker{proxyStore: store, clientCheck: checker, stopCh: make(chan struct{})}

	group := &model.ProxyGroup{ID: "g1", HealthCheckRetries: 3}
	node := &model.ProxyGroupNode{ID: "n1", ClientID: "client-1"}

	hc.checkNode(group, node)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.healthUpdates) != 1 {
		t.Fatalf("expected 1 health update, got %d", len(store.healthUpdates))
	}
	if store.healthUpdates[0].nodeID != "n1" || !store.healthUpdates[0].healthy {
		t.Errorf("expected UpdateNodeHealth(n1, true), got (%s, %v)",
			store.healthUpdates[0].nodeID, store.healthUpdates[0].healthy)
	}
}

func TestHealthChecker_CheckNode_Offline(t *testing.T) {
	store := &mockProxyGroupStore{
		nodeByID: map[string]*model.ProxyGroupNode{
			"n1": {ID: "n1", ClientID: "client-1", FailCount: 0},
		},
	}
	checker := &mockClientChecker{online: map[string]bool{"client-1": false}}

	hc := &HealthChecker{proxyStore: store, clientCheck: checker, stopCh: make(chan struct{})}

	group := &model.ProxyGroup{ID: "g1", HealthCheckRetries: 3}
	node := &model.ProxyGroupNode{ID: "n1", ClientID: "client-1"}

	hc.checkNode(group, node)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.healthUpdates) != 1 {
		t.Fatalf("expected 1 health update, got %d", len(store.healthUpdates))
	}
	if store.healthUpdates[0].healthy {
		t.Error("expected UpdateNodeHealth(n1, false)")
	}
}

func TestHealthChecker_CheckNode_ExceedsRetries(t *testing.T) {
	store := &mockProxyGroupStore{
		nodeByID: map[string]*model.ProxyGroupNode{
			"n1": {ID: "n1", ClientID: "client-1", FailCount: 3},
		},
	}
	checker := &mockClientChecker{online: map[string]bool{"client-1": false}}

	hc := &HealthChecker{proxyStore: store, clientCheck: checker, stopCh: make(chan struct{})}

	group := &model.ProxyGroup{ID: "g1", HealthCheckRetries: 3}
	node := &model.ProxyGroupNode{ID: "n1", ClientID: "client-1"}

	hc.checkNode(group, node)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.markedUnhealthy) != 1 {
		t.Fatalf("expected 1 MarkNodeUnhealthy call, got %d", len(store.markedUnhealthy))
	}
	if store.markedUnhealthy[0] != "n1" {
		t.Errorf("expected MarkNodeUnhealthy(n1), got %s", store.markedUnhealthy[0])
	}
}

func TestHealthChecker_CheckGroup(t *testing.T) {
	store := &mockProxyGroupStore{
		nodes: map[string][]model.ProxyGroupNode{
			"g1": {
				{ID: "n1", ClientID: "c1"},
				{ID: "n2", ClientID: "c2"},
			},
		},
		nodeByID: map[string]*model.ProxyGroupNode{},
	}
	checker := &mockClientChecker{online: map[string]bool{"c1": true, "c2": true}}

	hc := &HealthChecker{proxyStore: store, clientCheck: checker, stopCh: make(chan struct{})}

	group := &model.ProxyGroup{ID: "g1", HealthCheckRetries: 3}
	hc.checkGroup(group)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.healthUpdates) != 2 {
		t.Errorf("expected 2 health updates, got %d", len(store.healthUpdates))
	}
}

func TestHealthChecker_CheckAllGroups(t *testing.T) {
	store := &mockProxyGroupStore{
		groups: []model.ProxyGroup{
			{ID: "g1", HealthCheckEnabled: true},
			{ID: "g2", HealthCheckEnabled: false},
		},
		nodes: map[string][]model.ProxyGroupNode{
			"g1": {{ID: "n1", ClientID: "c1"}},
			"g2": {{ID: "n2", ClientID: "c2"}},
		},
		nodeByID: map[string]*model.ProxyGroupNode{},
	}
	checker := &mockClientChecker{online: map[string]bool{"c1": true, "c2": true}}

	hc := &HealthChecker{proxyStore: store, clientCheck: checker, stopCh: make(chan struct{})}
	hc.checkAllGroups()

	store.mu.Lock()
	defer store.mu.Unlock()
	// Only g1 should be checked (g2 has HealthCheckEnabled=false)
	if len(store.healthUpdates) != 1 {
		t.Errorf("expected 1 health update (only enabled group), got %d", len(store.healthUpdates))
	}
}

func TestHealthChecker_CheckNodeHealth_NotFound(t *testing.T) {
	store := &mockProxyGroupStore{
		nodeByID: map[string]*model.ProxyGroupNode{},
	}
	checker := &mockClientChecker{}

	hc := &HealthChecker{proxyStore: store, clientCheck: checker, stopCh: make(chan struct{})}

	err := hc.CheckNodeHealth("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent node")
	}
}

func TestHealthChecker_StartStop(t *testing.T) {
	var checkCount int64
	store := &mockProxyGroupStore{
		groups:   []model.ProxyGroup{},
		nodeByID: map[string]*model.ProxyGroupNode{},
	}
	// Wrap List to count calls
	origStore := store
	wrapper := &countingProxyGroupStore{
		mockProxyGroupStore: origStore,
		listCount:           &checkCount,
	}

	checker := &mockClientChecker{}

	hc := &HealthChecker{
		proxyStore:  wrapper,
		clientCheck: checker,
		stopCh:      make(chan struct{}),
		interval:    50 * time.Millisecond,
	}

	hc.Start()
	time.Sleep(150 * time.Millisecond)
	hc.Stop()

	count := atomic.LoadInt64(&checkCount)
	if count < 2 {
		t.Errorf("expected checkAllGroups to be called at least 2 times, got %d", count)
	}
}

// countingProxyGroupStore wraps mockProxyGroupStore to count List calls.
type countingProxyGroupStore struct {
	*mockProxyGroupStore
	listCount *int64
}

func (c *countingProxyGroupStore) List(params storage.ProxyGroupListParams) ([]model.ProxyGroup, int64, error) {
	atomic.AddInt64(c.listCount, 1)
	return c.mockProxyGroupStore.List(params)
}
