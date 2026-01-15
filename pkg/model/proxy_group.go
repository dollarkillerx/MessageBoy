package model

import (
	"time"
)

// LoadBalanceMethod 负载均衡方法
type LoadBalanceMethod string

const (
	LoadBalanceRoundRobin LoadBalanceMethod = "round_robin"
	LoadBalanceRandom     LoadBalanceMethod = "random"
	LoadBalanceLeastConn  LoadBalanceMethod = "least_conn"
	LoadBalanceIPHash     LoadBalanceMethod = "ip_hash"
)

// NodeStatus 节点状态
type NodeStatus string

const (
	NodeStatusHealthy   NodeStatus = "healthy"
	NodeStatusUnhealthy NodeStatus = "unhealthy"
	NodeStatusUnknown   NodeStatus = "unknown"
)

// ProxyGroup 代理组模型
type ProxyGroup struct {
	ID                  string            `json:"id" gorm:"primaryKey;size:36"`
	Name                string            `json:"name" gorm:"size:100;not null;uniqueIndex"`
	Description         string            `json:"description" gorm:"type:text"`
	LoadBalanceMethod   LoadBalanceMethod `json:"load_balance_method" gorm:"size:20"`
	HealthCheckEnabled  bool              `json:"health_check_enabled"`
	HealthCheckInterval int               `json:"health_check_interval"`
	HealthCheckTimeout  int               `json:"health_check_timeout"`
	HealthCheckRetries  int               `json:"health_check_retries"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 关联 (不使用外键约束，仅用于查询时 Preload)
	Nodes []ProxyGroupNode `json:"nodes,omitempty" gorm:"-"`
}

func (ProxyGroup) TableName() string {
	return "proxy_groups"
}

// SetDefaults 设置默认值
func (g *ProxyGroup) SetDefaults() {
	if g.LoadBalanceMethod == "" {
		g.LoadBalanceMethod = LoadBalanceRoundRobin
	}
	if g.HealthCheckInterval == 0 {
		g.HealthCheckInterval = 30
	}
	if g.HealthCheckTimeout == 0 {
		g.HealthCheckTimeout = 5
	}
	if g.HealthCheckRetries == 0 {
		g.HealthCheckRetries = 3
	}
}

// ProxyGroupNode 代理组节点模型
type ProxyGroupNode struct {
	ID       string     `json:"id" gorm:"primaryKey;size:36"`
	GroupID  string     `json:"group_id" gorm:"size:36;not null;index"`
	ClientID string     `json:"client_id" gorm:"size:36;not null;index"`
	Priority int        `json:"priority"`
	Weight   int        `json:"weight"`
	Status   NodeStatus `json:"status" gorm:"size:20"`

	// 健康检查状态
	LastCheckAt *time.Time `json:"last_check_at"`
	LastCheckOK bool       `json:"last_check_ok"`
	FailCount   int        `json:"fail_count"`

	// 连接统计
	ActiveConns int   `json:"active_conns"`
	TotalConns  int64 `json:"total_conns"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 关联 (不创建外键约束，手动查询填充)
	Client *Client `json:"client,omitempty" gorm:"-"`
}

func (ProxyGroupNode) TableName() string {
	return "proxy_group_nodes"
}

// SetDefaults 设置默认值
func (n *ProxyGroupNode) SetDefaults() {
	if n.Priority == 0 {
		n.Priority = 100
	}
	if n.Weight == 0 {
		n.Weight = 100
	}
	if n.Status == "" {
		n.Status = NodeStatusUnknown
	}
}
