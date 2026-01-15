package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/dollarkillerx/MessageBoy/internal/storage"
	"github.com/dollarkillerx/MessageBoy/pkg/model"
)

// ==================== ProxyGroup CRUD ====================

// CreateProxyGroupMethod - 创建代理组
type CreateProxyGroupMethod struct {
	storage *storage.Storage
}

func NewCreateProxyGroupMethod(s *storage.Storage) *CreateProxyGroupMethod {
	return &CreateProxyGroupMethod{storage: s}
}

func (m *CreateProxyGroupMethod) Name() string { return "createProxyGroup" }

type CreateProxyGroupParams struct {
	Name                string `json:"name"`
	Description         string `json:"description"`
	LoadBalanceMethod   string `json:"load_balance_method"`
	HealthCheckEnabled  *bool  `json:"health_check_enabled"`
	HealthCheckInterval *int   `json:"health_check_interval"`
	HealthCheckTimeout  *int   `json:"health_check_timeout"`
	HealthCheckRetries  *int   `json:"health_check_retries"`
}

func (m *CreateProxyGroupMethod) Execute(ctx context.Context, params json.RawMessage) (any, error) {
	var p CreateProxyGroupParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.Name == "" {
		return nil, errors.New("name is required")
	}

	// 检查名称是否已存在
	_, err := m.storage.ProxyGroup.GetByName(p.Name)
	if err == nil {
		return nil, errors.New("group name already exists")
	}

	group := &model.ProxyGroup{
		ID:                uuid.New().String(),
		Name:              p.Name,
		Description:       p.Description,
		LoadBalanceMethod: model.LoadBalanceRoundRobin,
	}

	if p.LoadBalanceMethod != "" {
		group.LoadBalanceMethod = model.LoadBalanceMethod(p.LoadBalanceMethod)
	}
	if p.HealthCheckEnabled != nil {
		group.HealthCheckEnabled = *p.HealthCheckEnabled
	} else {
		group.HealthCheckEnabled = true
	}
	if p.HealthCheckInterval != nil {
		group.HealthCheckInterval = *p.HealthCheckInterval
	} else {
		group.HealthCheckInterval = 30
	}
	if p.HealthCheckTimeout != nil {
		group.HealthCheckTimeout = *p.HealthCheckTimeout
	} else {
		group.HealthCheckTimeout = 5
	}
	if p.HealthCheckRetries != nil {
		group.HealthCheckRetries = *p.HealthCheckRetries
	} else {
		group.HealthCheckRetries = 3
	}

	if err := m.storage.ProxyGroup.Create(group); err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	return map[string]any{
		"id":   group.ID,
		"name": group.Name,
	}, nil
}

func (m *CreateProxyGroupMethod) RequireAuth() bool { return true }

// GetProxyGroupListMethod - 获取代理组列表
type GetProxyGroupListMethod struct {
	storage *storage.Storage
}

func NewGetProxyGroupListMethod(s *storage.Storage) *GetProxyGroupListMethod {
	return &GetProxyGroupListMethod{storage: s}
}

func (m *GetProxyGroupListMethod) Name() string { return "getProxyGroupList" }

type GetProxyGroupListParams struct {
	Page   int    `json:"page"`
	Limit  int    `json:"limit"`
	Search string `json:"search"`
}

func (m *GetProxyGroupListMethod) Execute(ctx context.Context, params json.RawMessage) (any, error) {
	var p GetProxyGroupListParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.Page <= 0 {
		p.Page = 1
	}
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 20
	}

	groups, total, err := m.storage.ProxyGroup.List(storage.ProxyGroupListParams{
		Page:   p.Page,
		Limit:  p.Limit,
		Search: p.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get group list: %w", err)
	}

	// 获取每个组的节点
	groupList := make([]map[string]any, len(groups))
	for i, g := range groups {
		nodes, _ := m.storage.ProxyGroup.GetNodesByGroupID(g.ID)
		healthyCount := 0
		nodeList := make([]map[string]any, len(nodes))
		for j, n := range nodes {
			if n.Status == model.NodeStatusHealthy {
				healthyCount++
			}
			nodeList[j] = map[string]any{
				"id":            n.ID,
				"group_id":      n.GroupID,
				"client_id":     n.ClientID,
				"priority":      n.Priority,
				"weight":        n.Weight,
				"status":        n.Status,
				"active_conns":  n.ActiveConns,
				"last_check_at": n.LastCheckAt,
				"created_at":    n.CreatedAt,
			}
		}

		groupList[i] = map[string]any{
			"id":                   g.ID,
			"name":                 g.Name,
			"description":          g.Description,
			"load_balance_method":  g.LoadBalanceMethod,
			"health_check_enabled": g.HealthCheckEnabled,
			"health_check_interval": g.HealthCheckInterval,
			"node_count":           len(nodes),
			"healthy_node_count":   healthyCount,
			"nodes":                nodeList,
			"created_at":           g.CreatedAt,
		}
	}

	pages := (total + int64(p.Limit) - 1) / int64(p.Limit)

	return map[string]any{
		"groups": groupList,
		"total":  total,
		"page":   p.Page,
		"limit":  p.Limit,
		"pages":  pages,
	}, nil
}

func (m *GetProxyGroupListMethod) RequireAuth() bool { return true }

// GetProxyGroupMethod - 获取单个代理组详情
type GetProxyGroupMethod struct {
	storage *storage.Storage
}

func NewGetProxyGroupMethod(s *storage.Storage) *GetProxyGroupMethod {
	return &GetProxyGroupMethod{storage: s}
}

func (m *GetProxyGroupMethod) Name() string { return "getProxyGroup" }

type GetProxyGroupParams struct {
	ID string `json:"id"`
}

func (m *GetProxyGroupMethod) Execute(ctx context.Context, params json.RawMessage) (any, error) {
	var p GetProxyGroupParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	group, nodes, err := m.storage.ProxyGroup.GetGroupWithNodes(p.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("group not found")
		}
		return nil, fmt.Errorf("failed to get group: %w", err)
	}

	nodeList := make([]map[string]any, len(nodes))
	for i, n := range nodes {
		node := map[string]any{
			"id":            n.ID,
			"client_id":     n.ClientID,
			"priority":      n.Priority,
			"weight":        n.Weight,
			"status":        n.Status,
			"active_conns":  n.ActiveConns,
			"last_check_at": n.LastCheckAt,
			"last_check_ok": n.LastCheckOK,
			"fail_count":    n.FailCount,
		}
		if n.Client != nil {
			node["client_name"] = n.Client.Name
			node["client_status"] = n.Client.Status
		}
		nodeList[i] = node
	}

	return map[string]any{
		"id":                    group.ID,
		"name":                  group.Name,
		"description":           group.Description,
		"load_balance_method":   group.LoadBalanceMethod,
		"health_check_enabled":  group.HealthCheckEnabled,
		"health_check_interval": group.HealthCheckInterval,
		"health_check_timeout":  group.HealthCheckTimeout,
		"health_check_retries":  group.HealthCheckRetries,
		"nodes":                 nodeList,
		"created_at":            group.CreatedAt,
		"updated_at":            group.UpdatedAt,
	}, nil
}

func (m *GetProxyGroupMethod) RequireAuth() bool { return true }

// UpdateProxyGroupMethod - 更新代理组
type UpdateProxyGroupMethod struct {
	storage *storage.Storage
}

func NewUpdateProxyGroupMethod(s *storage.Storage) *UpdateProxyGroupMethod {
	return &UpdateProxyGroupMethod{storage: s}
}

func (m *UpdateProxyGroupMethod) Name() string { return "updateProxyGroup" }

type UpdateProxyGroupParams struct {
	ID                  string  `json:"id"`
	Name                *string `json:"name"`
	Description         *string `json:"description"`
	LoadBalanceMethod   *string `json:"load_balance_method"`
	HealthCheckEnabled  *bool   `json:"health_check_enabled"`
	HealthCheckInterval *int    `json:"health_check_interval"`
	HealthCheckTimeout  *int    `json:"health_check_timeout"`
	HealthCheckRetries  *int    `json:"health_check_retries"`
}

func (m *UpdateProxyGroupMethod) Execute(ctx context.Context, params json.RawMessage) (any, error) {
	var p UpdateProxyGroupParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	group, err := m.storage.ProxyGroup.GetByID(p.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("group not found")
		}
		return nil, fmt.Errorf("failed to get group: %w", err)
	}

	if p.Name != nil {
		group.Name = *p.Name
	}
	if p.Description != nil {
		group.Description = *p.Description
	}
	if p.LoadBalanceMethod != nil {
		group.LoadBalanceMethod = model.LoadBalanceMethod(*p.LoadBalanceMethod)
	}
	if p.HealthCheckEnabled != nil {
		group.HealthCheckEnabled = *p.HealthCheckEnabled
	}
	if p.HealthCheckInterval != nil {
		group.HealthCheckInterval = *p.HealthCheckInterval
	}
	if p.HealthCheckTimeout != nil {
		group.HealthCheckTimeout = *p.HealthCheckTimeout
	}
	if p.HealthCheckRetries != nil {
		group.HealthCheckRetries = *p.HealthCheckRetries
	}

	if err := m.storage.ProxyGroup.Update(group); err != nil {
		return nil, fmt.Errorf("failed to update group: %w", err)
	}

	return map[string]any{"success": true}, nil
}

func (m *UpdateProxyGroupMethod) RequireAuth() bool { return true }

// DeleteProxyGroupMethod - 删除代理组
type DeleteProxyGroupMethod struct {
	storage *storage.Storage
}

func NewDeleteProxyGroupMethod(s *storage.Storage) *DeleteProxyGroupMethod {
	return &DeleteProxyGroupMethod{storage: s}
}

func (m *DeleteProxyGroupMethod) Name() string { return "deleteProxyGroup" }

type DeleteProxyGroupParams struct {
	ID string `json:"id"`
}

func (m *DeleteProxyGroupMethod) Execute(ctx context.Context, params json.RawMessage) (any, error) {
	var p DeleteProxyGroupParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	if err := m.storage.ProxyGroup.Delete(p.ID); err != nil {
		return nil, fmt.Errorf("failed to delete group: %w", err)
	}

	return map[string]any{"success": true}, nil
}

func (m *DeleteProxyGroupMethod) RequireAuth() bool { return true }

// ==================== ProxyGroupNode CRUD ====================

// AddProxyGroupNodeMethod - 添加节点到代理组
type AddProxyGroupNodeMethod struct {
	storage *storage.Storage
}

func NewAddProxyGroupNodeMethod(s *storage.Storage) *AddProxyGroupNodeMethod {
	return &AddProxyGroupNodeMethod{storage: s}
}

func (m *AddProxyGroupNodeMethod) Name() string { return "addProxyGroupNode" }

type AddProxyGroupNodeParams struct {
	GroupID  string `json:"group_id"`
	ClientID string `json:"client_id"`
	Priority *int   `json:"priority"`
	Weight   *int   `json:"weight"`
}

func (m *AddProxyGroupNodeMethod) Execute(ctx context.Context, params json.RawMessage) (any, error) {
	var p AddProxyGroupNodeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.GroupID == "" {
		return nil, errors.New("group_id is required")
	}
	if p.ClientID == "" {
		return nil, errors.New("client_id is required")
	}

	// 验证组存在
	_, err := m.storage.ProxyGroup.GetByID(p.GroupID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("group not found")
		}
		return nil, fmt.Errorf("failed to verify group: %w", err)
	}

	// 验证客户端存在
	client, err := m.storage.Client.GetByID(p.ClientID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("client not found")
		}
		return nil, fmt.Errorf("failed to verify client: %w", err)
	}

	// 检查客户端是否已在代理组中
	existingNodes, err := m.storage.ProxyGroup.GetNodesByGroupID(p.GroupID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing nodes: %w", err)
	}
	for _, n := range existingNodes {
		if n.ClientID == p.ClientID {
			return nil, errors.New("该客户端已在代理组中")
		}
	}

	node := &model.ProxyGroupNode{
		ID:       uuid.New().String(),
		GroupID:  p.GroupID,
		ClientID: p.ClientID,
		Priority: 100,
		Weight:   100,
		Status:   model.NodeStatusUnknown,
	}

	if p.Priority != nil {
		node.Priority = *p.Priority
	}
	if p.Weight != nil {
		node.Weight = *p.Weight
	}

	// 根据 client 在线状态设置初始健康状态
	if client.Status == model.ClientStatusOnline {
		node.Status = model.NodeStatusHealthy
	}

	if err := m.storage.ProxyGroup.AddNode(node); err != nil {
		return nil, fmt.Errorf("failed to add node: %w", err)
	}

	return map[string]any{
		"id":        node.ID,
		"group_id":  node.GroupID,
		"client_id": node.ClientID,
	}, nil
}

func (m *AddProxyGroupNodeMethod) RequireAuth() bool { return true }

// RemoveProxyGroupNodeMethod - 从代理组移除节点
type RemoveProxyGroupNodeMethod struct {
	storage *storage.Storage
}

func NewRemoveProxyGroupNodeMethod(s *storage.Storage) *RemoveProxyGroupNodeMethod {
	return &RemoveProxyGroupNodeMethod{storage: s}
}

func (m *RemoveProxyGroupNodeMethod) Name() string { return "removeProxyGroupNode" }

type RemoveProxyGroupNodeParams struct {
	ID       string `json:"id"`        // 节点 ID
	GroupID  string `json:"group_id"`  // 或者通过 group_id + client_id 删除
	ClientID string `json:"client_id"`
}

func (m *RemoveProxyGroupNodeMethod) Execute(ctx context.Context, params json.RawMessage) (any, error) {
	var p RemoveProxyGroupNodeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID != "" {
		if err := m.storage.ProxyGroup.RemoveNode(p.ID); err != nil {
			return nil, fmt.Errorf("failed to remove node: %w", err)
		}
	} else if p.GroupID != "" && p.ClientID != "" {
		if err := m.storage.ProxyGroup.RemoveNodeByClientID(p.GroupID, p.ClientID); err != nil {
			return nil, fmt.Errorf("failed to remove node: %w", err)
		}
	} else {
		return nil, errors.New("id or (group_id and client_id) is required")
	}

	return map[string]any{"success": true}, nil
}

func (m *RemoveProxyGroupNodeMethod) RequireAuth() bool { return true }

// UpdateProxyGroupNodeMethod - 更新节点配置
type UpdateProxyGroupNodeMethod struct {
	storage *storage.Storage
}

func NewUpdateProxyGroupNodeMethod(s *storage.Storage) *UpdateProxyGroupNodeMethod {
	return &UpdateProxyGroupNodeMethod{storage: s}
}

func (m *UpdateProxyGroupNodeMethod) Name() string { return "updateProxyGroupNode" }

type UpdateProxyGroupNodeParams struct {
	ID       string `json:"id"`
	Priority *int   `json:"priority"`
	Weight   *int   `json:"weight"`
}

func (m *UpdateProxyGroupNodeMethod) Execute(ctx context.Context, params json.RawMessage) (any, error) {
	var p UpdateProxyGroupNodeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	node, err := m.storage.ProxyGroup.GetNode(p.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("node not found")
		}
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	if p.Priority != nil {
		node.Priority = *p.Priority
	}
	if p.Weight != nil {
		node.Weight = *p.Weight
	}

	if err := m.storage.ProxyGroup.UpdateNode(node); err != nil {
		return nil, fmt.Errorf("failed to update node: %w", err)
	}

	return map[string]any{"success": true}, nil
}

func (m *UpdateProxyGroupNodeMethod) RequireAuth() bool { return true }
