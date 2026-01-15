package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/dollarkillerx/MessageBoy/internal/conf"
	"github.com/dollarkillerx/MessageBoy/internal/storage"
	"github.com/dollarkillerx/MessageBoy/pkg/model"
)

// CreateClientMethod - 创建 Client
type CreateClientMethod struct {
	storage *storage.Storage
	cfg     *conf.Config
}

func NewCreateClientMethod(s *storage.Storage, cfg *conf.Config) *CreateClientMethod {
	return &CreateClientMethod{storage: s, cfg: cfg}
}

func (m *CreateClientMethod) Name() string { return "createClient" }

type CreateClientParams struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	RelayIP     string `json:"relay_ip"` // 中继地址，为空时使用连接 IP
	SSHHost     string `json:"ssh_host"`
	SSHPort     int    `json:"ssh_port"`
	SSHUser     string `json:"ssh_user"`
	SSHPassword string `json:"ssh_password"`
	SSHKeyPath  string `json:"ssh_key_path"`
}

func (m *CreateClientMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p CreateClientParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.Name == "" {
		return nil, errors.New("name is required")
	}

	token := generateToken()
	secretKey := generateSecretKey()

	client := &model.Client{
		ID:          uuid.New().String(),
		Name:        p.Name,
		Description: p.Description,
		RelayIP:     p.RelayIP,
		SSHHost:     p.SSHHost,
		SSHPort:     p.SSHPort,
		SSHUser:     p.SSHUser,
		SSHPassword: p.SSHPassword,
		SSHKeyPath:  p.SSHKeyPath,
		Token:       token,
		SecretKey:   secretKey,
		Status:      model.ClientStatusOffline,
	}

	if client.SSHPort == 0 {
		client.SSHPort = 22
	}

	if err := m.storage.Client.Create(client); err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return map[string]interface{}{
		"id":              client.ID,
		"name":            client.Name,
		"token":           client.Token,
		"install_command": m.generateInstallCommand(client.Token),
	}, nil
}

func (m *CreateClientMethod) generateInstallCommand(token string) string {
	return fmt.Sprintf(
		"curl -sSL %s/install.sh | bash -s -- --server %s --token %s",
		m.cfg.Server.ExternalURL, m.cfg.Server.ExternalURL, token,
	)
}

func (m *CreateClientMethod) RequireAuth() bool { return true }

// GetClientListMethod - 获取 Client 列表
type GetClientListMethod struct {
	storage *storage.Storage
}

func NewGetClientListMethod(s *storage.Storage) *GetClientListMethod {
	return &GetClientListMethod{storage: s}
}

func (m *GetClientListMethod) Name() string { return "getClientList" }

type GetClientListParams struct {
	Page   int    `json:"page"`
	Limit  int    `json:"limit"`
	Search string `json:"search"`
	Status string `json:"status"`
}

func (m *GetClientListMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p GetClientListParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.Page <= 0 {
		p.Page = 1
	}
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 20
	}

	clients, total, err := m.storage.Client.List(storage.ClientListParams{
		Page:   p.Page,
		Limit:  p.Limit,
		Search: p.Search,
		Status: p.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get client list: %w", err)
	}

	pages := (total + int64(p.Limit) - 1) / int64(p.Limit)

	clientList := make([]map[string]interface{}, len(clients))
	for i, c := range clients {
		clientList[i] = map[string]interface{}{
			"id":         c.ID,
			"name":       c.Name,
			"status":     c.Status,
			"last_ip":    c.LastIP,
			"relay_ip":   c.RelayIP,
			"last_seen":  c.LastSeen,
			"hostname":   c.Hostname,
			"version":    c.Version,
			"ssh_host":   c.SSHHost,
			"ssh_port":   c.SSHPort,
			"ssh_user":   c.SSHUser,
			"created_at": c.CreatedAt,
		}
	}

	return map[string]interface{}{
		"clients": clientList,
		"total":   total,
		"page":    p.Page,
		"limit":   p.Limit,
		"pages":   pages,
	}, nil
}

func (m *GetClientListMethod) RequireAuth() bool { return true }

// GetClientMethod - 获取单个 Client
type GetClientMethod struct {
	storage *storage.Storage
}

func NewGetClientMethod(s *storage.Storage) *GetClientMethod {
	return &GetClientMethod{storage: s}
}

func (m *GetClientMethod) Name() string { return "getClient" }

type GetClientParams struct {
	ID string `json:"id"`
}

func (m *GetClientMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p GetClientParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	client, err := m.storage.Client.GetByID(p.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("client not found")
		}
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	return map[string]interface{}{
		"id":           client.ID,
		"name":         client.Name,
		"description":  client.Description,
		"relay_ip":     client.RelayIP,
		"ssh_host":     client.SSHHost,
		"ssh_port":     client.SSHPort,
		"ssh_user":     client.SSHUser,
		"ssh_key_path": client.SSHKeyPath,
		"status":       client.Status,
		"last_ip":      client.LastIP,
		"last_seen":    client.LastSeen,
		"hostname":     client.Hostname,
		"version":      client.Version,
		"created_at":   client.CreatedAt,
		"updated_at":   client.UpdatedAt,
	}, nil
}

func (m *GetClientMethod) RequireAuth() bool { return true }

// UpdateClientMethod - 更新 Client
type UpdateClientMethod struct {
	storage *storage.Storage
}

func NewUpdateClientMethod(s *storage.Storage) *UpdateClientMethod {
	return &UpdateClientMethod{storage: s}
}

func (m *UpdateClientMethod) Name() string { return "updateClient" }

type UpdateClientParams struct {
	ID          string  `json:"id"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
	RelayIP     *string `json:"relay_ip"`
	SSHHost     *string `json:"ssh_host"`
	SSHPort     *int    `json:"ssh_port"`
	SSHUser     *string `json:"ssh_user"`
	SSHPassword *string `json:"ssh_password"`
	SSHKeyPath  *string `json:"ssh_key_path"`
}

func (m *UpdateClientMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p UpdateClientParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	client, err := m.storage.Client.GetByID(p.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("client not found")
		}
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	if p.Name != nil {
		client.Name = *p.Name
	}
	if p.Description != nil {
		client.Description = *p.Description
	}
	if p.RelayIP != nil {
		client.RelayIP = *p.RelayIP
	}
	if p.SSHHost != nil {
		client.SSHHost = *p.SSHHost
	}
	if p.SSHPort != nil {
		client.SSHPort = *p.SSHPort
	}
	if p.SSHUser != nil {
		client.SSHUser = *p.SSHUser
	}
	if p.SSHPassword != nil {
		client.SSHPassword = *p.SSHPassword
	}
	if p.SSHKeyPath != nil {
		client.SSHKeyPath = *p.SSHKeyPath
	}

	if err := m.storage.Client.Update(client); err != nil {
		return nil, fmt.Errorf("failed to update client: %w", err)
	}

	return map[string]interface{}{
		"success": true,
	}, nil
}

func (m *UpdateClientMethod) RequireAuth() bool { return true }

// DeleteClientMethod - 删除 Client
type DeleteClientMethod struct {
	storage *storage.Storage
}

func NewDeleteClientMethod(s *storage.Storage) *DeleteClientMethod {
	return &DeleteClientMethod{storage: s}
}

func (m *DeleteClientMethod) Name() string { return "deleteClient" }

type DeleteClientParams struct {
	ID string `json:"id"`
}

func (m *DeleteClientMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p DeleteClientParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	if err := m.storage.Client.Delete(p.ID); err != nil {
		return nil, fmt.Errorf("failed to delete client: %w", err)
	}

	return map[string]interface{}{
		"success": true,
	}, nil
}

func (m *DeleteClientMethod) RequireAuth() bool { return true }

// RegenerateClientTokenMethod - 重新生成 Token
type RegenerateClientTokenMethod struct {
	storage *storage.Storage
	cfg     *conf.Config
}

func NewRegenerateClientTokenMethod(s *storage.Storage, cfg *conf.Config) *RegenerateClientTokenMethod {
	return &RegenerateClientTokenMethod{storage: s, cfg: cfg}
}

func (m *RegenerateClientTokenMethod) Name() string { return "regenerateClientToken" }

type RegenerateClientTokenParams struct {
	ID string `json:"id"`
}

func (m *RegenerateClientTokenMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p RegenerateClientTokenParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	_, err := m.storage.Client.GetByID(p.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("client not found")
		}
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	newToken := generateToken()
	if err := m.storage.Client.UpdateToken(p.ID, newToken); err != nil {
		return nil, fmt.Errorf("failed to update token: %w", err)
	}

	return map[string]interface{}{
		"token":           newToken,
		"install_command": m.generateInstallCommand(newToken),
	}, nil
}

func (m *RegenerateClientTokenMethod) generateInstallCommand(token string) string {
	return fmt.Sprintf(
		"curl -sSL %s/install.sh | bash -s -- --server %s --token %s",
		m.cfg.Server.ExternalURL, m.cfg.Server.ExternalURL, token,
	)
}

func (m *RegenerateClientTokenMethod) RequireAuth() bool { return true }

// GetClientInstallCommandMethod - 获取安装命令
type GetClientInstallCommandMethod struct {
	storage *storage.Storage
	cfg     *conf.Config
}

func NewGetClientInstallCommandMethod(s *storage.Storage, cfg *conf.Config) *GetClientInstallCommandMethod {
	return &GetClientInstallCommandMethod{storage: s, cfg: cfg}
}

func (m *GetClientInstallCommandMethod) Name() string { return "getClientInstallCommand" }

type GetClientInstallCommandParams struct {
	ID string `json:"id"`
}

func (m *GetClientInstallCommandMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p GetClientInstallCommandParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.ID == "" {
		return nil, errors.New("id is required")
	}

	client, err := m.storage.Client.GetByID(p.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("client not found")
		}
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	return map[string]interface{}{
		"install_command": fmt.Sprintf(
			"curl -sSL %s/install.sh | bash -s -- --server %s --token %s",
			m.cfg.Server.ExternalURL, m.cfg.Server.ExternalURL, client.Token,
		),
		"manual_command": fmt.Sprintf(
			"./messageboy-client --server %s --token %s",
			m.cfg.Server.ExternalURL, client.Token,
		),
	}, nil
}

func (m *GetClientInstallCommandMethod) RequireAuth() bool { return true }

// 辅助函数
func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateSecretKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
