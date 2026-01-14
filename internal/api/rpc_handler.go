package api

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/dollarkillerx/MessageBoy/internal/middleware"
	"github.com/dollarkillerx/MessageBoy/pkg/common/resp"
)

type RpcMethod interface {
	Name() string
	Execute(ctx context.Context, params json.RawMessage) (interface{}, error)
	RequireAuth() bool
}

type RpcHandler struct {
	methods    map[string]RpcMethod
	mu         sync.RWMutex
	jwtManager *middleware.JWTManager
}

func NewRpcHandler(jwtManager *middleware.JWTManager) *RpcHandler {
	return &RpcHandler{
		methods:    make(map[string]RpcMethod),
		jwtManager: jwtManager,
	}
}

func (h *RpcHandler) Register(method RpcMethod) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.methods[method.Name()] = method
}

func (h *RpcHandler) Handle(c *gin.Context) {
	var request resp.RpcRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resp.ErrorResponse(c, "", resp.ErrCodeParseError, "invalid JSON")
		return
	}

	if request.JsonRPC != resp.JSONRPCVersion {
		resp.ErrorResponse(c, request.ID, resp.ErrCodeInvalidRequest, "invalid JSON-RPC version")
		return
	}

	if request.Method == "" {
		resp.ErrorResponse(c, request.ID, resp.ErrCodeInvalidRequest, "method is required")
		return
	}

	h.mu.RLock()
	method, ok := h.methods[request.Method]
	h.mu.RUnlock()

	if !ok {
		resp.ErrorResponse(c, request.ID, resp.ErrCodeMethodNotFound, "method not found: "+request.Method)
		return
	}

	// 检查认证
	if method.RequireAuth() {
		if !h.isAuthenticated(c) {
			resp.ErrorResponse(c, request.ID, resp.ErrCodeAuthRequired, "authentication required")
			return
		}
	}

	// 创建带有 gin.Context 的 context
	ctx := context.WithValue(c.Request.Context(), ginContextKey, c)

	result, err := method.Execute(ctx, request.Params)
	if err != nil {
		resp.ErrorResponse(c, request.ID, resp.ErrCodeServerError, err.Error())
		return
	}

	resp.SuccessResponse(c, request.ID, result)
}

func (h *RpcHandler) isAuthenticated(c *gin.Context) bool {
	authHeader := c.GetHeader(middleware.AuthHeaderKey)
	if authHeader == "" {
		return false
	}

	tokenString := authHeader
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		tokenString = authHeader[7:]
	}

	claims, err := h.jwtManager.ValidateToken(tokenString)
	if err != nil {
		return false
	}

	c.Set(middleware.ContextUser, claims.Username)
	return true
}

type contextKey string

const ginContextKey contextKey = "gin_context"

func GetGinContext(ctx context.Context) *gin.Context {
	if c, ok := ctx.Value(ginContextKey).(*gin.Context); ok {
		return c
	}
	return nil
}
