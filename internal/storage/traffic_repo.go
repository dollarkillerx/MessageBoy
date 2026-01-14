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
}

// RealtimeTraffic 实时流量统计 (内存中)
type RealtimeTraffic struct {
	RuleID      string
	ClientID    string
	BytesIn     int64
	BytesOut    int64
	ActiveConns int32
	TotalConns  int64
}

func NewTrafficRepository(db *gorm.DB) *TrafficRepository {
	return &TrafficRepository{
		db:            db,
		realtimeStats: make(map[string]*RealtimeTraffic),
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
}

// AddBytesOut 增加出站流量
func (r *TrafficRepository) AddBytesOut(ruleID, clientID string, bytes int64) {
	stats := r.getOrCreateStats(ruleID, clientID)
	atomic.AddInt64(&stats.BytesOut, bytes)
}

// IncrementConn 增加连接数
func (r *TrafficRepository) IncrementConn(ruleID, clientID string) {
	stats := r.getOrCreateStats(ruleID, clientID)
	atomic.AddInt32(&stats.ActiveConns, 1)
	atomic.AddInt64(&stats.TotalConns, 1)
}

// DecrementConn 减少活跃连接数
func (r *TrafficRepository) DecrementConn(ruleID, clientID string) {
	stats := r.getOrCreateStats(ruleID, clientID)
	atomic.AddInt32(&stats.ActiveConns, -1)
}

// AddConnections 添加连接数 (用于 Client 上报)
func (r *TrafficRepository) AddConnections(ruleID, clientID string, count int64) {
	stats := r.getOrCreateStats(ruleID, clientID)
	atomic.AddInt64(&stats.TotalConns, count)
}

// FlushToDatabase 将内存统计刷新到数据库
func (r *TrafficRepository) FlushToDatabase() error {
	r.mu.Lock()
	statsToFlush := make([]*RealtimeTraffic, 0, len(r.realtimeStats))
	for _, stats := range r.realtimeStats {
		// 复制数据
		statsCopy := &RealtimeTraffic{
			RuleID:      stats.RuleID,
			ClientID:    stats.ClientID,
			BytesIn:     atomic.SwapInt64(&stats.BytesIn, 0),
			BytesOut:    atomic.SwapInt64(&stats.BytesOut, 0),
			ActiveConns: atomic.LoadInt32(&stats.ActiveConns),
			TotalConns:  atomic.SwapInt64(&stats.TotalConns, 0),
		}
		if statsCopy.BytesIn > 0 || statsCopy.BytesOut > 0 || statsCopy.TotalConns > 0 {
			statsToFlush = append(statsToFlush, statsCopy)
		}
	}
	r.mu.Unlock()

	if len(statsToFlush) == 0 {
		return nil
	}

	now := time.Now()
	for _, stats := range statsToFlush {
		// 查找或创建今天的统计记录
		var existing model.TrafficStats
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

		err := r.db.Where("rule_id = ? AND client_id = ? AND period_start = ?",
			stats.RuleID, stats.ClientID, today).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			// 创建新记录
			newStats := model.TrafficStats{
				ID:               uuid.New().String(),
				RuleID:           stats.RuleID,
				ClientID:         stats.ClientID,
				BytesIn:          stats.BytesIn,
				BytesOut:         stats.BytesOut,
				TotalBytes:       stats.BytesIn + stats.BytesOut,
				ActiveConns:      int(stats.ActiveConns),
				TotalConnections: stats.TotalConns,
				PeriodStart:      today,
				PeriodEnd:        today.Add(24 * time.Hour),
				CreatedAt:        now,
				UpdatedAt:        now,
			}
			if err := r.db.Create(&newStats).Error; err != nil {
				return err
			}
		} else if err == nil {
			// 更新现有记录
			updates := map[string]interface{}{
				"bytes_in":          gorm.Expr("bytes_in + ?", stats.BytesIn),
				"bytes_out":         gorm.Expr("bytes_out + ?", stats.BytesOut),
				"total_bytes":       gorm.Expr("total_bytes + ?", stats.BytesIn+stats.BytesOut),
				"active_conns":      stats.ActiveConns,
				"total_connections": gorm.Expr("total_connections + ?", stats.TotalConns),
				"updated_at":        now,
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
		TotalConns int64
	}

	err := r.db.Table("traffic_stats").
		Select(`
			traffic_stats.rule_id,
			COALESCE(forward_rules.name, '') as rule_name,
			traffic_stats.client_id,
			COALESCE(clients.name, '') as client_name,
			SUM(traffic_stats.bytes_in) as bytes_in,
			SUM(traffic_stats.bytes_out) as bytes_out,
			SUM(traffic_stats.total_bytes) as total_bytes,
			SUM(traffic_stats.total_connections) as total_conns
		`).
		Joins("LEFT JOIN forward_rules ON traffic_stats.rule_id = forward_rules.id").
		Joins("LEFT JOIN clients ON traffic_stats.client_id = clients.id").
		Group("traffic_stats.rule_id, traffic_stats.client_id, forward_rules.name, clients.name").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	// 获取实时活跃连接数
	r.mu.RLock()
	defer r.mu.RUnlock()

	summaries := make([]model.TrafficSummary, 0, len(results))
	for _, res := range results {
		summary := model.TrafficSummary{
			RuleID:           res.RuleID,
			RuleName:         res.RuleName,
			ClientID:         res.ClientID,
			ClientName:       res.ClientName,
			BytesIn:          res.BytesIn,
			BytesOut:         res.BytesOut,
			TotalBytes:       res.TotalBytes,
			TotalConnections: res.TotalConns,
			BytesInStr:       model.FormatBytes(res.BytesIn),
			BytesOutStr:      model.FormatBytes(res.BytesOut),
			TotalBytesStr:    model.FormatBytes(res.TotalBytes),
		}

		// 添加实时活跃连接数
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

// GetTotalStats 获取总流量统计
func (r *TrafficRepository) GetTotalStats() (bytesIn, bytesOut, totalConns int64, err error) {
	var result struct {
		BytesIn    int64
		BytesOut   int64
		TotalConns int64
	}

	err = r.db.Table("traffic_stats").
		Select("SUM(bytes_in) as bytes_in, SUM(bytes_out) as bytes_out, SUM(total_connections) as total_conns").
		Scan(&result).Error

	return result.BytesIn, result.BytesOut, result.TotalConns, err
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
