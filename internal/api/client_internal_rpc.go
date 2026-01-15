package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/dollarkillerx/MessageBoy/internal/conf"
	"github.com/dollarkillerx/MessageBoy/internal/storage"
	"github.com/dollarkillerx/MessageBoy/pkg/model"
)

// ClientRegisterMethod - Client 注册
type ClientRegisterMethod struct {
	storage *storage.Storage
	cfg     *conf.Config
}

func NewClientRegisterMethod(s *storage.Storage, cfg *conf.Config) *ClientRegisterMethod {
	return &ClientRegisterMethod{storage: s, cfg: cfg}
}

func (m *ClientRegisterMethod) Name() string { return "clientRegister" }

type ClientRegisterParams struct {
	Token    string `json:"token"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
}

func (m *ClientRegisterMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p ClientRegisterParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.Token == "" {
		return nil, errors.New("token is required")
	}

	client, err := m.storage.Client.GetByToken(p.Token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid token")
		}
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}

	// 获取客户端 IP
	clientIP := ""
	if ginCtx := GetGinContext(ctx); ginCtx != nil {
		clientIP = ginCtx.ClientIP()
	}

	// 更新 client 信息
	now := time.Now()
	client.Status = model.ClientStatusOnline
	client.LastIP = clientIP
	client.LastSeen = &now
	client.Hostname = p.Hostname
	client.Version = p.Version

	if err := m.storage.Client.Update(client); err != nil {
		return nil, fmt.Errorf("failed to update client: %w", err)
	}

	return map[string]interface{}{
		"client_id":          client.ID,
		"secret_key":         client.SecretKey,
		"ws_endpoint":        fmt.Sprintf("%s%s", m.cfg.Server.ExternalURL, m.cfg.WebSocket.Endpoint),
		"heartbeat_interval": m.cfg.WebSocket.PingInterval,
	}, nil
}

func (m *ClientRegisterMethod) RequireAuth() bool { return false }

// ClientHeartbeatMethod - Client 心跳
type ClientHeartbeatMethod struct {
	storage *storage.Storage
}

func NewClientHeartbeatMethod(s *storage.Storage) *ClientHeartbeatMethod {
	return &ClientHeartbeatMethod{storage: s}
}

func (m *ClientHeartbeatMethod) Name() string { return "clientHeartbeat" }

type ClientHeartbeatParams struct {
	ClientID    string `json:"client_id"`
	Uptime      int64  `json:"uptime"`
	Connections int    `json:"connections"`
	BytesIn     int64  `json:"bytes_in"`
	BytesOut    int64  `json:"bytes_out"`
}

func (m *ClientHeartbeatMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p ClientHeartbeatParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ClientID == "" {
		return nil, errors.New("client_id is required")
	}

	clientIP := ""
	if ginCtx := GetGinContext(ctx); ginCtx != nil {
		clientIP = ginCtx.ClientIP()
	}

	if err := m.storage.Client.UpdateStatus(p.ClientID, model.ClientStatusOnline, clientIP); err != nil {
		return nil, fmt.Errorf("failed to update heartbeat: %w", err)
	}

	return map[string]interface{}{
		"ack":         true,
		"server_time": time.Now().Unix(),
	}, nil
}

func (m *ClientHeartbeatMethod) RequireAuth() bool { return false }

// ClientGetRulesMethod - Client 获取转发规则
type ClientGetRulesMethod struct {
	storage *storage.Storage
}

func NewClientGetRulesMethod(s *storage.Storage) *ClientGetRulesMethod {
	return &ClientGetRulesMethod{storage: s}
}

func (m *ClientGetRulesMethod) Name() string { return "clientGetRules" }

type ClientGetRulesParams struct {
	ClientID string `json:"client_id"`
}

func (m *ClientGetRulesMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p ClientGetRulesParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ClientID == "" {
		return nil, errors.New("client_id is required")
	}

	rules, err := m.storage.Forward.GetByClientID(p.ClientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rules: %w", err)
	}

	ruleList := make([]map[string]interface{}, len(rules))
	for i, r := range rules {
		rule := map[string]interface{}{
			"id":          r.ID,
			"type":        r.Type,
			"listen_addr": r.ListenAddr,
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
		"rules":   ruleList,
		"version": fmt.Sprintf("%d", time.Now().Unix()),
	}, nil
}

func (m *ClientGetRulesMethod) RequireAuth() bool { return false }

// ClientReportTrafficMethod - Client 上报流量统计
type ClientReportTrafficMethod struct {
	storage *storage.Storage
}

func NewClientReportTrafficMethod(s *storage.Storage) *ClientReportTrafficMethod {
	return &ClientReportTrafficMethod{storage: s}
}

func (m *ClientReportTrafficMethod) Name() string { return "clientReportTraffic" }

type TrafficReportItem struct {
	RuleID      string `json:"rule_id"`
	BytesIn     int64  `json:"bytes_in"`
	BytesOut    int64  `json:"bytes_out"`
	Connections int64  `json:"connections"`
}

type ClientReportTrafficParams struct {
	ClientID string              `json:"client_id"`
	Reports  []TrafficReportItem `json:"reports"`
}

func (m *ClientReportTrafficMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p ClientReportTrafficParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ClientID == "" {
		return nil, errors.New("client_id is required")
	}

	// 累加流量到统计器
	for _, report := range p.Reports {
		if report.BytesIn > 0 {
			m.storage.Traffic.AddBytesIn(report.RuleID, p.ClientID, report.BytesIn)
		}
		if report.BytesOut > 0 {
			m.storage.Traffic.AddBytesOut(report.RuleID, p.ClientID, report.BytesOut)
		}
		if report.Connections > 0 {
			m.storage.Traffic.AddConnections(report.RuleID, p.ClientID, report.Connections)
		}
	}

	return map[string]interface{}{
		"ack": true,
	}, nil
}

func (m *ClientReportTrafficMethod) RequireAuth() bool { return false }

// ClientReportRuleStatusMethod - Client 上报规则状态
type ClientReportRuleStatusMethod struct {
	storage *storage.Storage
}

func NewClientReportRuleStatusMethod(s *storage.Storage) *ClientReportRuleStatusMethod {
	return &ClientReportRuleStatusMethod{storage: s}
}

func (m *ClientReportRuleStatusMethod) Name() string { return "clientReportRuleStatus" }

type RuleStatusReportItem struct {
	RuleID string `json:"rule_id"`
	Status string `json:"status"` // running, error, stopped
	Error  string `json:"error,omitempty"`
}

type ClientReportRuleStatusParams struct {
	ClientID string                 `json:"client_id"`
	Reports  []RuleStatusReportItem `json:"reports"`
}

func (m *ClientReportRuleStatusMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p ClientReportRuleStatusParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ClientID == "" {
		return nil, errors.New("client_id is required")
	}

	// 更新规则状态
	for _, report := range p.Reports {
		status := model.RuleStatus(report.Status)
		if err := m.storage.Forward.UpdateStatus(report.RuleID, status, report.Error); err != nil {
			// 忽略单条更新失败，继续处理其他
			continue
		}
	}

	return map[string]interface{}{
		"ack": true,
	}, nil
}

func (m *ClientReportRuleStatusMethod) RequireAuth() bool { return false }
