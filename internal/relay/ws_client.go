package relay

import (
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/dollarkillerx/MessageBoy/pkg/common/crypto"
)

const (
	// AES-GCM Nonce 大小
	nonceSize = 12
	// 中继数据加密共享密钥 (32字节 = 64位十六进制)
	// 所有客户端使用相同密钥，以便中继场景下互相解密
	relaySharedKey = "a]T#k9$mP2vL8nQ5wX1cZ7bY4dF6gH3j"
)

// WSClientConn WebSocket 客户端连接
type WSClientConn struct {
	endpoint  string
	clientID  string
	secretKey string

	conn    *websocket.Conn
	sendCh  chan *sendItem
	recvCh  chan *TunnelMessage
	closeCh chan struct{}
	closed  bool
	mu      sync.Mutex

	streams   *StreamManager
	reconnect bool
}

// 全局共享加密器 (用于中继数据加密)
var sharedCrypto *crypto.AESCrypto

func init() {
	var err error
	sharedCrypto, err = crypto.NewAESCrypto([]byte(relaySharedKey))
	if err != nil {
		panic("failed to init shared crypto: " + err.Error())
	}
}

// sendItem 发送队列项
type sendItem struct {
	buf  *[]byte // 来自 pool
	size int
}

// NewWSClientConn 创建 WebSocket 客户端连接
func NewWSClientConn(endpoint, clientID, secretKey string) (*WSClientConn, error) {
	return &WSClientConn{
		endpoint:  endpoint,
		clientID:  clientID,
		secretKey: secretKey,
		sendCh:    make(chan *sendItem, 512),
		recvCh:    make(chan *TunnelMessage, 512),
		closeCh:   make(chan struct{}),
		streams:   NewStreamManager(),
		reconnect: true,
	}, nil
}

// Connect 连接到 WebSocket 服务器
func (c *WSClientConn) Connect() error {
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return err
	}

	// 转换 http -> ws, https -> wss
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}

	q := u.Query()
	q.Set("client_id", c.clientID)
	u.RawQuery = q.Encode()

	log.Info().Str("url", u.String()).Msg("Connecting to WebSocket server")

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.closed = false
	c.mu.Unlock()

	go c.readPump()
	go c.writePump()

	log.Info().Msg("WebSocket connected")
	return nil
}

// readPump 读取消息循环
func (c *WSClientConn) readPump() {
	defer c.handleDisconnect()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Warn().Err(err).Msg("WebSocket read error")
			}
			return
		}

		msg, err := UnmarshalBinary(message)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to unmarshal tunnel message")
			continue
		}

		// 使用共享密钥解密 Data 类型的 payload
		if len(msg.Payload) > nonceSize && msg.Type == MsgTypeData {
			nonce := msg.Payload[:nonceSize]
			ciphertext := msg.Payload[nonceSize:]
			decrypted, err := sharedCrypto.Decrypt(ciphertext, nonce)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to decrypt payload")
				continue
			}
			msg.Payload = decrypted
		}

		select {
		case c.recvCh <- msg:
		case <-c.closeCh:
			return
		}
	}
}

// writePump 发送消息循环
func (c *WSClientConn) writePump() {
	for {
		select {
		case item := <-c.sendCh:
			c.mu.Lock()
			if c.conn == nil || c.closed {
				c.mu.Unlock()
				PutBuffer(item.buf)
				return
			}
			err := c.conn.WriteMessage(websocket.BinaryMessage, (*item.buf)[:item.size])
			c.mu.Unlock()

			// 归还 buffer
			PutBuffer(item.buf)

			if err != nil {
				log.Warn().Err(err).Msg("WebSocket write error")
				return
			}
		case <-c.closeCh:
			return
		}
	}
}

// handleDisconnect 处理断开连接
func (c *WSClientConn) handleDisconnect() {
	c.mu.Lock()
	wasConnected := c.conn != nil
	c.conn = nil
	c.mu.Unlock()

	if wasConnected && c.reconnect {
		log.Info().Msg("WebSocket disconnected, attempting reconnect...")
		go c.reconnectLoop()
	}
}

// reconnectLoop 重连循环
func (c *WSClientConn) reconnectLoop() {
	backoff := time.Second

	for {
		select {
		case <-c.closeCh:
			return
		case <-time.After(backoff):
		}

		if err := c.Connect(); err != nil {
			log.Warn().Err(err).Dur("backoff", backoff).Msg("Reconnect failed")
			backoff *= 2
			if backoff > 60*time.Second {
				backoff = 60 * time.Second
			}
		} else {
			return
		}
	}
}

// Send 发送消息（使用 buffer pool，零拷贝优化）
func (c *WSClientConn) Send(msg *TunnelMessage) error {
	buf := GetBuffer()

	// 使用共享密钥加密 Data 类型的 payload
	if msg.Type == MsgTypeData && len(msg.Payload) > 0 {
		ciphertext, nonce, err := sharedCrypto.Encrypt(msg.Payload)
		if err != nil {
			PutBuffer(buf)
			return err
		}
		// 格式: Nonce(12B) + CipherText
		encryptedPayload := make([]byte, nonceSize+len(ciphertext))
		copy(encryptedPayload[:nonceSize], nonce)
		copy(encryptedPayload[nonceSize:], ciphertext)
		msg.Payload = encryptedPayload
	}

	n, err := msg.MarshalTo(*buf)
	if err != nil {
		PutBuffer(buf)
		return err
	}

	select {
	case c.sendCh <- &sendItem{buf: buf, size: n}:
		return nil
	case <-c.closeCh:
		PutBuffer(buf)
		return nil
	default:
		// channel 满了，尝试非阻塞发送
		select {
		case c.sendCh <- &sendItem{buf: buf, size: n}:
			return nil
		case <-c.closeCh:
			PutBuffer(buf)
			return nil
		}
	}
}

// SendRaw 发送原始数据（用于已经序列化好的数据）
func (c *WSClientConn) SendRaw(data []byte) error {
	buf := GetBuffer()
	if len(data) > len(*buf) {
		PutBuffer(buf)
		return ErrBufferTooSmall
	}
	n := copy(*buf, data)

	select {
	case c.sendCh <- &sendItem{buf: buf, size: n}:
		return nil
	case <-c.closeCh:
		PutBuffer(buf)
		return nil
	}
}

// Recv 接收消息
func (c *WSClientConn) Recv() *TunnelMessage {
	select {
	case msg := <-c.recvCh:
		return msg
	case <-c.closeCh:
		return nil
	}
}

// Close 关闭连接
func (c *WSClientConn) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.closed {
		c.closed = true
		c.reconnect = false
		close(c.closeCh)
		if c.conn != nil {
			c.conn.Close()
		}
		c.streams.CloseAll()
	}
}

// GetStreams 获取流管理器
func (c *WSClientConn) GetStreams() *StreamManager {
	return c.streams
}

// IsConnected 检查是否已连接
func (c *WSClientConn) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil && !c.closed
}
