package model

import (
	"fmt"
	"time"
)

// TrafficStats 流量统计
type TrafficStats struct {
	ID       string `json:"id" gorm:"primaryKey;type:varchar(36)"`
	RuleID   string `json:"rule_id" gorm:"type:varchar(36);not null;index"`
	ClientID string `json:"client_id" gorm:"type:varchar(36);not null;index"`

	// 流量统计 (字节)
	BytesIn    int64 `json:"bytes_in" gorm:"default:0"`
	BytesOut   int64 `json:"bytes_out" gorm:"default:0"`
	TotalBytes int64 `json:"total_bytes" gorm:"default:0"`

	// 连接统计
	Connections int64 `json:"connections" gorm:"default:0"`
	ActiveConns int   `json:"active_conns" gorm:"default:0"`

	// 时间范围 (用于按时间段统计)
	PeriodStart time.Time `json:"period_start" gorm:"index"`
	PeriodEnd   time.Time `json:"period_end"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (TrafficStats) TableName() string {
	return "traffic_stats"
}

// TrafficSummary 流量汇总 (用于 API 返回)
type TrafficSummary struct {
	RuleID     string `json:"rule_id"`
	RuleName   string `json:"rule_name"`
	ClientID   string `json:"client_id"`
	ClientName string `json:"client_name"`

	BytesIn    int64 `json:"bytes_in"`
	BytesOut   int64 `json:"bytes_out"`
	TotalBytes int64 `json:"total_bytes"`

	Connections int64 `json:"connections"`
	ActiveConns int   `json:"active_conns"`

	// 格式化的流量字符串
	BytesInStr    string `json:"bytes_in_str"`
	BytesOutStr   string `json:"bytes_out_str"`
	TotalBytesStr string `json:"total_bytes_str"`
}

// FormatBytes 格式化字节数为人类可读的字符串
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	var value float64
	var unit string

	switch {
	case bytes >= TB:
		value = float64(bytes) / TB
		unit = "TB"
	case bytes >= GB:
		value = float64(bytes) / GB
		unit = "GB"
	case bytes >= MB:
		value = float64(bytes) / MB
		unit = "MB"
	case bytes >= KB:
		value = float64(bytes) / KB
		unit = "KB"
	default:
		return fmt.Sprintf("%d B", bytes)
	}

	return fmt.Sprintf("%.2f %s", value, unit)
}

// FormatBandwidth 格式化带宽为人类可读的字符串 (bytes per second)
func FormatBandwidth(bytesPerSec int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	var value float64
	var unit string

	switch {
	case bytesPerSec >= GB:
		value = float64(bytesPerSec) / GB
		unit = "GB/s"
	case bytesPerSec >= MB:
		value = float64(bytesPerSec) / MB
		unit = "MB/s"
	case bytesPerSec >= KB:
		value = float64(bytesPerSec) / KB
		unit = "KB/s"
	default:
		return fmt.Sprintf("%d B/s", bytesPerSec)
	}

	return fmt.Sprintf("%.2f %s", value, unit)
}
