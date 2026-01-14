package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/dollarkillerx/MessageBoy/internal/conf"
	"github.com/dollarkillerx/MessageBoy/internal/middleware"
)

const Version = "2.0.0"

// PingMethod - 心跳检测
type PingMethod struct{}

func (m *PingMethod) Name() string { return "ping" }

func (m *PingMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return map[string]interface{}{
		"pong":    true,
		"time":    time.Now().Unix(),
		"version": Version,
	}, nil
}

func (m *PingMethod) RequireAuth() bool { return false }

// AdminLoginMethod - 管理员登录
type AdminLoginMethod struct {
	adminCfg   *conf.AdminConfig
	jwtManager *middleware.JWTManager
}

func NewAdminLoginMethod(adminCfg *conf.AdminConfig, jwtManager *middleware.JWTManager) *AdminLoginMethod {
	return &AdminLoginMethod{
		adminCfg:   adminCfg,
		jwtManager: jwtManager,
	}
}

func (m *AdminLoginMethod) Name() string { return "adminLogin" }

type AdminLoginParams struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (m *AdminLoginMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var p AdminLoginParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("invalid params")
	}

	if p.Username == "" || p.Password == "" {
		return nil, errors.New("username and password are required")
	}

	if p.Username != m.adminCfg.Username || p.Password != m.adminCfg.Password {
		return nil, errors.New("invalid credentials")
	}

	token, expireAt, err := m.jwtManager.GenerateToken(p.Username)
	if err != nil {
		return nil, errors.New("failed to generate token")
	}

	return map[string]interface{}{
		"token":     token,
		"expire_at": expireAt.Format(time.RFC3339),
	}, nil
}

func (m *AdminLoginMethod) RequireAuth() bool { return false }
