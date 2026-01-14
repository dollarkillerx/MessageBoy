package model

import "time"

type ClientStatus string

const (
	ClientStatusOnline  ClientStatus = "online"
	ClientStatusOffline ClientStatus = "offline"
)

type Client struct {
	ID          string       `json:"id" gorm:"primaryKey;type:varchar(36)"`
	Name        string       `json:"name" gorm:"type:varchar(100);not null"`
	Description string       `json:"description" gorm:"type:text"`

	// SSH 信息
	SSHHost    string `json:"ssh_host" gorm:"type:varchar(255)"`
	SSHPort    int    `json:"ssh_port" gorm:"default:22"`
	SSHUser    string `json:"ssh_user" gorm:"type:varchar(100)"`
	SSHKeyPath string `json:"ssh_key_path" gorm:"type:varchar(500)"`

	// 认证信息
	Token     string `json:"token" gorm:"type:varchar(64);uniqueIndex"`
	SecretKey string `json:"-" gorm:"type:varchar(64)"`

	// 连接状态
	Status   ClientStatus `json:"status" gorm:"type:varchar(20);default:offline"`
	LastIP   string       `json:"last_ip" gorm:"type:varchar(45)"`
	LastSeen *time.Time   `json:"last_seen"`
	Hostname string       `json:"hostname" gorm:"type:varchar(255)"`
	Version  string       `json:"version" gorm:"type:varchar(20)"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Client) TableName() string {
	return "clients"
}
