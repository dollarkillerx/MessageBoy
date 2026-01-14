package relay

import (
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/dollarkillerx/MessageBoy/pkg/common/crypto"
)

type WSClientConn struct {
	endpoint  string
	clientID  string
	secretKey string
	crypto    *crypto.AESCrypto

	conn      *websocket.Conn
	sendCh    chan []byte
	recvCh    chan *TunnelMessage
	closeCh   chan struct{}
	closed    bool
	mu        sync.Mutex

	streams   *StreamManager
	reconnect bool
}

func NewWSClientConn(endpoint, clientID, secretKey string) (*WSClientConn, error) {
	aes, err := crypto.NewAESCryptoFromHex(secretKey)
	if err != nil {
		return nil, err
	}

	return &WSClientConn{
		endpoint:  endpoint,
		clientID:  clientID,
		secretKey: secretKey,
		crypto:    aes,
		sendCh:    make(chan []byte, 256),
		recvCh:    make(chan *TunnelMessage, 256),
		closeCh:   make(chan struct{}),
		streams:   NewStreamManager(),
		reconnect: true,
	}, nil
}

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

		msg, err := UnmarshalTunnelMessage(message)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to unmarshal tunnel message")
			continue
		}

		// 解密 payload
		if msg.Payload != nil && msg.Nonce != nil {
			decrypted, err := c.crypto.Decrypt(msg.Payload, msg.Nonce)
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

func (c *WSClientConn) writePump() {
	for {
		select {
		case message := <-c.sendCh:
			c.mu.Lock()
			if c.conn == nil || c.closed {
				c.mu.Unlock()
				return
			}
			err := c.conn.WriteMessage(websocket.BinaryMessage, message)
			c.mu.Unlock()

			if err != nil {
				log.Warn().Err(err).Msg("WebSocket write error")
				return
			}
		case <-c.closeCh:
			return
		}
	}
}

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

func (c *WSClientConn) Send(msg *TunnelMessage) error {
	// 加密 payload
	if msg.Payload != nil {
		ciphertext, nonce, err := c.crypto.Encrypt(msg.Payload)
		if err != nil {
			return err
		}
		msg.Payload = ciphertext
		msg.Nonce = nonce
	}

	data, err := msg.Marshal()
	if err != nil {
		return err
	}

	select {
	case c.sendCh <- data:
		return nil
	case <-c.closeCh:
		return nil
	}
}

func (c *WSClientConn) Recv() *TunnelMessage {
	select {
	case msg := <-c.recvCh:
		return msg
	case <-c.closeCh:
		return nil
	}
}

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

func (c *WSClientConn) GetStreams() *StreamManager {
	return c.streams
}
