package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/dollarkillerx/MessageBoy/internal/api"
	"github.com/dollarkillerx/MessageBoy/internal/conf"
	"github.com/dollarkillerx/MessageBoy/internal/proxy"
	"github.com/dollarkillerx/MessageBoy/internal/storage"
)

func main() {
	configPath := flag.String("config", "configs/server.toml", "path to config file")
	flag.Parse()

	// 加载配置
	cfg, err := conf.LoadConfig(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	// 初始化日志
	initLogger(cfg)

	log.Info().Msg("Starting MessageBoy Server Manager")

	// 初始化数据库
	store, err := storage.NewStorage(&cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize storage")
	}
	defer store.Close()

	// 创建 API 服务器
	server := api.NewApiServer(cfg, store)

	// 创建健康检查器
	healthChecker := proxy.NewHealthChecker(store, server.GetWSServer())
	healthChecker.Start()
	defer healthChecker.Stop()

	// 创建负载均衡器并注入到 API 服务器
	loadBalancer := proxy.NewLoadBalancer(store)
	server.SetLoadBalancer(loadBalancer)

	// 设置流量统计器
	server.GetWSServer().SetTrafficCounter(store.Traffic)

	// 启动定期流量统计刷新 (每分钟)
	trafficFlushStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := store.Traffic.FlushToDatabase(); err != nil {
					log.Error().Err(err).Msg("Failed to flush traffic stats")
				}
			case <-trafficFlushStop:
				// 最后一次刷新
				store.Traffic.FlushToDatabase()
				return
			}
		}
	}()

	// 优雅关闭
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info().Msg("Shutting down...")
		close(trafficFlushStop)
		healthChecker.Stop()
		os.Exit(0)
	}()

	// 启动服务器
	if err := server.Run(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

func initLogger(cfg *conf.Config) {
	level, err := zerolog.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	if cfg.Server.Debug {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}
