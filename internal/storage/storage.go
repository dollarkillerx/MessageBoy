package storage

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/dollarkillerx/MessageBoy/internal/conf"
	"github.com/dollarkillerx/MessageBoy/pkg/model"
)

type Storage struct {
	DB         *gorm.DB
	Client     *ClientRepository
	Forward    *ForwardRepository
	ProxyGroup *ProxyGroupRepository
}

func NewStorage(cfg *conf.DatabaseConfig) (*Storage, error) {
	logLevel := logger.Silent
	if cfg.Host != "" {
		logLevel = logger.Info
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	// 自动迁移
	if err := db.AutoMigrate(
		&model.Client{},
		&model.ForwardRule{},
		&model.ProxyGroup{},
		&model.ProxyGroupNode{},
	); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	log.Info().Msg("Database connected and migrated successfully")

	return &Storage{
		DB:         db,
		Client:     NewClientRepository(db),
		Forward:    NewForwardRepository(db),
		ProxyGroup: NewProxyGroupRepository(db),
	}, nil
}

func (s *Storage) Close() error {
	sqlDB, err := s.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
