package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/dollarkillerx/MessageBoy/internal/relay"
	"github.com/dollarkillerx/MessageBoy/internal/storage"
	"github.com/dollarkillerx/MessageBoy/pkg/model"
)

// CreateForwardRuleMethod - 创建转发规则
type CreateForwardRuleMethod struct {
	storage  *storage.Storage
	wsServer *relay.WSServer
}

func NewCreateForwardRuleMethod(s *storage.Storage, ws *relay.WSServer) *CreateForwardRuleMethod {
	return &CreateForwardRuleMethod{storage: s, wsServer: ws}
}

func (m *CreateForwardRuleMethod) Name() string { return "createForwardRule" }

type CreateForwardRuleParams struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	ListenAddr   string   `json:"listen_addr"`
	ListenClient string   `json:"listen_client"`
	TargetAddr   string   `json:"target_addr"`
	RelayChain   []string `json:"relay_chain"`
	ExitAddr     string   `json:"exit_addr"`
}

func (m *CreateForwardRuleMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p CreateForwardRuleParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.Name == "" {
		return nil, errors.New("name is required")
	}
	if p.Type == "" {
		return nil, errors.New("type is required")
	}
	if p.ListenAddr == "" {
		return nil, errors.New("listen_addr is required")
	}
	if p.ListenClient == "" {
		return nil, errors.New("listen_client is required")
	}

	// 验证 client 存在
	_, err := m.storage.Client.GetByID(p.ListenClient)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("listen_client not found")
		}
		return nil, fmt.Errorf("failed to verify client: %w", err)
	}

	rule := &model.ForwardRule{
		ID:           uuid.New().String(),
		Name:         p.Name,
		Type:         model.ForwardType(p.Type),
		Enabled:      true,
		ListenAddr:   p.ListenAddr,
		ListenClient: p.ListenClient,
	}

	if p.Type == "direct" {
		if p.TargetAddr == "" {
			return nil, errors.New("target_addr is required for direct type")
		}
		rule.TargetAddr = p.TargetAddr
	} else if p.Type == "relay" {
		if len(p.RelayChain) == 0 {
			return nil, errors.New("relay_chain is required for relay type")
		}
		if p.ExitAddr == "" {
			return nil, errors.New("exit_addr is required for relay type")
		}
		rule.RelayChain = p.RelayChain
		rule.ExitAddr = p.ExitAddr
	} else {
		return nil, errors.New("invalid type, must be 'direct' or 'relay'")
	}

	if err := m.storage.Forward.Create(rule); err != nil {
		return nil, fmt.Errorf("failed to create rule: %w", err)
	}

	// 通知相关 client 规则已更新
	if m.wsServer != nil {
		m.wsServer.NotifyRuleUpdate(p.ListenClient)
	}

	return map[string]interface{}{
		"id":   rule.ID,
		"name": rule.Name,
		"type": rule.Type,
	}, nil
}

func (m *CreateForwardRuleMethod) RequireAuth() bool { return true }

// GetForwardRuleListMethod - 获取转发规则列表
type GetForwardRuleListMethod struct {
	storage *storage.Storage
}

func NewGetForwardRuleListMethod(s *storage.Storage) *GetForwardRuleListMethod {
	return &GetForwardRuleListMethod{storage: s}
}

func (m *GetForwardRuleListMethod) Name() string { return "getForwardRuleList" }

type GetForwardRuleListParams struct {
	Page     int    `json:"page"`
	Limit    int    `json:"limit"`
	ClientID string `json:"client_id"`
	Type     string `json:"type"`
	Enabled  *bool  `json:"enabled"`
}

func (m *GetForwardRuleListMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p GetForwardRuleListParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.Page <= 0 {
		p.Page = 1
	}
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 20
	}

	rules, total, err := m.storage.Forward.List(storage.ForwardListParams{
		Page:     p.Page,
		Limit:    p.Limit,
		ClientID: p.ClientID,
		Type:     p.Type,
		Enabled:  p.Enabled,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get rule list: %w", err)
	}

	// 获取 client 名称映射
	clientNames := make(map[string]string)
	for _, r := range rules {
		if _, ok := clientNames[r.ListenClient]; !ok {
			if client, err := m.storage.Client.GetByID(r.ListenClient); err == nil {
				clientNames[r.ListenClient] = client.Name
			}
		}
	}

	pages := (total + int64(p.Limit) - 1) / int64(p.Limit)

	ruleList := make([]map[string]interface{}, len(rules))
	for i, r := range rules {
		rule := map[string]interface{}{
			"id":                 r.ID,
			"name":               r.Name,
			"type":               r.Type,
			"enabled":            r.Enabled,
			"listen_addr":        r.ListenAddr,
			"listen_client":      r.ListenClient,
			"listen_client_name": clientNames[r.ListenClient],
			"created_at":         r.CreatedAt,
		}
		if r.Type == model.ForwardTypeDirect {
			rule["target_addr"] = r.TargetAddr
		} else {
			rule["relay_chain"] = r.RelayChain
			rule["exit_addr"] = r.ExitAddr
		}
		ruleList[i] = rule
	}

	return map[string]interface{}{
		"rules": ruleList,
		"total": total,
		"page":  p.Page,
		"limit": p.Limit,
		"pages": pages,
	}, nil
}

func (m *GetForwardRuleListMethod) RequireAuth() bool { return true }

// GetForwardRuleMethod - 获取单个转发规则
type GetForwardRuleMethod struct {
	storage *storage.Storage
}

func NewGetForwardRuleMethod(s *storage.Storage) *GetForwardRuleMethod {
	return &GetForwardRuleMethod{storage: s}
}

func (m *GetForwardRuleMethod) Name() string { return "getForwardRule" }

type GetForwardRuleParams struct {
	ID string `json:"id"`
}

func (m *GetForwardRuleMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p GetForwardRuleParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	rule, err := m.storage.Forward.GetByID(p.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("rule not found")
		}
		return nil, fmt.Errorf("failed to get rule: %w", err)
	}

	result := map[string]interface{}{
		"id":            rule.ID,
		"name":          rule.Name,
		"type":          rule.Type,
		"enabled":       rule.Enabled,
		"listen_addr":   rule.ListenAddr,
		"listen_client": rule.ListenClient,
		"created_at":    rule.CreatedAt,
		"updated_at":    rule.UpdatedAt,
	}

	if rule.Type == model.ForwardTypeDirect {
		result["target_addr"] = rule.TargetAddr
	} else {
		result["relay_chain"] = rule.RelayChain
		result["exit_addr"] = rule.ExitAddr
	}

	return result, nil
}

func (m *GetForwardRuleMethod) RequireAuth() bool { return true }

// UpdateForwardRuleMethod - 更新转发规则
type UpdateForwardRuleMethod struct {
	storage  *storage.Storage
	wsServer *relay.WSServer
}

func NewUpdateForwardRuleMethod(s *storage.Storage, ws *relay.WSServer) *UpdateForwardRuleMethod {
	return &UpdateForwardRuleMethod{storage: s, wsServer: ws}
}

func (m *UpdateForwardRuleMethod) Name() string { return "updateForwardRule" }

type UpdateForwardRuleParams struct {
	ID         string    `json:"id"`
	Name       *string   `json:"name"`
	ListenAddr *string   `json:"listen_addr"`
	TargetAddr *string   `json:"target_addr"`
	RelayChain *[]string `json:"relay_chain"`
	ExitAddr   *string   `json:"exit_addr"`
}

func (m *UpdateForwardRuleMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p UpdateForwardRuleParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	rule, err := m.storage.Forward.GetByID(p.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("rule not found")
		}
		return nil, fmt.Errorf("failed to get rule: %w", err)
	}

	if p.Name != nil {
		rule.Name = *p.Name
	}
	if p.ListenAddr != nil {
		rule.ListenAddr = *p.ListenAddr
	}
	if p.TargetAddr != nil {
		rule.TargetAddr = *p.TargetAddr
	}
	if p.RelayChain != nil {
		rule.RelayChain = *p.RelayChain
	}
	if p.ExitAddr != nil {
		rule.ExitAddr = *p.ExitAddr
	}

	if err := m.storage.Forward.Update(rule); err != nil {
		return nil, fmt.Errorf("failed to update rule: %w", err)
	}

	// 通知相关 client 规则已更新
	if m.wsServer != nil {
		m.wsServer.NotifyRuleUpdate(rule.ListenClient)
	}

	return map[string]interface{}{
		"success": true,
	}, nil
}

func (m *UpdateForwardRuleMethod) RequireAuth() bool { return true }

// DeleteForwardRuleMethod - 删除转发规则
type DeleteForwardRuleMethod struct {
	storage  *storage.Storage
	wsServer *relay.WSServer
}

func NewDeleteForwardRuleMethod(s *storage.Storage, ws *relay.WSServer) *DeleteForwardRuleMethod {
	return &DeleteForwardRuleMethod{storage: s, wsServer: ws}
}

func (m *DeleteForwardRuleMethod) Name() string { return "deleteForwardRule" }

type DeleteForwardRuleParams struct {
	ID string `json:"id"`
}

func (m *DeleteForwardRuleMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p DeleteForwardRuleParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	// 先获取规则，以便知道要通知哪个 client
	rule, err := m.storage.Forward.GetByID(p.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to get rule: %w", err)
	}

	if err := m.storage.Forward.Delete(p.ID); err != nil {
		return nil, fmt.Errorf("failed to delete rule: %w", err)
	}

	// 通知相关 client 规则已更新
	if m.wsServer != nil && rule != nil {
		m.wsServer.NotifyRuleUpdate(rule.ListenClient)
	}

	return map[string]interface{}{
		"success": true,
	}, nil
}

func (m *DeleteForwardRuleMethod) RequireAuth() bool { return true }

// ToggleForwardRuleMethod - 启用/禁用转发规则
type ToggleForwardRuleMethod struct {
	storage  *storage.Storage
	wsServer *relay.WSServer
}

func NewToggleForwardRuleMethod(s *storage.Storage, ws *relay.WSServer) *ToggleForwardRuleMethod {
	return &ToggleForwardRuleMethod{storage: s, wsServer: ws}
}

func (m *ToggleForwardRuleMethod) Name() string { return "toggleForwardRule" }

type ToggleForwardRuleParams struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

func (m *ToggleForwardRuleMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p ToggleForwardRuleParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	// 先获取规则，以便知道要通知哪个 client
	rule, err := m.storage.Forward.GetByID(p.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("rule not found")
		}
		return nil, fmt.Errorf("failed to get rule: %w", err)
	}

	if err := m.storage.Forward.ToggleEnabled(p.ID, p.Enabled); err != nil {
		return nil, fmt.Errorf("failed to toggle rule: %w", err)
	}

	// 通知相关 client 规则已更新
	if m.wsServer != nil {
		m.wsServer.NotifyRuleUpdate(rule.ListenClient)
	}

	return map[string]interface{}{
		"success": true,
	}, nil
}

func (m *ToggleForwardRuleMethod) RequireAuth() bool { return true }
