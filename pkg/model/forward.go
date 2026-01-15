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
		return "[]", nil
	}
	return json.Marshal(s)
}

func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to scan StringSlice: %v", value)
	}
	return json.Unmarshal(bytes, s)
}

func (StringSlice) GormDataType() string {
	return "text"
}

type ForwardRule struct {
	ID      string      `json:"id" gorm:"primaryKey;size:36"`
	Name    string      `json:"name" gorm:"size:100;not null"`
	Type    ForwardType `json:"type" gorm:"size:20;not null"`
	Enabled bool        `json:"enabled"`

	ListenAddr   string `json:"listen_addr" gorm:"size:100;not null"`
	ListenClient string `json:"listen_client" gorm:"size:36;not null;index"`

	// 直接转发
	TargetAddr string `json:"target_addr,omitempty" gorm:"size:255"`

	// 中继转发
	RelayChain StringSlice `json:"relay_chain,omitempty" gorm:"type:text"`
	ExitAddr   string      `json:"exit_addr,omitempty" gorm:"size:255"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ForwardRule) TableName() string {
	return "forward_rules"
}

// SetDefaults 设置默认值
func (r *ForwardRule) SetDefaults() {
	if r.Type == "" {
		r.Type = ForwardTypeDirect
	}
}
