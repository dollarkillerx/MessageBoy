package proxy

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/dollarkillerx/MessageBoy/internal/relay"
	"github.com/dollarkillerx/MessageBoy/internal/storage"
	"github.com/dollarkillerx/MessageBoy/pkg/model"
)

// ProxyGroupStore abstracts proxy group storage operations for testability.
type ProxyGroupStore interface {
	List(params storage.ProxyGroupListParams) ([]model.ProxyGroup, int64, error)
	GetNodesByGroupID(groupID string) ([]model.ProxyGroupNode, error)
	GetNode(id string) (*model.ProxyGroupNode, error)
	GetByID(id string) (*model.ProxyGroup, error)
	UpdateNodeHealth(nodeID string, healthy bool) error
	MarkNodeUnhealthy(nodeID string) error
}

// ClientChecker abstracts online-status checking for testability.
type ClientChecker interface {
	IsClientOnline(clientID string) bool
}

// HealthChecker 健康检查器
type HealthChecker struct {
	proxyStore  ProxyGroupStore
	clientCheck ClientChecker

	stopCh   chan struct{}
	wg       sync.WaitGroup
	interval time.Duration
}

func NewHealthChecker(s *storage.Storage, ws *relay.WSServer) *HealthChecker {
	return &HealthChecker{
		proxyStore:  s.ProxyGroup,
		clientCheck: ws,
		stopCh:      make(chan struct{}),
		interval:    10 * time.Second, // 默认检查间隔
	}
}

func (h *HealthChecker) Start() {
	h.wg.Add(1)
	go h.run()
	log.Info().Msg("Health checker started")
}

func (h *HealthChecker) Stop() {
	close(h.stopCh)
	h.wg.Wait()
	log.Info().Msg("Health checker stopped")
}

func (h *HealthChecker) run() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	// 启动时立即执行一次
	h.checkAllGroups()

	for {
		select {
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.checkAllGroups()
		}
	}
}

func (h *HealthChecker) checkAllGroups() {
	groups, _, err := h.proxyStore.List(storage.ProxyGroupListParams{
		Page:  1,
		Limit: 1000,
	})
	if err != nil {
		log.Warn().Err(err).Msg("Failed to list proxy groups for health check")
		return
	}

	for _, group := range groups {
		if !group.HealthCheckEnabled {
			continue
		}
		h.checkGroup(&group)
	}
}

func (h *HealthChecker) checkGroup(group *model.ProxyGroup) {
	nodes, err := h.proxyStore.GetNodesByGroupID(group.ID)
	if err != nil {
		log.Warn().Err(err).Str("group_id", group.ID).Msg("Failed to get nodes for health check")
		return
	}

	for _, node := range nodes {
		h.checkNode(group, &node)
	}
}

func (h *HealthChecker) checkNode(group *model.ProxyGroup, node *model.ProxyGroupNode) {
	// 检查 client 是否在线 (通过 WebSocket 连接状态)
	isOnline := h.clientCheck.IsClientOnline(node.ClientID)

	// 更新健康状态
	if err := h.proxyStore.UpdateNodeHealth(node.ID, isOnline); err != nil {
		log.Warn().Err(err).Str("node_id", node.ID).Msg("Failed to update node health")
		return
	}

	// 如果连续失败次数超过阈值，标记为不健康
	if !isOnline {
		newNode, _ := h.proxyStore.GetNode(node.ID)
		if newNode != nil && newNode.FailCount >= group.HealthCheckRetries {
			h.proxyStore.MarkNodeUnhealthy(node.ID)
			log.Warn().
				Str("node_id", node.ID).
				Str("client_id", node.ClientID).
				Int("fail_count", newNode.FailCount).
				Msg("Node marked as unhealthy")
		}
	} else {
		log.Debug().
			Str("node_id", node.ID).
			Str("client_id", node.ClientID).
			Msg("Node health check passed")
	}
}

// CheckNodeHealth 手动检查单个节点健康状态
func (h *HealthChecker) CheckNodeHealth(nodeID string) error {
	node, err := h.proxyStore.GetNode(nodeID)
	if err != nil {
		return err
	}

	group, err := h.proxyStore.GetByID(node.GroupID)
	if err != nil {
		return err
	}

	h.checkNode(group, node)
	return nil
}
