package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/dollarkillerx/MessageBoy/internal/client"
)

func main() {
	configPath := flag.String("config", "configs/client.toml", "path to config file")
	serverURL := flag.String("server", "", "server URL (overrides config)")
	token := flag.String("token", "", "registration token (overrides config)")
	flag.Parse()

	// 加载配置
	cfg, err := client.LoadClientConfig(*configPath)
	if err != nil {
		// 如果配置文件不存在但提供了命令行参数，使用默认配置
		if *serverURL != "" && *token != "" {
			cfg = &client.ClientConfig{
				Client: client.ClientSection{
					ServerURL: *serverURL,
					Token:     *token,
				},
				Connection: client.ConnectionSection{
					ReconnectInterval:    5,
					MaxReconnectInterval: 60,
					HeartbeatInterval:    30,
				},
				Logging: client.LoggingSection{
					Level: "info",
				},
				Forwarder: client.ForwarderSection{
					BufferSize:     32768,
					ConnectTimeout: 10,
					IdleTimeout:    300,
				},
			}
		} else {
			log.Fatal().Err(err).Msg("Failed to load config")
		}
	}

	// 命令行参数覆盖配置
	if *serverURL != "" {
		cfg.Client.ServerURL = *serverURL
	}
	if *token != "" {
		cfg.Client.Token = *token
	}

	// 验证必要配置
	if cfg.Client.ServerURL == "" || cfg.Client.Token == "" {
		log.Fatal().Msg("Server URL and token are required")
	}

	// 初始化日志
	initLogger(cfg)

	log.Info().Msg("Starting MessageBoy Client")

	// 创建 client
	c := client.New(cfg)

	// 优雅关闭
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info().Msg("Shutting down...")
		c.Stop()
	}()

	// 运行
	if err := c.Run(); err != nil {
		log.Fatal().Err(err).Msg("Client error")
	}
}

func initLogger(cfg *client.ClientConfig) {
	level, err := zerolog.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}
