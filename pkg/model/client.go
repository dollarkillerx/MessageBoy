package model

import "time"

type ClientStatus string

const (
	ClientStatusOnline  ClientStatus = "online"
	ClientStatusOffline ClientStatus = "offline"
)

type Client struct {
	ID          string `json:"id" gorm:"primaryKey;size:36"`
	Name        string `json:"name" gorm:"size:100;not null"`
	Description string `json:"description" gorm:"type:text"`

	// SSH 信息
	SSHHost     string `json:"ssh_host" gorm:"size:255"`
	SSHPort     int    `json:"ssh_port"`
	SSHUser     string `json:"ssh_user" gorm:"size:100"`
	SSHPassword string `json:"ssh_password,omitempty" gorm:"size:255"`
	SSHKeyPath  string `json:"ssh_key_path" gorm:"size:500"`

	// 认证信息
	Token     string `json:"token" gorm:"size:64;uniqueIndex"`
	SecretKey string `json:"-" gorm:"size:64"`

	// 连接状态
	Status   ClientStatus `json:"status" gorm:"size:20"`
	LastIP   string       `json:"last_ip" gorm:"size:45"`
	LastSeen *time.Time   `json:"last_seen"`
	Hostname string       `json:"hostname" gorm:"size:255"`
	Version  string       `json:"version" gorm:"size:20"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Client) TableName() string {
	return "mb_clients"
}

// SetDefaults 设置默认值
func (c *Client) SetDefaults() {
	if c.SSHPort == 0 {
		c.SSHPort = 22
	}
	if c.Status == "" {
		c.Status = ClientStatusOffline
	}
}
