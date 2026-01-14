package proxy

import (
	"errors"
	"hash/fnv"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/dollarkillerx/MessageBoy/internal/storage"
	"github.com/dollarkillerx/MessageBoy/pkg/model"
)

var (
	ErrNoHealthyNodes = errors.New("no healthy nodes available")
	ErrGroupNotFound  = errors.New("proxy group not found")
)

// LoadBalancer 负载均衡器
type LoadBalancer struct {
	storage *storage.Storage

	// 轮询计数器 (按组ID)
	rrCounters map[string]*uint64
	mu         sync.RWMutex
}

func NewLoadBalancer(s *storage.Storage) *LoadBalancer {
	return &LoadBalancer{
		storage:    s,
		rrCounters: make(map[string]*uint64),
	}
}

// SelectNode 根据负载均衡策略选择节点
func (lb *LoadBalancer) SelectNode(groupID string, clientIP string) (*model.ProxyGroupNode, error) {
	group, err := lb.storage.ProxyGroup.GetByID(groupID)
	if err != nil {
		return nil, ErrGroupNotFound
	}

	nodes, err := lb.storage.ProxyGroup.GetHealthyNodesByGroupID(groupID)
	if err != nil || len(nodes) == 0 {
		return nil, ErrNoHealthyNodes
	}

	switch group.LoadBalanceMethod {
	case model.LoadBalanceRoundRobin:
		return lb.selectRoundRobin(groupID, nodes), nil
	case model.LoadBalanceRandom:
		return lb.selectRandom(nodes), nil
	case model.LoadBalanceLeastConn:
		return lb.selectLeastConn(nodes), nil
	case model.LoadBalanceIPHash:
		return lb.selectIPHash(nodes, clientIP), nil
	default:
		return lb.selectRoundRobin(groupID, nodes), nil
	}
}

// SelectNodeByGroupName 通过组名选择节点
func (lb *LoadBalancer) SelectNodeByGroupName(groupName string, clientIP string) (*model.ProxyGroupNode, error) {
	group, err := lb.storage.ProxyGroup.GetByName(groupName)
	if err != nil {
		return nil, ErrGroupNotFound
	}
	return lb.SelectNode(group.ID, clientIP)
}

// selectRoundRobin 轮询选择
func (lb *LoadBalancer) selectRoundRobin(groupID string, nodes []model.ProxyGroupNode) *model.ProxyGroupNode {
	lb.mu.Lock()
	counter, ok := lb.rrCounters[groupID]
	if !ok {
		var c uint64
		counter = &c
		lb.rrCounters[groupID] = counter
	}
	lb.mu.Unlock()

	idx := atomic.AddUint64(counter, 1) % uint64(len(nodes))
	return &nodes[idx]
}

// selectRandom 随机选择
func (lb *LoadBalancer) selectRandom(nodes []model.ProxyGroupNode) *model.ProxyGroupNode {
	idx := rand.Intn(len(nodes))
	return &nodes[idx]
}

// selectLeastConn 最少连接选择
func (lb *LoadBalancer) selectLeastConn(nodes []model.ProxyGroupNode) *model.ProxyGroupNode {
	// nodes 已经按 active_conns ASC 排序
	return &nodes[0]
}

// selectIPHash IP 哈希选择
func (lb *LoadBalancer) selectIPHash(nodes []model.ProxyGroupNode, clientIP string) *model.ProxyGroupNode {
	h := fnv.New32a()
	h.Write([]byte(clientIP))
	idx := h.Sum32() % uint32(len(nodes))
	return &nodes[idx]
}

// IncrementConnections 增加节点连接数
func (lb *LoadBalancer) IncrementConnections(nodeID string) error {
	return lb.storage.ProxyGroup.IncrementNodeConns(nodeID)
}

// DecrementConnections 减少节点连接数
func (lb *LoadBalancer) DecrementConnections(nodeID string) error {
	return lb.storage.ProxyGroup.DecrementNodeConns(nodeID)
}

// GetGroupIDByName 通过组名获取组ID
func (lb *LoadBalancer) GetGroupIDByName(name string) (string, error) {
	group, err := lb.storage.ProxyGroup.GetByName(name)
	if err != nil {
		return "", err
	}
	return group.ID, nil
}

// ResolveTarget 解析目标，支持 @group_name 格式
// 返回: clientID, nodeID, error
func (lb *LoadBalancer) ResolveTarget(target string, clientIP string) (string, string, error) {
	if !storage.IsGroupReference(target) {
		// 不是组引用，直接返回 (视为 client ID)
		return target, "", nil
	}

	groupRef := storage.ParseGroupReference(target)

	// 尝试通过 ID 获取
	node, err := lb.SelectNode(groupRef, clientIP)
	if err == ErrGroupNotFound {
		// 尝试通过名称获取
		node, err = lb.SelectNodeByGroupName(groupRef, clientIP)
	}

	if err != nil {
		return "", "", err
	}

	return node.ClientID, node.ID, nil
}
