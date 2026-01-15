package storage

import (
	"time"

	"gorm.io/gorm"

	"github.com/dollarkillerx/MessageBoy/pkg/model"
)

type ProxyGroupRepository struct {
	db *gorm.DB
}

func NewProxyGroupRepository(db *gorm.DB) *ProxyGroupRepository {
	return &ProxyGroupRepository{db: db}
}

// Group CRUD

func (r *ProxyGroupRepository) Create(group *model.ProxyGroup) error {
	return r.db.Create(group).Error
}

func (r *ProxyGroupRepository) GetByID(id string) (*model.ProxyGroup, error) {
	var group model.ProxyGroup
	err := r.db.First(&group, "id = ?", id).Error
	return &group, err
}

func (r *ProxyGroupRepository) GetByName(name string) (*model.ProxyGroup, error) {
	var group model.ProxyGroup
	err := r.db.First(&group, "name = ?", name).Error
	return &group, err
}

type ProxyGroupListParams struct {
	Page   int
	Limit  int
	Search string
}

func (r *ProxyGroupRepository) List(params ProxyGroupListParams) ([]model.ProxyGroup, int64, error) {
	var groups []model.ProxyGroup
	var total int64

	query := r.db.Model(&model.ProxyGroup{})

	if params.Search != "" {
		query = query.Where("name ILIKE ? OR description ILIKE ?",
			"%"+params.Search+"%", "%"+params.Search+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	if err := query.Offset(offset).Limit(params.Limit).Order("created_at DESC").Find(&groups).Error; err != nil {
		return nil, 0, err
	}

	return groups, total, nil
}

func (r *ProxyGroupRepository) Update(group *model.ProxyGroup) error {
	return r.db.Save(group).Error
}

func (r *ProxyGroupRepository) Delete(id string) error {
	// 先删除组内的所有节点
	if err := r.db.Where("group_id = ?", id).Delete(&model.ProxyGroupNode{}).Error; err != nil {
		return err
	}
	return r.db.Delete(&model.ProxyGroup{}, "id = ?", id).Error
}

// Node CRUD

func (r *ProxyGroupRepository) AddNode(node *model.ProxyGroupNode) error {
	return r.db.Create(node).Error
}

func (r *ProxyGroupRepository) GetNode(id string) (*model.ProxyGroupNode, error) {
	var node model.ProxyGroupNode
	if err := r.db.First(&node, "id = ?", id).Error; err != nil {
		return nil, err
	}
	// 手动加载 Client
	if node.ClientID != "" {
		var client model.Client
		if err := r.db.First(&client, "id = ?", node.ClientID).Error; err == nil {
			node.Client = &client
		}
	}
	return &node, nil
}

func (r *ProxyGroupRepository) GetNodesByGroupID(groupID string) ([]model.ProxyGroupNode, error) {
	var nodes []model.ProxyGroupNode
	if err := r.db.Where("group_id = ?", groupID).Order("priority ASC, created_at ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	// 手动加载 Clients
	r.loadClientsForNodes(nodes)
	return nodes, nil
}

func (r *ProxyGroupRepository) GetHealthyNodesByGroupID(groupID string) ([]model.ProxyGroupNode, error) {
	var nodes []model.ProxyGroupNode
	if err := r.db.Where("group_id = ? AND status = ?", groupID, model.NodeStatusHealthy).
		Order("priority ASC, active_conns ASC").
		Find(&nodes).Error; err != nil {
		return nil, err
	}
	// 手动加载 Clients
	r.loadClientsForNodes(nodes)
	return nodes, nil
}

// loadClientsForNodes 批量加载节点关联的 Client
func (r *ProxyGroupRepository) loadClientsForNodes(nodes []model.ProxyGroupNode) {
	if len(nodes) == 0 {
		return
	}
	// 收集所有 ClientID
	clientIDs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.ClientID != "" {
			clientIDs = append(clientIDs, n.ClientID)
		}
	}
	if len(clientIDs) == 0 {
		return
	}
	// 批量查询 Clients
	var clients []model.Client
	r.db.Where("id IN ?", clientIDs).Find(&clients)
	// 建立 ID -> Client 映射
	clientMap := make(map[string]*model.Client)
	for i := range clients {
		clientMap[clients[i].ID] = &clients[i]
	}
	// 填充到 nodes
	for i := range nodes {
		if c, ok := clientMap[nodes[i].ClientID]; ok {
			nodes[i].Client = c
		}
	}
}

func (r *ProxyGroupRepository) UpdateNode(node *model.ProxyGroupNode) error {
	return r.db.Save(node).Error
}

func (r *ProxyGroupRepository) RemoveNode(id string) error {
	return r.db.Delete(&model.ProxyGroupNode{}, "id = ?", id).Error
}

func (r *ProxyGroupRepository) RemoveNodeByClientID(groupID, clientID string) error {
	return r.db.Where("group_id = ? AND client_id = ?", groupID, clientID).Delete(&model.ProxyGroupNode{}).Error
}

// 健康检查相关

func (r *ProxyGroupRepository) UpdateNodeHealth(nodeID string, healthy bool) error {
	now := time.Now()
	updates := map[string]interface{}{
		"last_check_at": now,
		"last_check_ok": healthy,
		"updated_at":    now,
	}

	if healthy {
		updates["status"] = model.NodeStatusHealthy
		updates["fail_count"] = 0
	} else {
		updates["fail_count"] = gorm.Expr("fail_count + 1")
	}

	return r.db.Model(&model.ProxyGroupNode{}).Where("id = ?", nodeID).Updates(updates).Error
}

func (r *ProxyGroupRepository) MarkNodeUnhealthy(nodeID string) error {
	return r.db.Model(&model.ProxyGroupNode{}).Where("id = ?", nodeID).
		Updates(map[string]interface{}{
			"status":     model.NodeStatusUnhealthy,
			"updated_at": time.Now(),
		}).Error
}

// 连接统计相关

func (r *ProxyGroupRepository) IncrementNodeConns(nodeID string) error {
	return r.db.Model(&model.ProxyGroupNode{}).Where("id = ?", nodeID).
		Updates(map[string]interface{}{
			"active_conns": gorm.Expr("active_conns + 1"),
			"updated_at":   time.Now(),
		}).Error
}

func (r *ProxyGroupRepository) DecrementNodeConns(nodeID string) error {
	return r.db.Model(&model.ProxyGroupNode{}).Where("id = ?", nodeID).
		Where("active_conns > 0").
		Updates(map[string]interface{}{
			"active_conns": gorm.Expr("active_conns - 1"),
			"updated_at":   time.Now(),
		}).Error
}

// GetGroupWithNodes 获取组及其所有节点
func (r *ProxyGroupRepository) GetGroupWithNodes(groupID string) (*model.ProxyGroup, []model.ProxyGroupNode, error) {
	group, err := r.GetByID(groupID)
	if err != nil {
		return nil, nil, err
	}

	nodes, err := r.GetNodesByGroupID(groupID)
	if err != nil {
		return nil, nil, err
	}

	return group, nodes, nil
}

// IsGroupReference 检查字符串是否是组引用 (格式: @group_name 或 @group_id)
func IsGroupReference(s string) bool {
	return len(s) > 1 && s[0] == '@'
}

// ParseGroupReference 解析组引用，返回组名或组ID
func ParseGroupReference(s string) string {
	if IsGroupReference(s) {
		return s[1:]
	}
	return s
}
