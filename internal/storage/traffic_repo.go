package storage

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/dollarkillerx/MessageBoy/pkg/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TrafficRepository 流量统计存储
type TrafficRepository struct {
	db *gorm.DB

	// 内存中的实时统计 (定期刷新到数据库)
	realtimeStats map[string]*RealtimeTraffic
	mu            sync.RWMutex

	// 带宽计算相关
	lastBytesIn    map[string]int64 // key: clientID
	lastBytesOut   map[string]int64
	lastUpdateTime time.Time
	bandwidthIn    map[string]int64 // bytes per second
	bandwidthOut   map[string]int64
	bwMu           sync.RWMutex
}

// RealtimeTraffic 实时流量统计 (内存中)
type RealtimeTraffic struct {
	RuleID      string
	ClientID    string
	BytesIn     int64 // 待刷新到数据库的增量
	BytesOut    int64
	ActiveConns int32 // 实时活跃连接数

	// 用于带宽计算的累积值（不会被重置）
	TotalBytesIn  int64
	TotalBytesOut int64
}

func NewTrafficRepository(db *gorm.DB) *TrafficRepository {
	return &TrafficRepository{
		db:            db,
		realtimeStats: make(map[string]*RealtimeTraffic),
		lastBytesIn:   make(map[string]int64),
		lastBytesOut:  make(map[string]int64),
		bandwidthIn:   make(map[string]int64),
		bandwidthOut:  make(map[string]int64),
	}
}

// getOrCreateStats 获取或创建实时统计
func (r *TrafficRepository) getOrCreateStats(ruleID, clientID string) *RealtimeTraffic {
	key := ruleID + ":" + clientID
	r.mu.RLock()
	stats, ok := r.realtimeStats[key]
	r.mu.RUnlock()

	if ok {
		return stats
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 双重检查
	if stats, ok = r.realtimeStats[key]; ok {
		return stats
	}

	stats = &RealtimeTraffic{
		RuleID:   ruleID,
		ClientID: clientID,
	}
	r.realtimeStats[key] = stats
	return stats
}

// AddBytesIn 增加入站流量
func (r *TrafficRepository) AddBytesIn(ruleID, clientID string, bytes int64) {
	stats := r.getOrCreateStats(ruleID, clientID)
	atomic.AddInt64(&stats.BytesIn, bytes)
	atomic.AddInt64(&stats.TotalBytesIn, bytes) // 累积值用于带宽计算
}

// AddBytesOut 增加出站流量
func (r *TrafficRepository) AddBytesOut(ruleID, clientID string, bytes int64) {
	stats := r.getOrCreateStats(ruleID, clientID)
	atomic.AddInt64(&stats.BytesOut, bytes)
	atomic.AddInt64(&stats.TotalBytesOut, bytes) // 累积值用于带宽计算
}

// IncrementConn 增加活跃连接数
func (r *TrafficRepository) IncrementConn(ruleID, clientID string) {
	stats := r.getOrCreateStats(ruleID, clientID)
	atomic.AddInt32(&stats.ActiveConns, 1)
}

// DecrementConn 减少活跃连接数
func (r *TrafficRepository) DecrementConn(ruleID, clientID string) {
	stats := r.getOrCreateStats(ruleID, clientID)
	atomic.AddInt32(&stats.ActiveConns, -1)
}

// FlushToDatabase 将内存统计刷新到数据库 (只刷新流量，连接数保留在内存)
func (r *TrafficRepository) FlushToDatabase() error {
	r.mu.Lock()
	statsToFlush := make([]*RealtimeTraffic, 0, len(r.realtimeStats))
	for _, stats := range r.realtimeStats {
		// 复制数据 (TotalConns 不重置，保留在内存中)
		statsCopy := &RealtimeTraffic{
			RuleID:      stats.RuleID,
			ClientID:    stats.ClientID,
			BytesIn:     atomic.SwapInt64(&stats.BytesIn, 0),
			BytesOut:    atomic.SwapInt64(&stats.BytesOut, 0),
			ActiveConns: atomic.LoadInt32(&stats.ActiveConns),
		}
		if statsCopy.BytesIn > 0 || statsCopy.BytesOut > 0 {
			statsToFlush = append(statsToFlush, statsCopy)
		}
	}
	r.mu.Unlock()

	if len(statsToFlush) == 0 {
		return nil
	}

	now := time.Now()
	for _, stats := range statsToFlush {
		// 只刷新流量数据，连接数只保存在内存中
		if stats.BytesIn == 0 && stats.BytesOut == 0 {
			continue
		}

		// 查找或创建今天的统计记录
		var existing model.TrafficStats
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

		err := r.db.Where("rule_id = ? AND client_id = ? AND period_start = ?",
			stats.RuleID, stats.ClientID, today).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			// 创建新记录 (不包含 TotalConnections)
			newStats := model.TrafficStats{
				ID:          uuid.New().String(),
				RuleID:      stats.RuleID,
				ClientID:    stats.ClientID,
				BytesIn:     stats.BytesIn,
				BytesOut:    stats.BytesOut,
				TotalBytes:  stats.BytesIn + stats.BytesOut,
				ActiveConns: int(stats.ActiveConns),
				PeriodStart: today,
				PeriodEnd:   today.Add(24 * time.Hour),
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := r.db.Create(&newStats).Error; err != nil {
				return err
			}
		} else if err == nil {
			// 更新现有记录 (不更新 TotalConnections)
			updates := map[string]interface{}{
				"bytes_in":     gorm.Expr("bytes_in + ?", stats.BytesIn),
				"bytes_out":    gorm.Expr("bytes_out + ?", stats.BytesOut),
				"total_bytes":  gorm.Expr("total_bytes + ?", stats.BytesIn+stats.BytesOut),
				"active_conns": stats.ActiveConns,
				"updated_at":   now,
			}
			if err := r.db.Model(&existing).Updates(updates).Error; err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

// GetSummaryByRule 获取按规则汇总的流量统计
func (r *TrafficRepository) GetSummaryByRule() ([]model.TrafficSummary, error) {
	var results []struct {
		RuleID     string
		RuleName   string
		ClientID   string
		ClientName string
		BytesIn    int64
		BytesOut   int64
		TotalBytes int64
	}

	err := r.db.Table("traffic_stats").
		Select(`
			traffic_stats.rule_id,
			COALESCE(forward_rules.name, '') as rule_name,
			traffic_stats.client_id,
			COALESCE(mb_clients.name, '') as client_name,
			SUM(traffic_stats.bytes_in) as bytes_in,
			SUM(traffic_stats.bytes_out) as bytes_out,
			SUM(traffic_stats.total_bytes) as total_bytes
		`).
		Joins("LEFT JOIN forward_rules ON traffic_stats.rule_id = forward_rules.id").
		Joins("LEFT JOIN mb_clients ON traffic_stats.client_id = mb_clients.id").
		Group("traffic_stats.rule_id, traffic_stats.client_id, forward_rules.name, mb_clients.name").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	// 获取实时连接数 (从内存)
	r.mu.RLock()
	defer r.mu.RUnlock()

	summaries := make([]model.TrafficSummary, 0, len(results))
	for _, res := range results {
		summary := model.TrafficSummary{
			RuleID:        res.RuleID,
			RuleName:      res.RuleName,
			ClientID:      res.ClientID,
			ClientName:    res.ClientName,
			BytesIn:       res.BytesIn,
			BytesOut:      res.BytesOut,
			TotalBytes:    res.TotalBytes,
			BytesInStr:    model.FormatBytes(res.BytesIn),
			BytesOutStr:   model.FormatBytes(res.BytesOut),
			TotalBytesStr: model.FormatBytes(res.TotalBytes),
		}

		// 从内存获取实时活跃连接数
		key := res.RuleID + ":" + res.ClientID
		if stats, ok := r.realtimeStats[key]; ok {
			summary.ActiveConns = int(atomic.LoadInt32(&stats.ActiveConns))
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// GetTodayStats 获取今日流量统计
func (r *TrafficRepository) GetTodayStats() ([]model.TrafficStats, error) {
	var stats []model.TrafficStats
	today := time.Now().Truncate(24 * time.Hour)

	err := r.db.Where("period_start >= ?", today).Find(&stats).Error
	return stats, err
}

// GetTotalStats 获取总流量统计和实时活跃连接数
func (r *TrafficRepository) GetTotalStats() (bytesIn, bytesOut int64, activeConns int, err error) {
	var result struct {
		BytesIn  int64
		BytesOut int64
	}

	err = r.db.Table("traffic_stats").
		Select("SUM(bytes_in) as bytes_in, SUM(bytes_out) as bytes_out").
		Scan(&result).Error

	if err != nil {
		return 0, 0, 0, err
	}

	// 从内存获取实时活跃连接数
	activeConns = r.GetRealtimeActiveConns()

	return result.BytesIn, result.BytesOut, activeConns, nil
}

// GetRealtimeActiveConns 获取实时活跃连接总数
func (r *TrafficRepository) GetRealtimeActiveConns() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var total int32
	for _, stats := range r.realtimeStats {
		total += atomic.LoadInt32(&stats.ActiveConns)
	}
	return int(total)
}

// ClientBandwidth 客户端带宽统计
type ClientBandwidth struct {
	ClientID     string
	ClientName   string
	BytesIn      int64
	BytesOut     int64
	BandwidthIn  int64 // bytes per second
	BandwidthOut int64 // bytes per second
}

// UpdateBandwidth 更新带宽计算 (应该每秒调用一次)
func (r *TrafficRepository) UpdateBandwidth() {
	r.mu.RLock()
	// 按客户端汇总当前流量（使用不会被重置的累积值）
	currentIn := make(map[string]int64)
	currentOut := make(map[string]int64)
	for _, stats := range r.realtimeStats {
		currentIn[stats.ClientID] += atomic.LoadInt64(&stats.TotalBytesIn)
		currentOut[stats.ClientID] += atomic.LoadInt64(&stats.TotalBytesOut)
	}
	r.mu.RUnlock()

	r.bwMu.Lock()
	defer r.bwMu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastUpdateTime).Seconds()
	if elapsed < 0.5 {
		return // 避免频繁更新
	}

	// 计算带宽
	for clientID, bytesIn := range currentIn {
		if lastIn, ok := r.lastBytesIn[clientID]; ok && elapsed > 0 {
			r.bandwidthIn[clientID] = int64(float64(bytesIn-lastIn) / elapsed)
		}
		r.lastBytesIn[clientID] = bytesIn
	}

	for clientID, bytesOut := range currentOut {
		if lastOut, ok := r.lastBytesOut[clientID]; ok && elapsed > 0 {
			r.bandwidthOut[clientID] = int64(float64(bytesOut-lastOut) / elapsed)
		}
		r.lastBytesOut[clientID] = bytesOut
	}

	r.lastUpdateTime = now
}

// GetClientBandwidth 获取所有客户端的带宽统计
func (r *TrafficRepository) GetClientBandwidth() []ClientBandwidth {
	r.bwMu.RLock()
	defer r.bwMu.RUnlock()

	// 收集所有客户端 ID
	clientIDs := make(map[string]bool)
	for clientID := range r.lastBytesIn {
		clientIDs[clientID] = true
	}
	for clientID := range r.lastBytesOut {
		clientIDs[clientID] = true
	}

	result := make([]ClientBandwidth, 0, len(clientIDs))
	for clientID := range clientIDs {
		bw := ClientBandwidth{
			ClientID:     clientID,
			BytesIn:      r.lastBytesIn[clientID],
			BytesOut:     r.lastBytesOut[clientID],
			BandwidthIn:  r.bandwidthIn[clientID],
			BandwidthOut: r.bandwidthOut[clientID],
		}
		result = append(result, bw)
	}

	return result
}
