package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
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
			"status":             r.Status,
			"last_error":         r.LastError,
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
	ID           string    `json:"id"`
	Name         *string   `json:"name"`
	ListenAddr   *string   `json:"listen_addr"`
	ListenClient *string   `json:"listen_client"`
	TargetAddr   *string   `json:"target_addr"`
	RelayChain   *[]string `json:"relay_chain"`
	ExitAddr     *string   `json:"exit_addr"`
}

func (m *UpdateForwardRuleMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	log.Info().RawJSON("params", params).Bool("wsServer_nil", m.wsServer == nil).Msg("=== updateForwardRule called ===")

	var p UpdateForwardRuleParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	log.Info().Str("rule_id", p.ID).Msg("Updating forward rule")

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

	// 记录原来的监听地址和客户端
	oldListenAddr := rule.ListenAddr
	oldListenClient := rule.ListenClient

	if p.Name != nil {
		rule.Name = *p.Name
	}
	if p.ListenAddr != nil {
		rule.ListenAddr = *p.ListenAddr
	}
	if p.ListenClient != nil {
		// 验证新客户端存在
		_, err := m.storage.Client.GetByID(*p.ListenClient)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("new listen_client not found")
			}
			return nil, fmt.Errorf("failed to verify new client: %w", err)
		}
		rule.ListenClient = *p.ListenClient
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

	// 如果监听地址或客户端发生变化，检查新端口是否可用
	if (rule.ListenAddr != oldListenAddr || rule.ListenClient != oldListenClient) && m.wsServer != nil {
		log.Info().
			Str("old_addr", oldListenAddr).
			Str("new_addr", rule.ListenAddr).
			Str("old_client", oldListenClient).
			Str("new_client", rule.ListenClient).
			Msg("Listen config changed, checking port availability on new client")

		available, errMsg := m.wsServer.CheckPortAvailable(
			rule.ListenClient,
			rule.ListenAddr,
			rule.ID,
			5*time.Second, // 超时时间
		)
		if !available {
			log.Warn().
				Str("addr", rule.ListenAddr).
				Str("client", rule.ListenClient).
				Str("error", errMsg).
				Msg("Port check failed")
			return nil, fmt.Errorf("端口 %s 在客户端 %s 上不可用: %s", rule.ListenAddr, rule.ListenClient, errMsg)
		}
		log.Info().Str("addr", rule.ListenAddr).Str("client", rule.ListenClient).Msg("Port check passed")
	}

	if err := m.storage.Forward.Update(rule); err != nil {
		return nil, fmt.Errorf("failed to update rule: %w", err)
	}

	log.Info().
		Str("rule_id", rule.ID).
		Str("old_client", oldListenClient).
		Str("new_client", rule.ListenClient).
		Str("listen_addr", rule.ListenAddr).
		Str("type", string(rule.Type)).
		Msg("Forward rule updated, preparing to notify clients")

	// 通知相关 client 规则已更新
	if m.wsServer != nil {
		// 如果客户端发生变化，需要先通知旧客户端停止，等待后再通知新客户端启动
		if oldListenClient != rule.ListenClient {
			// 通知旧客户端（让它停止这个规则）
			ok := m.wsServer.NotifyRuleUpdate(oldListenClient)
			log.Info().
				Str("rule_id", rule.ID).
				Str("old_client", oldListenClient).
				Bool("notify_success", ok).
				Msg("Notified old client to stop rule")

			// 等待旧客户端停止规则并释放端口
			time.Sleep(500 * time.Millisecond)

			// 通知新客户端（让它启动这个规则）
			ok = m.wsServer.NotifyRuleUpdate(rule.ListenClient)
			log.Info().
				Str("rule_id", rule.ID).
				Str("new_client", rule.ListenClient).
				Bool("notify_success", ok).
				Msg("Notified new client to start rule")
		} else {
			// 客户端未变化，直接通知
			ok := m.wsServer.NotifyRuleUpdate(rule.ListenClient)
			log.Info().
				Str("rule_id", rule.ID).
				Str("client", rule.ListenClient).
				Bool("notify_success", ok).
				Msg("Notified client to update rule")
		}
	} else {
		log.Warn().Str("rule_id", rule.ID).Msg("wsServer is nil, cannot notify clients")
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
