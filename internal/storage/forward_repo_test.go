package storage

import (
	"testing"

	"github.com/dollarkillerx/MessageBoy/pkg/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupForwardTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	err = db.AutoMigrate(
		&model.ForwardRule{},
		&model.Client{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func TestForwardRepository_Create(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	rule := &model.ForwardRule{
		ID:           "rule-1",
		Name:         "Test Rule",
		Type:         model.ForwardTypeDirect,
		ListenAddr:   "0.0.0.0:8080",
		ListenClient: "client-1",
		TargetAddr:   "127.0.0.1:80",
		Enabled:      true,
	}

	err := repo.Create(rule)
	if err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}

	// 验证创建成功
	var count int64
	db.Model(&model.ForwardRule{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 record, got %d", count)
	}
}

func TestForwardRepository_GetByID(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	// 创建测试数据
	rule := &model.ForwardRule{
		ID:           "rule-get",
		Name:         "Get Test",
		Type:         model.ForwardTypeDirect,
		ListenAddr:   "0.0.0.0:9090",
		ListenClient: "client-1",
		TargetAddr:   "127.0.0.1:90",
	}
	db.Create(rule)

	// 测试获取
	result, err := repo.GetByID("rule-get")
	if err != nil {
		t.Fatalf("failed to get rule: %v", err)
	}

	if result.Name != "Get Test" {
		t.Errorf("expected name 'Get Test', got '%s'", result.Name)
	}
}

func TestForwardRepository_GetByID_NotFound(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	_, err := repo.GetByID("non-existent")
	if err == nil {
		t.Error("expected error for non-existent ID")
	}
}

func TestForwardRepository_Update(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	// 创建测试数据
	rule := &model.ForwardRule{
		ID:           "rule-update",
		Name:         "Before Update",
		Type:         model.ForwardTypeDirect,
		ListenAddr:   "0.0.0.0:8080",
		ListenClient: "client-1",
		TargetAddr:   "127.0.0.1:80",
	}
	db.Create(rule)

	// 更新
	rule.Name = "After Update"
	err := repo.Update(rule)
	if err != nil {
		t.Fatalf("failed to update rule: %v", err)
	}

	// 验证更新
	var updated model.ForwardRule
	db.First(&updated, "id = ?", "rule-update")
	if updated.Name != "After Update" {
		t.Errorf("expected name 'After Update', got '%s'", updated.Name)
	}
}

func TestForwardRepository_Delete(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	// 创建测试数据
	rule := &model.ForwardRule{
		ID:           "rule-delete",
		Name:         "To Delete",
		Type:         model.ForwardTypeDirect,
		ListenAddr:   "0.0.0.0:8080",
		ListenClient: "client-1",
		TargetAddr:   "127.0.0.1:80",
	}
	db.Create(rule)

	// 删除
	err := repo.Delete("rule-delete")
	if err != nil {
		t.Fatalf("failed to delete rule: %v", err)
	}

	// 验证删除
	var count int64
	db.Model(&model.ForwardRule{}).Where("id = ?", "rule-delete").Count(&count)
	if count != 0 {
		t.Errorf("expected 0 records after delete, got %d", count)
	}
}

func TestForwardRepository_List(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	// 创建测试数据
	rules := []*model.ForwardRule{
		{ID: "rule-1", Name: "Rule 1", Type: model.ForwardTypeDirect, ListenClient: "client-1", Enabled: true},
		{ID: "rule-2", Name: "Rule 2", Type: model.ForwardTypeRelay, ListenClient: "client-1", Enabled: false},
		{ID: "rule-3", Name: "Rule 3", Type: model.ForwardTypeDirect, ListenClient: "client-2", Enabled: true},
	}
	for _, r := range rules {
		db.Create(r)
	}

	// 测试无过滤
	result, total, err := repo.List(ForwardListParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 results, got %d", len(result))
	}

	// 测试按 ClientID 过滤
	result, total, err = repo.List(ForwardListParams{Page: 1, Limit: 10, ClientID: "client-1"})
	if err != nil {
		t.Fatalf("failed to list by client: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2 for client-1, got %d", total)
	}

	// 测试按 Type 过滤
	result, total, err = repo.List(ForwardListParams{Page: 1, Limit: 10, Type: "direct"})
	if err != nil {
		t.Fatalf("failed to list by type: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2 for direct type, got %d", total)
	}

	// 测试按 Enabled 过滤
	enabled := true
	result, total, err = repo.List(ForwardListParams{Page: 1, Limit: 10, Enabled: &enabled})
	if err != nil {
		t.Fatalf("failed to list by enabled: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2 enabled, got %d", total)
	}
}

func TestForwardRepository_GetByClientID(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	// 创建测试数据
	rules := []*model.ForwardRule{
		{ID: "rule-1", Name: "Rule 1", ListenClient: "client-1", Enabled: true},
		{ID: "rule-2", Name: "Rule 2", ListenClient: "client-1", Enabled: false},
		{ID: "rule-3", Name: "Rule 3", ListenClient: "client-2", Enabled: true},
	}
	for _, r := range rules {
		db.Create(r)
	}

	// GetByClientID 只返回 enabled 的规则
	result, err := repo.GetByClientID("client-1")
	if err != nil {
		t.Fatalf("failed to get by client: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 enabled rule for client-1, got %d", len(result))
	}
}

func TestForwardRepository_ToggleEnabled(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	// 创建测试数据
	rule := &model.ForwardRule{
		ID:      "rule-toggle",
		Name:    "Toggle Test",
		Enabled: true,
	}
	db.Create(rule)

	// 禁用
	err := repo.ToggleEnabled("rule-toggle", false)
	if err != nil {
		t.Fatalf("failed to toggle: %v", err)
	}

	var updated model.ForwardRule
	db.First(&updated, "id = ?", "rule-toggle")
	if updated.Enabled != false {
		t.Error("expected Enabled to be false")
	}

	// 启用
	err = repo.ToggleEnabled("rule-toggle", true)
	if err != nil {
		t.Fatalf("failed to toggle back: %v", err)
	}

	db.First(&updated, "id = ?", "rule-toggle")
	if updated.Enabled != true {
		t.Error("expected Enabled to be true")
	}
}

func TestForwardRepository_UpdateStatus(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	// 创建测试数据
	rule := &model.ForwardRule{
		ID:     "rule-status",
		Name:   "Status Test",
		Status: model.RuleStatusPending,
	}
	db.Create(rule)

	// 更新为 running
	err := repo.UpdateStatus("rule-status", model.RuleStatusRunning, "")
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	var updated model.ForwardRule
	db.First(&updated, "id = ?", "rule-status")
	if updated.Status != model.RuleStatusRunning {
		t.Errorf("expected status 'running', got '%s'", updated.Status)
	}
	if updated.LastError != "" {
		t.Errorf("expected empty LastError, got '%s'", updated.LastError)
	}
}

func TestForwardRepository_UpdateStatus_WithError(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	// 创建测试数据
	rule := &model.ForwardRule{
		ID:     "rule-error",
		Name:   "Error Test",
		Status: model.RuleStatusPending,
	}
	db.Create(rule)

	// 更新为 error 状态并设置错误信息
	err := repo.UpdateStatus("rule-error", model.RuleStatusError, "address already in use")
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	var updated model.ForwardRule
	db.First(&updated, "id = ?", "rule-error")
	if updated.Status != model.RuleStatusError {
		t.Errorf("expected status 'error', got '%s'", updated.Status)
	}
	if updated.LastError != "address already in use" {
		t.Errorf("expected LastError 'address already in use', got '%s'", updated.LastError)
	}
}

func TestForwardRepository_UpdateStatus_ClearError(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	// 创建带有错误的测试数据
	rule := &model.ForwardRule{
		ID:        "rule-clear",
		Name:      "Clear Error Test",
		Status:    model.RuleStatusError,
		LastError: "previous error",
	}
	db.Create(rule)

	// 更新为 running 并清除错误
	err := repo.UpdateStatus("rule-clear", model.RuleStatusRunning, "")
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	var updated model.ForwardRule
	db.First(&updated, "id = ?", "rule-clear")
	if updated.Status != model.RuleStatusRunning {
		t.Errorf("expected status 'running', got '%s'", updated.Status)
	}
	if updated.LastError != "" {
		t.Errorf("expected empty LastError, got '%s'", updated.LastError)
	}
}

func TestForwardRepository_ResetStatusByClientID(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	// 创建测试数据
	rules := []*model.ForwardRule{
		{ID: "rule-1", Name: "Rule 1", ListenClient: "client-reset", Status: model.RuleStatusRunning, LastError: ""},
		{ID: "rule-2", Name: "Rule 2", ListenClient: "client-reset", Status: model.RuleStatusError, LastError: "some error"},
		{ID: "rule-3", Name: "Rule 3", ListenClient: "client-other", Status: model.RuleStatusRunning},
	}
	for _, r := range rules {
		db.Create(r)
	}

	// 重置 client-reset 的所有规则
	err := repo.ResetStatusByClientID("client-reset")
	if err != nil {
		t.Fatalf("failed to reset status: %v", err)
	}

	// 验证 client-reset 的规则已重置
	var rule1, rule2 model.ForwardRule
	db.First(&rule1, "id = ?", "rule-1")
	db.First(&rule2, "id = ?", "rule-2")

	if rule1.Status != model.RuleStatusPending {
		t.Errorf("expected rule-1 status 'pending', got '%s'", rule1.Status)
	}
	if rule2.Status != model.RuleStatusPending {
		t.Errorf("expected rule-2 status 'pending', got '%s'", rule2.Status)
	}
	if rule2.LastError != "" {
		t.Errorf("expected rule-2 LastError to be cleared, got '%s'", rule2.LastError)
	}

	// 验证其他 client 的规则未受影响
	var rule3 model.ForwardRule
	db.First(&rule3, "id = ?", "rule-3")
	if rule3.Status != model.RuleStatusRunning {
		t.Errorf("expected rule-3 status 'running' (unchanged), got '%s'", rule3.Status)
	}
}

func TestForwardRepository_List_Pagination(t *testing.T) {
	db := setupForwardTestDB(t)
	repo := NewForwardRepository(db)

	// 创建多个测试数据
	for i := 1; i <= 15; i++ {
		rule := &model.ForwardRule{
			ID:   "rule-" + string(rune('a'+i-1)),
			Name: "Rule " + string(rune('A'+i-1)),
		}
		db.Create(rule)
	}

	// 测试分页
	result, total, err := repo.List(ForwardListParams{Page: 1, Limit: 5})
	if err != nil {
		t.Fatalf("failed to list page 1: %v", err)
	}
	if total != 15 {
		t.Errorf("expected total 15, got %d", total)
	}
	if len(result) != 5 {
		t.Errorf("expected 5 results on page 1, got %d", len(result))
	}

	result, _, err = repo.List(ForwardListParams{Page: 2, Limit: 5})
	if err != nil {
		t.Fatalf("failed to list page 2: %v", err)
	}
	if len(result) != 5 {
		t.Errorf("expected 5 results on page 2, got %d", len(result))
	}

	result, _, err = repo.List(ForwardListParams{Page: 3, Limit: 5})
	if err != nil {
		t.Fatalf("failed to list page 3: %v", err)
	}
	if len(result) != 5 {
		t.Errorf("expected 5 results on page 3, got %d", len(result))
	}

	result, _, err = repo.List(ForwardListParams{Page: 4, Limit: 5})
	if err != nil {
		t.Fatalf("failed to list page 4: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results on page 4, got %d", len(result))
	}
}
