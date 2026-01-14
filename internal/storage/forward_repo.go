package storage

import (
	"github.com/dollarkillerx/MessageBoy/pkg/model"
	"gorm.io/gorm"
)

type ForwardRepository struct {
	db *gorm.DB
}

func NewForwardRepository(db *gorm.DB) *ForwardRepository {
	return &ForwardRepository{db: db}
}

func (r *ForwardRepository) Create(rule *model.ForwardRule) error {
	return r.db.Create(rule).Error
}

func (r *ForwardRepository) GetByID(id string) (*model.ForwardRule, error) {
	var rule model.ForwardRule
	if err := r.db.Where("id = ?", id).First(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *ForwardRepository) Update(rule *model.ForwardRule) error {
	return r.db.Save(rule).Error
}

func (r *ForwardRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.ForwardRule{}).Error
}

type ForwardListParams struct {
	Page     int
	Limit    int
	ClientID string
	Type     string
	Enabled  *bool
}

func (r *ForwardRepository) List(params ForwardListParams) ([]model.ForwardRule, int64, error) {
	var rules []model.ForwardRule
	var total int64

	query := r.db.Model(&model.ForwardRule{})

	if params.ClientID != "" {
		query = query.Where("listen_client = ?", params.ClientID)
	}

	if params.Type != "" {
		query = query.Where("type = ?", params.Type)
	}

	if params.Enabled != nil {
		query = query.Where("enabled = ?", *params.Enabled)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(params.Limit).Find(&rules).Error; err != nil {
		return nil, 0, err
	}

	return rules, total, nil
}

func (r *ForwardRepository) GetByClientID(clientID string) ([]model.ForwardRule, error) {
	var rules []model.ForwardRule
	if err := r.db.Where("listen_client = ? AND enabled = ?", clientID, true).Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *ForwardRepository) ToggleEnabled(id string, enabled bool) error {
	return r.db.Model(&model.ForwardRule{}).Where("id = ?", id).Update("enabled", enabled).Error
}
