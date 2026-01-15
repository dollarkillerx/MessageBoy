package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"

	"github.com/dollarkillerx/MessageBoy/internal/middleware"
	"github.com/dollarkillerx/MessageBoy/internal/storage"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type WebSSHMessage struct {
	Type string `json:"type"` // input, resize, ping
	Data string `json:"data"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

type WebSSHHandler struct {
	storage    *storage.Storage
	jwtManager *middleware.JWTManager
}

func NewWebSSHHandler(s *storage.Storage, jwtManager *middleware.JWTManager) *WebSSHHandler {
	return &WebSSHHandler{
		storage:    s,
		jwtManager: jwtManager,
	}
}

func (h *WebSSHHandler) Handle(c *gin.Context) {
	clientID := c.Param("clientId")
	token := c.Query("token")

	// 验证 JWT
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token required"})
		return
	}

	_, err := h.jwtManager.ValidateToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	// 获取客户端信息
	client, err := h.storage.Client.GetByID(clientID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	if client.SSHHost == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client has no SSH configuration"})
		return
	}

	// 升级到 WebSocket
	wsConn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upgrade websocket")
		return
	}
	defer wsConn.Close()

	// 建立 SSH 连接
	sshConfig := &ssh.ClientConfig{
		User:            client.SSHUser,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// 优先使用密码认证
	if client.SSHPassword != "" {
		sshConfig.Auth = []ssh.AuthMethod{
			ssh.Password(client.SSHPassword),
		}
	}

	sshAddr := fmt.Sprintf("%s:%d", client.SSHHost, client.SSHPort)
	sshConn, err := ssh.Dial("tcp", sshAddr, sshConfig)
	if err != nil {
		h.sendError(wsConn, fmt.Sprintf("SSH 连接失败: %v", err))
		return
	}
	defer sshConn.Close()

	// 创建 SSH 会话
	session, err := sshConn.NewSession()
	if err != nil {
		h.sendError(wsConn, fmt.Sprintf("创建 SSH 会话失败: %v", err))
		return
	}
	defer session.Close()

	// 请求 PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		h.sendError(wsConn, fmt.Sprintf("请求 PTY 失败: %v", err))
		return
	}

	// 获取 stdin/stdout
	stdin, err := session.StdinPipe()
	if err != nil {
		h.sendError(wsConn, fmt.Sprintf("获取 stdin 失败: %v", err))
		return
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		h.sendError(wsConn, fmt.Sprintf("获取 stdout 失败: %v", err))
		return
	}

	// 启动 shell
	if err := session.Shell(); err != nil {
		h.sendError(wsConn, fmt.Sprintf("启动 shell 失败: %v", err))
		return
	}

	var wg sync.WaitGroup
	done := make(chan struct{})

	// 从 SSH stdout 读取数据发送到 WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			select {
			case <-done:
				return
			default:
				n, err := stdout.Read(buf)
				if err != nil {
					return
				}
				if n > 0 {
					h.sendOutput(wsConn, string(buf[:n]))
				}
			}
		}
	}()

	// 从 WebSocket 读取数据发送到 SSH stdin
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)
		for {
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				return
			}

			var msg WebSSHMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}

			switch msg.Type {
			case "input":
				stdin.Write([]byte(msg.Data))
			case "resize":
				if msg.Cols > 0 && msg.Rows > 0 {
					session.WindowChange(msg.Rows, msg.Cols)
				}
			case "ping":
				h.sendPong(wsConn)
			}
		}
	}()

	// 等待会话结束
	session.Wait()
	wg.Wait()
}

func (h *WebSSHHandler) sendOutput(conn *websocket.Conn, data string) {
	msg := map[string]string{
		"type": "output",
		"data": data,
	}
	jsonData, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, jsonData)
}

func (h *WebSSHHandler) sendError(conn *websocket.Conn, data string) {
	msg := map[string]string{
		"type": "error",
		"data": data,
	}
	jsonData, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, jsonData)
}

func (h *WebSSHHandler) sendPong(conn *websocket.Conn) {
	msg := map[string]string{
		"type": "pong",
	}
	jsonData, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, jsonData)
}
