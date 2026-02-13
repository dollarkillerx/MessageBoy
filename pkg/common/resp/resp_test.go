package resp

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNewRpcError(t *testing.T) {
	err := NewRpcError(ErrCodeInternalError, "something went wrong")
	if err.Code != ErrCodeInternalError {
		t.Errorf("expected code %d, got %d", ErrCodeInternalError, err.Code)
	}
	if err.Message != "something went wrong" {
		t.Errorf("expected message 'something went wrong', got %q", err.Message)
	}
}

func TestSuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	SuccessResponse(c, "req-1", map[string]string{"key": "val"})

	var resp RpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.JsonRPC != JSONRPCVersion {
		t.Errorf("expected jsonrpc %q, got %q", JSONRPCVersion, resp.JsonRPC)
	}
	if resp.ID != "req-1" {
		t.Errorf("expected id 'req-1', got %q", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("expected error to be nil, got %+v", resp.Error)
	}

	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if result["key"] != "val" {
		t.Errorf("expected result key=val, got %q", result["key"])
	}
}

func TestSuccessResponse_MarshalError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// channels cannot be marshaled to JSON
	SuccessResponse(c, "req-2", make(chan int))

	var resp RpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error response for unmarshalable input")
	}
	if resp.Error.Code != ErrCodeInternalError {
		t.Errorf("expected error code %d, got %d", ErrCodeInternalError, resp.Error.Code)
	}
}

func TestErrorResponse(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	ErrorResponse(c, "req-3", ErrCodeNotFound, "not found")

	var resp RpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error in response")
	}
	if resp.Error.Code != ErrCodeNotFound {
		t.Errorf("expected code %d, got %d", ErrCodeNotFound, resp.Error.Code)
	}
	if resp.Error.Message != "not found" {
		t.Errorf("expected message 'not found', got %q", resp.Error.Message)
	}
	if resp.Result != nil {
		t.Errorf("expected nil result, got %v", resp.Result)
	}
}

func TestErrorWithDataResponse(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	ErrorWithDataResponse(c, "req-4", ErrCodeInvalidParams, "bad params", map[string]string{"field": "name"})

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	var errObj struct {
		Code    int               `json:"code"`
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
	}
	if err := json.Unmarshal(raw["error"], &errObj); err != nil {
		t.Fatalf("failed to unmarshal error: %v", err)
	}

	if errObj.Code != ErrCodeInvalidParams {
		t.Errorf("expected code %d, got %d", ErrCodeInvalidParams, errObj.Code)
	}
	if errObj.Data["field"] != "name" {
		t.Errorf("expected data field=name, got %q", errObj.Data["field"])
	}
}
