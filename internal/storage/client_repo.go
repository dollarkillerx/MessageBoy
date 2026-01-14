package storage

import (
	"time"

	"github.com/dollarkillerx/MessageBoy/pkg/model"
	"gorm.io/gorm"
)

type ClientRepository struct {
	db *gorm.DB
}

func NewClientRepository(db *gorm.DB) *ClientRepository {
	return &ClientRepository{db: db}
}

func (r *ClientRepository) Create(client *model.Client) error {
	return r.db.Create(client).Error
}

func (r *ClientRepository) GetByID(id string) (*model.Client, error) {
	var client model.Client
	if err := r.db.Where("id = ?", id).First(&client).Error; err != nil {
		return nil, err
	}
	return &client, nil
}

func (r *ClientRepository) GetByToken(token string) (*model.Client, error) {
	var client model.Client
	if err := r.db.Where("token = ?", token).First(&client).Error; err != nil {
		return nil, err
	}
	return &client, nil
}

func (r *ClientRepository) Update(client *model.Client) error {
	return r.db.Save(client).Error
}

func (r *ClientRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.Client{}).Error
}

type ClientListParams struct {
	Page   int
	Limit  int
	Search string
	Status string
}

func (r *ClientRepository) List(params ClientListParams) ([]model.Client, int64, error) {
	var clients []model.Client
	var total int64

	query := r.db.Model(&model.Client{})

	if params.Search != "" {
		search := "%" + params.Search + "%"
		query = query.Where("name ILIKE ? OR description ILIKE ?", search, search)
	}

	if params.Status != "" {
		query = query.Where("status = ?", params.Status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(params.Limit).Find(&clients).Error; err != nil {
		return nil, 0, err
	}

	return clients, total, nil
}

func (r *ClientRepository) UpdateStatus(id string, status model.ClientStatus, ip string) error {
	now := time.Now()
	return r.db.Model(&model.Client{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":    status,
		"last_ip":   ip,
		"last_seen": &now,
	}).Error
}

func (r *ClientRepository) UpdateToken(id string, token string) error {
	return r.db.Model(&model.Client{}).Where("id = ?", id).Update("token", token).Error
}
