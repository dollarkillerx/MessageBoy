package client

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type ClientConfig struct {
	Client     ClientSection     `mapstructure:"Client"`
	Connection ConnectionSection `mapstructure:"Connection"`
	Logging    LoggingSection    `mapstructure:"Logging"`
	Forwarder  ForwarderSection  `mapstructure:"Forwarder"`
}

type ClientSection struct {
	ServerURL string `mapstructure:"ServerURL"`
	Token     string `mapstructure:"Token"`
}

type ConnectionSection struct {
	ReconnectInterval    int `mapstructure:"ReconnectInterval"`
	MaxReconnectInterval int `mapstructure:"MaxReconnectInterval"`
	HeartbeatInterval    int `mapstructure:"HeartbeatInterval"`
}

type LoggingSection struct {
	Level string `mapstructure:"Level"`
	File  string `mapstructure:"File"`
}

type ForwarderSection struct {
	BufferSize     int `mapstructure:"BufferSize"`
	ConnectTimeout int `mapstructure:"ConnectTimeout"`
	IdleTimeout    int `mapstructure:"IdleTimeout"`
}

func LoadClientConfig(configPath string) (*ClientConfig, error) {
	v := viper.New()

	v.SetConfigFile(configPath)
	v.SetConfigType("toml")

	setClientDefaults(v)

	v.SetEnvPrefix("MBC")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg ClientConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setClientDefaults(v *viper.Viper) {
	v.SetDefault("Client.ServerURL", "http://localhost:8080")
	v.SetDefault("Client.Token", "")

	v.SetDefault("Connection.ReconnectInterval", 5)
	v.SetDefault("Connection.MaxReconnectInterval", 60)
	v.SetDefault("Connection.HeartbeatInterval", 30)

	v.SetDefault("Logging.Level", "info")
	v.SetDefault("Logging.File", "")

	v.SetDefault("Forwarder.BufferSize", 32768)
	v.SetDefault("Forwarder.ConnectTimeout", 10)
	v.SetDefault("Forwarder.IdleTimeout", 300)
}
