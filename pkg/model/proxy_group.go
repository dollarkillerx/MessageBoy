package model

import (
	"time"
)

// LoadBalanceMethod 负载均衡方法
type LoadBalanceMethod string

const (
	LoadBalanceRoundRobin   LoadBalanceMethod = "round_robin"   // 轮询
	LoadBalanceRandom       LoadBalanceMethod = "random"        // 随机
	LoadBalanceLeastConn    LoadBalanceMethod = "least_conn"    // 最少连接
	LoadBalanceIPHash       LoadBalanceMethod = "ip_hash"       // IP 哈希
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
	ID                  string            `json:"id" gorm:"primaryKey;type:varchar(36)"`
	Name                string            `json:"name" gorm:"type:varchar(100);not null;uniqueIndex"`
	Description         string            `json:"description" gorm:"type:text"`
	LoadBalanceMethod   LoadBalanceMethod `json:"load_balance_method" gorm:"type:varchar(20);default:round_robin"`

	// 健康检查配置
	HealthCheckEnabled  bool              `json:"health_check_enabled" gorm:"default:true"`
	HealthCheckInterval int               `json:"health_check_interval" gorm:"default:30"`  // 秒
	HealthCheckTimeout  int               `json:"health_check_timeout" gorm:"default:5"`    // 秒
	HealthCheckRetries  int               `json:"health_check_retries" gorm:"default:3"`    // 失败重试次数

	// 元数据
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

func (ProxyGroup) TableName() string {
	return "proxy_groups"
}

// ProxyGroupNode 代理组节点模型
type ProxyGroupNode struct {
	ID           string     `json:"id" gorm:"primaryKey;type:varchar(36)"`
	GroupID      string     `json:"group_id" gorm:"type:varchar(36);not null;index"`
	ClientID     string     `json:"client_id" gorm:"type:varchar(36);not null"`
	Priority     int        `json:"priority" gorm:"default:100"`           // 优先级，数字越小优先级越高
	Weight       int        `json:"weight" gorm:"default:100"`             // 权重，用于加权轮询
	Status       NodeStatus `json:"status" gorm:"type:varchar(20);default:unknown"`

	// 健康检查状态
	LastCheckAt  *time.Time `json:"last_check_at"`
	LastCheckOK  bool       `json:"last_check_ok" gorm:"default:false"`
	FailCount    int        `json:"fail_count" gorm:"default:0"`           // 连续失败次数

	// 连接统计
	ActiveConns  int        `json:"active_conns" gorm:"default:0"`         // 当前活跃连接数
	TotalConns   int64      `json:"total_conns" gorm:"default:0"`          // 总连接数

	// 元数据
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	// 关联
	Group        *ProxyGroup `json:"group,omitempty" gorm:"foreignKey:GroupID"`
	Client       *Client     `json:"client,omitempty" gorm:"foreignKey:ClientID"`
}

func (ProxyGroupNode) TableName() string {
	return "proxy_group_nodes"
}
