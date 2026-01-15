package storage

import (
	"sync"
	"testing"

	"github.com/dollarkillerx/MessageBoy/pkg/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// 自动迁移
	err = db.AutoMigrate(
		&model.TrafficStats{},
		&model.ForwardRule{},
		&model.Client{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func TestTrafficRepository_AddBytes(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTrafficRepository(db)

	// 添加流量
	repo.AddBytesIn("rule1", "client1", 100)
	repo.AddBytesIn("rule1", "client1", 200)
	repo.AddBytesOut("rule1", "client1", 50)

	// 验证内存中的统计
	stats := repo.getOrCreateStats("rule1", "client1")
	if stats.BytesIn != 300 {
		t.Errorf("expected bytes_in 300, got %d", stats.BytesIn)
	}
	if stats.BytesOut != 50 {
		t.Errorf("expected bytes_out 50, got %d", stats.BytesOut)
	}
}

func TestTrafficRepository_Connections(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTrafficRepository(db)

	// 测试活跃连接计数
	repo.IncrementConn("rule1", "client1")
	repo.IncrementConn("rule1", "client1")
	repo.IncrementConn("rule1", "client1")
	repo.DecrementConn("rule1", "client1")

	stats := repo.getOrCreateStats("rule1", "client1")
	if stats.ActiveConns != 2 {
		t.Errorf("expected 2 active conns, got %d", stats.ActiveConns)
	}
}

func TestTrafficRepository_GetRealtimeActiveConns(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTrafficRepository(db)

	repo.IncrementConn("rule1", "client1")
	repo.IncrementConn("rule1", "client1")
	repo.IncrementConn("rule2", "client2")

	total := repo.GetRealtimeActiveConns()
	if total != 3 {
		t.Errorf("expected 3 active conns, got %d", total)
	}

	repo.DecrementConn("rule1", "client1")
	total = repo.GetRealtimeActiveConns()
	if total != 2 {
		t.Errorf("expected 2 active conns after decrement, got %d", total)
	}
}

func TestTrafficRepository_FlushToDatabase(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTrafficRepository(db)

	// 添加流量
	repo.AddBytesIn("rule1", "client1", 100)
	repo.AddBytesOut("rule1", "client1", 50)
	repo.IncrementConn("rule1", "client1")

	// 刷新到数据库
	err := repo.FlushToDatabase()
	if err != nil {
		t.Fatalf("failed to flush: %v", err)
	}

	// 验证数据库中的数据
	var stats model.TrafficStats
	err = db.Where("rule_id = ? AND client_id = ?", "rule1", "client1").First(&stats).Error
	if err != nil {
		t.Fatalf("failed to query stats: %v", err)
	}

	if stats.BytesIn != 100 {
		t.Errorf("expected bytes_in 100, got %d", stats.BytesIn)
	}
	if stats.BytesOut != 50 {
		t.Errorf("expected bytes_out 50, got %d", stats.BytesOut)
	}
	if stats.TotalBytes != 150 {
		t.Errorf("expected total_bytes 150, got %d", stats.TotalBytes)
	}
	// 验证内存中的活跃连接数
	memStats := repo.getOrCreateStats("rule1", "client1")
	if memStats.ActiveConns != 1 {
		t.Errorf("expected 1 active connection in memory, got %d", memStats.ActiveConns)
	}
}

func TestTrafficRepository_FlushToDatabase_Accumulate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTrafficRepository(db)

	// 第一次添加并刷新
	repo.AddBytesIn("rule1", "client1", 100)
	err := repo.FlushToDatabase()
	if err != nil {
		t.Fatalf("first flush failed: %v", err)
	}

	// 第二次添加并刷新
	repo.AddBytesIn("rule1", "client1", 200)
	err = repo.FlushToDatabase()
	if err != nil {
		t.Fatalf("second flush failed: %v", err)
	}

	// 验证数据已累加
	var stats model.TrafficStats
	err = db.Where("rule_id = ? AND client_id = ?", "rule1", "client1").First(&stats).Error
	if err != nil {
		t.Fatalf("failed to query stats: %v", err)
	}

	if stats.BytesIn != 300 {
		t.Errorf("expected bytes_in 300 (accumulated), got %d", stats.BytesIn)
	}
}

func TestTrafficRepository_GetTotalStats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTrafficRepository(db)

	// 添加多个规则的流量并刷新
	repo.AddBytesIn("rule1", "client1", 100)
	repo.AddBytesOut("rule1", "client1", 50)
	repo.IncrementConn("rule1", "client1")
	repo.IncrementConn("rule1", "client1")

	repo.AddBytesIn("rule2", "client2", 200)
	repo.AddBytesOut("rule2", "client2", 100)
	repo.IncrementConn("rule2", "client2")

	err := repo.FlushToDatabase()
	if err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	// 获取总计
	bytesIn, bytesOut, activeConns, err := repo.GetTotalStats()
	if err != nil {
		t.Fatalf("failed to get total stats: %v", err)
	}

	if bytesIn != 300 {
		t.Errorf("expected total bytes_in 300, got %d", bytesIn)
	}
	if bytesOut != 150 {
		t.Errorf("expected total bytes_out 150, got %d", bytesOut)
	}
	if activeConns != 3 {
		t.Errorf("expected 3 active connections, got %d", activeConns)
	}
}

func TestTrafficRepository_Concurrent(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTrafficRepository(db)

	var wg sync.WaitGroup

	// 并发添加流量
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			repo.AddBytesIn("rule1", "client1", 10)
			repo.AddBytesOut("rule1", "client1", 5)
			repo.IncrementConn("rule1", "client1")
		}()
	}

	wg.Wait()

	stats := repo.getOrCreateStats("rule1", "client1")
	if stats.BytesIn != 1000 {
		t.Errorf("expected bytes_in 1000, got %d", stats.BytesIn)
	}
	if stats.BytesOut != 500 {
		t.Errorf("expected bytes_out 500, got %d", stats.BytesOut)
	}
	if stats.ActiveConns != 100 {
		t.Errorf("expected 100 active conns, got %d", stats.ActiveConns)
	}
}

func TestTrafficRepository_GetSummaryByRule(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTrafficRepository(db)

	// 创建测试数据
	client := &model.Client{ID: "client1", Name: "Test Client"}
	db.Create(client)

	rule := &model.ForwardRule{ID: "rule1", Name: "Test Rule", ListenClient: "client1"}
	db.Create(rule)

	// 添加流量并刷新
	repo.AddBytesIn("rule1", "client1", 100)
	repo.AddBytesOut("rule1", "client1", 50)
	repo.IncrementConn("rule1", "client1")
	repo.IncrementConn("rule1", "client1")

	err := repo.FlushToDatabase()
	if err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	// 获取汇总
	summaries, err := repo.GetSummaryByRule()
	if err != nil {
		t.Fatalf("failed to get summary: %v", err)
	}

	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	summary := summaries[0]
	if summary.RuleName != "Test Rule" {
		t.Errorf("expected rule name 'Test Rule', got '%s'", summary.RuleName)
	}
	if summary.ClientName != "Test Client" {
		t.Errorf("expected client name 'Test Client', got '%s'", summary.ClientName)
	}
	if summary.BytesIn != 100 {
		t.Errorf("expected bytes_in 100, got %d", summary.BytesIn)
	}
	if summary.BytesOut != 50 {
		t.Errorf("expected bytes_out 50, got %d", summary.BytesOut)
	}
	if summary.ActiveConns != 2 {
		t.Errorf("expected 2 active conns, got %d", summary.ActiveConns)
	}
}

func TestTrafficRepository_FlushEmpty(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTrafficRepository(db)

	// 没有数据时刷新不应报错
	err := repo.FlushToDatabase()
	if err != nil {
		t.Errorf("flush with no data should not error: %v", err)
	}

	// 验证没有创建记录
	var count int64
	db.Model(&model.TrafficStats{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 records, got %d", count)
	}
}
