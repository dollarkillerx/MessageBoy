package resp

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

const JSONRPCVersion = "2.0"

// 错误码定义
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
	ErrCodeServerError    = -32000
	ErrCodeAuthRequired   = -32001
	ErrCodePermDenied     = -32002
	ErrCodeNotFound       = -32003
	ErrCodeConflict       = -32004
)

type RpcRequest struct {
	JsonRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      string          `json:"id"`
}

type RpcResponse struct {
	JsonRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RpcError       `json:"error,omitempty"`
}

type RpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func NewRpcError(code int, message string) *RpcError {
	return &RpcError{
		Code:    code,
		Message: message,
	}
}

func SuccessResponse(c *gin.Context, id string, result interface{}) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		ErrorResponse(c, id, ErrCodeInternalError, "failed to marshal result")
		return
	}

	c.JSON(http.StatusOK, RpcResponse{
		JsonRPC: JSONRPCVersion,
		ID:      id,
		Result:  resultJSON,
	})
}

func ErrorResponse(c *gin.Context, id string, code int, message string) {
	c.JSON(http.StatusOK, RpcResponse{
		JsonRPC: JSONRPCVersion,
		ID:      id,
		Error: &RpcError{
			Code:    code,
			Message: message,
		},
	})
}

func ErrorWithDataResponse(c *gin.Context, id string, code int, message string, data interface{}) {
	c.JSON(http.StatusOK, RpcResponse{
		JsonRPC: JSONRPCVersion,
		ID:      id,
		Error: &RpcError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	})
}
