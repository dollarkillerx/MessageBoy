package api

import (
	"context"
	"encoding/json"

	"github.com/dollarkillerx/MessageBoy/internal/storage"
	"github.com/dollarkillerx/MessageBoy/pkg/model"
)

// GetTrafficSummaryMethod 获取流量汇总
type GetTrafficSummaryMethod struct {
	storage *storage.Storage
}

func NewGetTrafficSummaryMethod(s *storage.Storage) *GetTrafficSummaryMethod {
	return &GetTrafficSummaryMethod{storage: s}
}

func (m *GetTrafficSummaryMethod) Name() string        { return "getTrafficSummary" }
func (m *GetTrafficSummaryMethod) RequireAuth() bool   { return true }

func (m *GetTrafficSummaryMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	summaries, err := m.storage.Traffic.GetSummaryByRule()
	if err != nil {
		return nil, err
	}
	return summaries, nil
}

// GetTotalTrafficMethod 获取总流量统计
type GetTotalTrafficMethod struct {
	storage *storage.Storage
}

func NewGetTotalTrafficMethod(s *storage.Storage) *GetTotalTrafficMethod {
	return &GetTotalTrafficMethod{storage: s}
}

func (m *GetTotalTrafficMethod) Name() string        { return "getTotalTraffic" }
func (m *GetTotalTrafficMethod) RequireAuth() bool   { return true }

func (m *GetTotalTrafficMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	bytesIn, bytesOut, totalConns, err := m.storage.Traffic.GetTotalStats()
	if err != nil {
		return nil, err
	}

	activeConns := m.storage.Traffic.GetRealtimeActiveConns()

	return map[string]interface{}{
		"bytes_in":           bytesIn,
		"bytes_out":          bytesOut,
		"total_bytes":        bytesIn + bytesOut,
		"bytes_in_str":       model.FormatBytes(bytesIn),
		"bytes_out_str":      model.FormatBytes(bytesOut),
		"total_bytes_str":    model.FormatBytes(bytesIn + bytesOut),
		"total_connections":  totalConns,
		"active_connections": activeConns,
	}, nil
}

// GetTodayTrafficMethod 获取今日流量
type GetTodayTrafficMethod struct {
	storage *storage.Storage
}

func NewGetTodayTrafficMethod(s *storage.Storage) *GetTodayTrafficMethod {
	return &GetTodayTrafficMethod{storage: s}
}

func (m *GetTodayTrafficMethod) Name() string        { return "getTodayTraffic" }
func (m *GetTodayTrafficMethod) RequireAuth() bool   { return true }

func (m *GetTodayTrafficMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	stats, err := m.storage.Traffic.GetTodayStats()
	if err != nil {
		return nil, err
	}

	var bytesIn, bytesOut, totalConns int64
	for _, s := range stats {
		bytesIn += s.BytesIn
		bytesOut += s.BytesOut
		totalConns += s.TotalConnections
	}

	activeConns := m.storage.Traffic.GetRealtimeActiveConns()

	return map[string]interface{}{
		"bytes_in":           bytesIn,
		"bytes_out":          bytesOut,
		"total_bytes":        bytesIn + bytesOut,
		"bytes_in_str":       model.FormatBytes(bytesIn),
		"bytes_out_str":      model.FormatBytes(bytesOut),
		"total_bytes_str":    model.FormatBytes(bytesIn + bytesOut),
		"total_connections":  totalConns,
		"active_connections": activeConns,
	}, nil
}

// GetClientBandwidthMethod 获取客户端带宽统计
type GetClientBandwidthMethod struct {
	storage *storage.Storage
}

func NewGetClientBandwidthMethod(s *storage.Storage) *GetClientBandwidthMethod {
	return &GetClientBandwidthMethod{storage: s}
}

func (m *GetClientBandwidthMethod) Name() string      { return "getClientBandwidth" }
func (m *GetClientBandwidthMethod) RequireAuth() bool { return true }

func (m *GetClientBandwidthMethod) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	// 更新带宽计算
	m.storage.Traffic.UpdateBandwidth()

	// 获取带宽数据
	bandwidths := m.storage.Traffic.GetClientBandwidth()

	// 格式化输出
	result := make([]map[string]interface{}, 0, len(bandwidths))
	for _, bw := range bandwidths {
		result = append(result, map[string]interface{}{
			"client_id":         bw.ClientID,
			"bandwidth_in":      bw.BandwidthIn,
			"bandwidth_out":     bw.BandwidthOut,
			"bandwidth_in_str":  model.FormatBandwidth(bw.BandwidthIn),
			"bandwidth_out_str": model.FormatBandwidth(bw.BandwidthOut),
		})
	}

	return result, nil
}
