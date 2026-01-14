package conf

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"Server"`
	Database  DatabaseConfig  `mapstructure:"Database"`
	JWT       JWTConfig       `mapstructure:"JWT"`
	Admin     AdminConfig     `mapstructure:"Admin"`
	WebSocket WebSocketConfig `mapstructure:"WebSocket"`
	Logging   LoggingConfig   `mapstructure:"Logging"`
}

type ServerConfig struct {
	Host        string `mapstructure:"Host"`
	Port        int    `mapstructure:"Port"`
	Debug       bool   `mapstructure:"Debug"`
	ExternalURL string `mapstructure:"ExternalURL"`
}

type DatabaseConfig struct {
	Host            string `mapstructure:"Host"`
	Port            int    `mapstructure:"Port"`
	User            string `mapstructure:"User"`
	Password        string `mapstructure:"Password"`
	DBName          string `mapstructure:"DBName"`
	SSLMode         string `mapstructure:"SSLMode"`
	MaxIdleConns    int    `mapstructure:"MaxIdleConns"`
	MaxOpenConns    int    `mapstructure:"MaxOpenConns"`
	ConnMaxLifetime int    `mapstructure:"ConnMaxLifetime"`
}

type JWTConfig struct {
	SecretKey   string `mapstructure:"SecretKey"`
	ExpireHours int    `mapstructure:"ExpireHours"`
	Issuer      string `mapstructure:"Issuer"`
}

type AdminConfig struct {
	Username string `mapstructure:"Username"`
	Password string `mapstructure:"Password"`
}

type WebSocketConfig struct {
	Endpoint         string `mapstructure:"Endpoint"`
	PingInterval     int    `mapstructure:"PingInterval"`
	PongTimeout      int    `mapstructure:"PongTimeout"`
	OfflineThreshold int    `mapstructure:"OfflineThreshold"`
}

type LoggingConfig struct {
	Level   string `mapstructure:"Level"`
	File    string `mapstructure:"File"`
	MaxSize int    `mapstructure:"MaxSize"`
	MaxAge  int    `mapstructure:"MaxAge"`
}

func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(configPath)
	v.SetConfigType("toml")

	// 设置默认值
	setDefaults(v)

	// 支持环境变量覆盖
	v.SetEnvPrefix("MB")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("Server.Host", "0.0.0.0")
	v.SetDefault("Server.Port", 8080)
	v.SetDefault("Server.Debug", false)
	v.SetDefault("Server.ExternalURL", "http://localhost:8080")

	// Database defaults
	v.SetDefault("Database.Host", "localhost")
	v.SetDefault("Database.Port", 5432)
	v.SetDefault("Database.User", "messageboy")
	v.SetDefault("Database.Password", "")
	v.SetDefault("Database.DBName", "messageboy")
	v.SetDefault("Database.SSLMode", "disable")
	v.SetDefault("Database.MaxIdleConns", 10)
	v.SetDefault("Database.MaxOpenConns", 100)
	v.SetDefault("Database.ConnMaxLifetime", 3600)

	// JWT defaults
	v.SetDefault("JWT.SecretKey", "change-me-in-production")
	v.SetDefault("JWT.ExpireHours", 24)
	v.SetDefault("JWT.Issuer", "messageboy")

	// Admin defaults
	v.SetDefault("Admin.Username", "admin")
	v.SetDefault("Admin.Password", "admin123")

	// WebSocket defaults
	v.SetDefault("WebSocket.Endpoint", "/ws")
	v.SetDefault("WebSocket.PingInterval", 30)
	v.SetDefault("WebSocket.PongTimeout", 60)
	v.SetDefault("WebSocket.OfflineThreshold", 90)

	// Logging defaults
	v.SetDefault("Logging.Level", "info")
	v.SetDefault("Logging.File", "")
	v.SetDefault("Logging.MaxSize", 100)
	v.SetDefault("Logging.MaxAge", 30)
}

func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}
