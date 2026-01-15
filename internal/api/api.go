package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/dollarkillerx/MessageBoy/internal/conf"
	"github.com/dollarkillerx/MessageBoy/internal/middleware"
	"github.com/dollarkillerx/MessageBoy/internal/proxy"
	"github.com/dollarkillerx/MessageBoy/internal/relay"
	"github.com/dollarkillerx/MessageBoy/internal/storage"
)

type ApiServer struct {
	cfg          *conf.Config
	storage      *storage.Storage
	jwtManager   *middleware.JWTManager
	rpcHandler   *RpcHandler
	wsServer     *relay.WSServer
	loadBalancer *proxy.LoadBalancer
	webSSH       *WebSSHHandler
	engine       *gin.Engine
}

// GetWSServer 返回 WebSocket 服务器实例
func (s *ApiServer) GetWSServer() *relay.WSServer {
	return s.wsServer
}

// SetLoadBalancer 设置负载均衡器
func (s *ApiServer) SetLoadBalancer(lb *proxy.LoadBalancer) {
	s.loadBalancer = lb
	s.wsServer.SetLoadBalancer(lb)
}

// GetLoadBalancer 返回负载均衡器
func (s *ApiServer) GetLoadBalancer() *proxy.LoadBalancer {
	return s.loadBalancer
}

func NewApiServer(cfg *conf.Config, storage *storage.Storage) *ApiServer {
	if !cfg.Server.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	jwtManager := middleware.NewJWTManager(&cfg.JWT)
	rpcHandler := NewRpcHandler(jwtManager)
	wsServer := relay.NewWSServer()
	webSSH := NewWebSSHHandler(storage, jwtManager)

	server := &ApiServer{
		cfg:        cfg,
		storage:    storage,
		jwtManager: jwtManager,
		rpcHandler: rpcHandler,
		wsServer:   wsServer,
		webSSH:     webSSH,
		engine:     gin.New(),
	}

	server.setupMiddleware()
	server.setupRoutes()
	server.registerRpcMethods()

	return server
}

func (s *ApiServer) setupMiddleware() {
	s.engine.Use(gin.Recovery())
	s.engine.Use(corsMiddleware())
	s.engine.Use(loggerMiddleware())
}

func (s *ApiServer) setupRoutes() {
	s.engine.GET("/health", s.healthCheck)
	s.engine.POST("/api/rpc", s.rpcHandler.Handle)
	s.engine.GET(s.cfg.WebSocket.Endpoint, s.handleWebSocket)
	s.engine.GET("/api/ws/ssh/:clientId", s.webSSH.Handle)
}

func (s *ApiServer) handleWebSocket(c *gin.Context) {
	s.wsServer.HandleConnection(c.Writer, c.Request)
}

func (s *ApiServer) registerRpcMethods() {
	// 基础方法
	s.rpcHandler.Register(&PingMethod{})
	s.rpcHandler.Register(NewAdminLoginMethod(&s.cfg.Admin, s.jwtManager))

	// Client 管理方法
	s.rpcHandler.Register(NewCreateClientMethod(s.storage, s.cfg))
	s.rpcHandler.Register(NewGetClientListMethod(s.storage))
	s.rpcHandler.Register(NewGetClientMethod(s.storage))
	s.rpcHandler.Register(NewUpdateClientMethod(s.storage))
	s.rpcHandler.Register(NewDeleteClientMethod(s.storage))
	s.rpcHandler.Register(NewRegenerateClientTokenMethod(s.storage, s.cfg))
	s.rpcHandler.Register(NewGetClientInstallCommandMethod(s.storage, s.cfg))

	// Client 内部方法
	s.rpcHandler.Register(NewClientRegisterMethod(s.storage, s.cfg))
	s.rpcHandler.Register(NewClientHeartbeatMethod(s.storage))
	s.rpcHandler.Register(NewClientGetRulesMethod(s.storage))
	s.rpcHandler.Register(NewClientReportTrafficMethod(s.storage))

	// 转发规则管理方法 (传入 wsServer 用于规则变更通知)
	s.rpcHandler.Register(NewCreateForwardRuleMethod(s.storage, s.wsServer))
	s.rpcHandler.Register(NewGetForwardRuleListMethod(s.storage))
	s.rpcHandler.Register(NewGetForwardRuleMethod(s.storage))
	s.rpcHandler.Register(NewUpdateForwardRuleMethod(s.storage, s.wsServer))
	s.rpcHandler.Register(NewDeleteForwardRuleMethod(s.storage, s.wsServer))
	s.rpcHandler.Register(NewToggleForwardRuleMethod(s.storage, s.wsServer))

	// 代理组管理方法
	s.rpcHandler.Register(NewCreateProxyGroupMethod(s.storage))
	s.rpcHandler.Register(NewGetProxyGroupListMethod(s.storage))
	s.rpcHandler.Register(NewGetProxyGroupMethod(s.storage))
	s.rpcHandler.Register(NewUpdateProxyGroupMethod(s.storage))
	s.rpcHandler.Register(NewDeleteProxyGroupMethod(s.storage))
	s.rpcHandler.Register(NewAddProxyGroupNodeMethod(s.storage))
	s.rpcHandler.Register(NewRemoveProxyGroupNodeMethod(s.storage))
	s.rpcHandler.Register(NewUpdateProxyGroupNodeMethod(s.storage))

	// 流量统计方法
	s.rpcHandler.Register(NewGetTrafficSummaryMethod(s.storage))
	s.rpcHandler.Register(NewGetTotalTrafficMethod(s.storage))
	s.rpcHandler.Register(NewGetTodayTrafficMethod(s.storage))
}

// GetStorage 返回存储实例 (用于设置流量统计器)
func (s *ApiServer) GetStorage() *storage.Storage {
	return s.storage
}

func (s *ApiServer) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"version": Version,
	})
}

func (s *ApiServer) Run() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	log.Info().Str("addr", addr).Msg("Starting API server")
	return s.engine.Run(addr)
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func loggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := c.Request.URL.Path
		c.Next()
		log.Debug().
			Str("method", c.Request.Method).
			Str("path", start).
			Int("status", c.Writer.Status()).
			Msg("HTTP request")
	}
}
