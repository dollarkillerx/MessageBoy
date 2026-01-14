package client

import (
	"sync"
	"sync/atomic"
)

// TrafficCounter 客户端流量统计器
type TrafficCounter struct {
	stats map[string]*RuleTraffic
	mu    sync.RWMutex
}

// RuleTraffic 单条规则的流量统计
type RuleTraffic struct {
	RuleID      string
	BytesIn     int64
	BytesOut    int64
	Connections int64
	ActiveConns int32
}

// TrafficReport 流量上报数据
type TrafficReport struct {
	RuleID      string `json:"rule_id"`
	BytesIn     int64  `json:"bytes_in"`
	BytesOut    int64  `json:"bytes_out"`
	Connections int64  `json:"connections"`
}

func NewTrafficCounter() *TrafficCounter {
	return &TrafficCounter{
		stats: make(map[string]*RuleTraffic),
	}
}

func (tc *TrafficCounter) getOrCreate(ruleID string) *RuleTraffic {
	tc.mu.RLock()
	stat, ok := tc.stats[ruleID]
	tc.mu.RUnlock()

	if ok {
		return stat
	}

	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Double check
	if stat, ok = tc.stats[ruleID]; ok {
		return stat
	}

	stat = &RuleTraffic{RuleID: ruleID}
	tc.stats[ruleID] = stat
	return stat
}

// AddBytesIn 增加入站流量
func (tc *TrafficCounter) AddBytesIn(ruleID string, bytes int64) {
	stat := tc.getOrCreate(ruleID)
	atomic.AddInt64(&stat.BytesIn, bytes)
}

// AddBytesOut 增加出站流量
func (tc *TrafficCounter) AddBytesOut(ruleID string, bytes int64) {
	stat := tc.getOrCreate(ruleID)
	atomic.AddInt64(&stat.BytesOut, bytes)
}

// IncrementConn 增加连接数
func (tc *TrafficCounter) IncrementConn(ruleID string) {
	stat := tc.getOrCreate(ruleID)
	atomic.AddInt64(&stat.Connections, 1)
	atomic.AddInt32(&stat.ActiveConns, 1)
}

// DecrementConn 减少活跃连接数
func (tc *TrafficCounter) DecrementConn(ruleID string) {
	stat := tc.getOrCreate(ruleID)
	atomic.AddInt32(&stat.ActiveConns, -1)
}

// GetAndReset 获取并重置流量统计 (用于上报)
func (tc *TrafficCounter) GetAndReset() []TrafficReport {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	reports := make([]TrafficReport, 0, len(tc.stats))
	for ruleID, stat := range tc.stats {
		bytesIn := atomic.SwapInt64(&stat.BytesIn, 0)
		bytesOut := atomic.SwapInt64(&stat.BytesOut, 0)
		conns := atomic.SwapInt64(&stat.Connections, 0)

		// 只上报有流量的规则
		if bytesIn > 0 || bytesOut > 0 || conns > 0 {
			reports = append(reports, TrafficReport{
				RuleID:      ruleID,
				BytesIn:     bytesIn,
				BytesOut:    bytesOut,
				Connections: conns,
			})
		}
	}

	return reports
}

// CountingReader 带计数功能的 Reader
type CountingReader struct {
	reader  interface{ Read([]byte) (int, error) }
	counter *TrafficCounter
	ruleID  string
	isIn    bool // true: bytes_in, false: bytes_out
}

func NewCountingReader(r interface{ Read([]byte) (int, error) }, counter *TrafficCounter, ruleID string, isIn bool) *CountingReader {
	return &CountingReader{
		reader:  r,
		counter: counter,
		ruleID:  ruleID,
		isIn:    isIn,
	}
}

func (cr *CountingReader) Read(p []byte) (int, error) {
	n, err := cr.reader.Read(p)
	if n > 0 {
		if cr.isIn {
			cr.counter.AddBytesIn(cr.ruleID, int64(n))
		} else {
			cr.counter.AddBytesOut(cr.ruleID, int64(n))
		}
	}
	return n, err
}

// CountingWriter 带计数功能的 Writer
type CountingWriter struct {
	writer  interface{ Write([]byte) (int, error) }
	counter *TrafficCounter
	ruleID  string
	isIn    bool
}

func NewCountingWriter(w interface{ Write([]byte) (int, error) }, counter *TrafficCounter, ruleID string, isIn bool) *CountingWriter {
	return &CountingWriter{
		writer:  w,
		counter: counter,
		ruleID:  ruleID,
		isIn:    isIn,
	}
}

func (cw *CountingWriter) Write(p []byte) (int, error) {
	n, err := cw.writer.Write(p)
	if n > 0 {
		if cw.isIn {
			cw.counter.AddBytesIn(cw.ruleID, int64(n))
		} else {
			cw.counter.AddBytesOut(cw.ruleID, int64(n))
		}
	}
	return n, err
}
