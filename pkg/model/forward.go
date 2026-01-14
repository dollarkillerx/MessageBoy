package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type ForwardType string

const (
	ForwardTypeDirect ForwardType = "direct"
	ForwardTypeRelay  ForwardType = "relay"
)

type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan StringSlice: %v", value)
	}
	return json.Unmarshal(bytes, s)
}

func (StringSlice) GormDataType() string {
	return "jsonb"
}

type ForwardRule struct {
	ID      string      `json:"id" gorm:"primaryKey;type:varchar(36)"`
	Name    string      `json:"name" gorm:"type:varchar(100);not null"`
	Type    ForwardType `json:"type" gorm:"type:varchar(20);not null"`
	Enabled bool        `json:"enabled" gorm:"default:true"`

	ListenAddr   string `json:"listen_addr" gorm:"type:varchar(100);not null"`
	ListenClient string `json:"listen_client" gorm:"type:varchar(36);not null"`

	// 直接转发
	TargetAddr string `json:"target_addr,omitempty" gorm:"type:varchar(255)"`

	// 中继转发
	RelayChain StringSlice `json:"relay_chain,omitempty" gorm:"type:jsonb"`
	ExitAddr   string      `json:"exit_addr,omitempty" gorm:"type:varchar(255)"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ForwardRule) TableName() string {
	return "forward_rules"
}
