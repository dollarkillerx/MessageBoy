package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dollarkillerx/MessageBoy/internal/storage"
	"github.com/dollarkillerx/MessageBoy/pkg/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestStorage(t *testing.T) *storage.Storage {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// 自动迁移
	err = db.AutoMigrate(
		&model.Client{},
		&model.ForwardRule{},
		&model.ProxyGroup{},
		&model.ProxyGroupNode{},
		&model.TrafficStats{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	store := &storage.Storage{}
	store.Traffic = storage.NewTrafficRepository(db)

	return store
}

func TestClientReportTrafficMethod_Name(t *testing.T) {
	method := NewClientReportTrafficMethod(nil)
	if method.Name() != "clientReportTraffic" {
		t.Errorf("expected name 'clientReportTraffic', got '%s'", method.Name())
	}
}

func TestClientReportTrafficMethod_RequireAuth(t *testing.T) {
	method := NewClientReportTrafficMethod(nil)
	if method.RequireAuth() != false {
		t.Error("expected RequireAuth to return false")
	}
}

func TestClientReportTrafficMethod_Execute_Success(t *testing.T) {
	store := setupTestStorage(t)
	method := NewClientReportTrafficMethod(store)

	params := ClientReportTrafficParams{
		ClientID: "client1",
		Reports: []TrafficReportItem{
			{
				RuleID:      "rule1",
				BytesIn:     100,
				BytesOut:    50,
				Connections: 5,
			},
			{
				RuleID:      "rule2",
				BytesIn:     200,
				BytesOut:    100,
				Connections: 3,
			},
		},
	}

	paramsJSON, _ := json.Marshal(params)

	result, err := method.Execute(context.Background(), paramsJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["ack"] != true {
		t.Error("expected ack to be true")
	}

	// 验证流量已被记录到 repository
	// 刷新到数据库
	err = store.Traffic.FlushToDatabase()
	if err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	bytesIn, bytesOut, totalConns, err := store.Traffic.GetTotalStats()
	if err != nil {
		t.Fatalf("failed to get total stats: %v", err)
	}

	if bytesIn != 300 {
		t.Errorf("expected total bytes_in 300, got %d", bytesIn)
	}
	if bytesOut != 150 {
		t.Errorf("expected total bytes_out 150, got %d", bytesOut)
	}
	if totalConns != 8 {
		t.Errorf("expected 8 total connections, got %d", totalConns)
	}
}

func TestClientReportTrafficMethod_Execute_MissingClientID(t *testing.T) {
	store := setupTestStorage(t)
	method := NewClientReportTrafficMethod(store)

	params := ClientReportTrafficParams{
		ClientID: "",
		Reports:  []TrafficReportItem{},
	}

	paramsJSON, _ := json.Marshal(params)

	_, err := method.Execute(context.Background(), paramsJSON)
	if err == nil {
		t.Error("expected error for missing client_id")
	}
	if err.Error() != "client_id is required" {
		t.Errorf("expected 'client_id is required' error, got '%s'", err.Error())
	}
}

func TestClientReportTrafficMethod_Execute_InvalidParams(t *testing.T) {
	store := setupTestStorage(t)
	method := NewClientReportTrafficMethod(store)

	// 无效的 JSON
	_, err := method.Execute(context.Background(), []byte("invalid json"))
	if err == nil {
		t.Error("expected error for invalid params")
	}
	if err.Error() != "invalid params" {
		t.Errorf("expected 'invalid params' error, got '%s'", err.Error())
	}
}

func TestClientReportTrafficMethod_Execute_EmptyReports(t *testing.T) {
	store := setupTestStorage(t)
	method := NewClientReportTrafficMethod(store)

	params := ClientReportTrafficParams{
		ClientID: "client1",
		Reports:  []TrafficReportItem{},
	}

	paramsJSON, _ := json.Marshal(params)

	result, err := method.Execute(context.Background(), paramsJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["ack"] != true {
		t.Error("expected ack to be true even with empty reports")
	}
}

func TestClientReportTrafficMethod_Execute_ZeroValues(t *testing.T) {
	store := setupTestStorage(t)
	method := NewClientReportTrafficMethod(store)

	// 测试零值不会被添加
	params := ClientReportTrafficParams{
		ClientID: "client1",
		Reports: []TrafficReportItem{
			{
				RuleID:      "rule1",
				BytesIn:     0,
				BytesOut:    0,
				Connections: 0,
			},
		},
	}

	paramsJSON, _ := json.Marshal(params)

	result, err := method.Execute(context.Background(), paramsJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap := result.(map[string]interface{})
	if resultMap["ack"] != true {
		t.Error("expected ack to be true")
	}

	// 刷新并验证没有数据
	store.Traffic.FlushToDatabase()

	bytesIn, bytesOut, totalConns, _ := store.Traffic.GetTotalStats()
	if bytesIn != 0 || bytesOut != 0 || totalConns != 0 {
		t.Errorf("expected all zeros for zero-value reports, got in=%d, out=%d, conns=%d",
			bytesIn, bytesOut, totalConns)
	}
}

func TestClientReportTrafficMethod_Execute_MultipleReportsAccumulate(t *testing.T) {
	store := setupTestStorage(t)
	method := NewClientReportTrafficMethod(store)

	// 第一次上报
	params1 := ClientReportTrafficParams{
		ClientID: "client1",
		Reports: []TrafficReportItem{
			{RuleID: "rule1", BytesIn: 100, BytesOut: 50, Connections: 2},
		},
	}
	paramsJSON1, _ := json.Marshal(params1)
	method.Execute(context.Background(), paramsJSON1)

	// 第二次上报 (同一个 client)
	params2 := ClientReportTrafficParams{
		ClientID: "client1",
		Reports: []TrafficReportItem{
			{RuleID: "rule1", BytesIn: 200, BytesOut: 100, Connections: 3},
		},
	}
	paramsJSON2, _ := json.Marshal(params2)
	method.Execute(context.Background(), paramsJSON2)

	// 刷新并验证累加
	store.Traffic.FlushToDatabase()

	bytesIn, bytesOut, totalConns, _ := store.Traffic.GetTotalStats()
	if bytesIn != 300 {
		t.Errorf("expected accumulated bytes_in 300, got %d", bytesIn)
	}
	if bytesOut != 150 {
		t.Errorf("expected accumulated bytes_out 150, got %d", bytesOut)
	}
	if totalConns != 5 {
		t.Errorf("expected accumulated connections 5, got %d", totalConns)
	}
}

// ClientReportRuleStatus tests

func setupTestStorageWithForward(t *testing.T) *storage.Storage {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// 自动迁移
	err = db.AutoMigrate(
		&model.Client{},
		&model.ForwardRule{},
		&model.ProxyGroup{},
		&model.ProxyGroupNode{},
		&model.TrafficStats{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	store := &storage.Storage{}
	store.Forward = storage.NewForwardRepository(db)
	store.Traffic = storage.NewTrafficRepository(db)

	return store
}

func TestClientReportRuleStatusMethod_Name(t *testing.T) {
	method := NewClientReportRuleStatusMethod(nil)
	if method.Name() != "clientReportRuleStatus" {
		t.Errorf("expected name 'clientReportRuleStatus', got '%s'", method.Name())
	}
}

func TestClientReportRuleStatusMethod_RequireAuth(t *testing.T) {
	method := NewClientReportRuleStatusMethod(nil)
	if method.RequireAuth() != false {
		t.Error("expected RequireAuth to return false")
	}
}

func TestClientReportRuleStatusMethod_Execute_Success(t *testing.T) {
	store := setupTestStorageWithForward(t)
	method := NewClientReportRuleStatusMethod(store)

	// 先创建一个规则
	rule := &model.ForwardRule{
		ID:           "rule-test-1",
		Name:         "Test Rule",
		Type:         model.ForwardTypeDirect,
		ListenAddr:   "0.0.0.0:8080",
		ListenClient: "client-1",
		TargetAddr:   "127.0.0.1:80",
		Status:       model.RuleStatusPending,
	}
	if err := store.Forward.Create(rule); err != nil {
		t.Fatalf("failed to create test rule: %v", err)
	}

	params := ClientReportRuleStatusParams{
		ClientID: "client-1",
		Reports: []RuleStatusReportItem{
			{
				RuleID: "rule-test-1",
				Status: "running",
				Error:  "",
			},
		},
	}

	paramsJSON, _ := json.Marshal(params)

	result, err := method.Execute(context.Background(), paramsJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["ack"] != true {
		t.Error("expected ack to be true")
	}

	// 验证状态已更新
	updatedRule, err := store.Forward.GetByID("rule-test-1")
	if err != nil {
		t.Fatalf("failed to get updated rule: %v", err)
	}

	if updatedRule.Status != model.RuleStatusRunning {
		t.Errorf("expected status 'running', got '%s'", updatedRule.Status)
	}
}

func TestClientReportRuleStatusMethod_Execute_WithError(t *testing.T) {
	store := setupTestStorageWithForward(t)
	method := NewClientReportRuleStatusMethod(store)

	// 先创建一个规则
	rule := &model.ForwardRule{
		ID:           "rule-test-2",
		Name:         "Test Rule 2",
		Type:         model.ForwardTypeDirect,
		ListenAddr:   "0.0.0.0:9090",
		ListenClient: "client-1",
		TargetAddr:   "127.0.0.1:90",
		Status:       model.RuleStatusPending,
	}
	if err := store.Forward.Create(rule); err != nil {
		t.Fatalf("failed to create test rule: %v", err)
	}

	params := ClientReportRuleStatusParams{
		ClientID: "client-1",
		Reports: []RuleStatusReportItem{
			{
				RuleID: "rule-test-2",
				Status: "error",
				Error:  "address already in use",
			},
		},
	}

	paramsJSON, _ := json.Marshal(params)

	result, err := method.Execute(context.Background(), paramsJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap := result.(map[string]interface{})
	if resultMap["ack"] != true {
		t.Error("expected ack to be true")
	}

	// 验证状态和错误已更新
	updatedRule, err := store.Forward.GetByID("rule-test-2")
	if err != nil {
		t.Fatalf("failed to get updated rule: %v", err)
	}

	if updatedRule.Status != model.RuleStatusError {
		t.Errorf("expected status 'error', got '%s'", updatedRule.Status)
	}

	if updatedRule.LastError != "address already in use" {
		t.Errorf("expected LastError 'address already in use', got '%s'", updatedRule.LastError)
	}
}

func TestClientReportRuleStatusMethod_Execute_MissingClientID(t *testing.T) {
	store := setupTestStorageWithForward(t)
	method := NewClientReportRuleStatusMethod(store)

	params := ClientReportRuleStatusParams{
		ClientID: "",
		Reports:  []RuleStatusReportItem{},
	}

	paramsJSON, _ := json.Marshal(params)

	_, err := method.Execute(context.Background(), paramsJSON)
	if err == nil {
		t.Error("expected error for missing client_id")
	}
	if err.Error() != "client_id is required" {
		t.Errorf("expected 'client_id is required' error, got '%s'", err.Error())
	}
}

func TestClientReportRuleStatusMethod_Execute_InvalidParams(t *testing.T) {
	store := setupTestStorageWithForward(t)
	method := NewClientReportRuleStatusMethod(store)

	// 无效的 JSON
	_, err := method.Execute(context.Background(), []byte("invalid json"))
	if err == nil {
		t.Error("expected error for invalid params")
	}
	if err.Error() != "invalid params" {
		t.Errorf("expected 'invalid params' error, got '%s'", err.Error())
	}
}

func TestClientReportRuleStatusMethod_Execute_MultipleReports(t *testing.T) {
	store := setupTestStorageWithForward(t)
	method := NewClientReportRuleStatusMethod(store)

	// 创建多个规则
	rules := []*model.ForwardRule{
		{
			ID:           "rule-multi-1",
			Name:         "Rule 1",
			Type:         model.ForwardTypeDirect,
			ListenAddr:   "0.0.0.0:8001",
			ListenClient: "client-1",
			TargetAddr:   "127.0.0.1:81",
			Status:       model.RuleStatusPending,
		},
		{
			ID:           "rule-multi-2",
			Name:         "Rule 2",
			Type:         model.ForwardTypeDirect,
			ListenAddr:   "0.0.0.0:8002",
			ListenClient: "client-1",
			TargetAddr:   "127.0.0.1:82",
			Status:       model.RuleStatusPending,
		},
	}

	for _, r := range rules {
		if err := store.Forward.Create(r); err != nil {
			t.Fatalf("failed to create test rule: %v", err)
		}
	}

	params := ClientReportRuleStatusParams{
		ClientID: "client-1",
		Reports: []RuleStatusReportItem{
			{RuleID: "rule-multi-1", Status: "running"},
			{RuleID: "rule-multi-2", Status: "error", Error: "connection refused"},
		},
	}

	paramsJSON, _ := json.Marshal(params)

	result, err := method.Execute(context.Background(), paramsJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap := result.(map[string]interface{})
	if resultMap["ack"] != true {
		t.Error("expected ack to be true")
	}

	// 验证两个规则都已更新
	rule1, _ := store.Forward.GetByID("rule-multi-1")
	if rule1.Status != model.RuleStatusRunning {
		t.Errorf("expected rule1 status 'running', got '%s'", rule1.Status)
	}

	rule2, _ := store.Forward.GetByID("rule-multi-2")
	if rule2.Status != model.RuleStatusError {
		t.Errorf("expected rule2 status 'error', got '%s'", rule2.Status)
	}
	if rule2.LastError != "connection refused" {
		t.Errorf("expected rule2 error 'connection refused', got '%s'", rule2.LastError)
	}
}

func TestClientReportRuleStatusMethod_Execute_NonExistentRule(t *testing.T) {
	store := setupTestStorageWithForward(t)
	method := NewClientReportRuleStatusMethod(store)

	// 上报不存在的规则 - 应该忽略而不是报错
	params := ClientReportRuleStatusParams{
		ClientID: "client-1",
		Reports: []RuleStatusReportItem{
			{RuleID: "non-existent-rule", Status: "running"},
		},
	}

	paramsJSON, _ := json.Marshal(params)

	result, err := method.Execute(context.Background(), paramsJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap := result.(map[string]interface{})
	if resultMap["ack"] != true {
		t.Error("expected ack to be true even for non-existent rule")
	}
}
