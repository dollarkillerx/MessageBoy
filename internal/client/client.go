package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/dollarkillerx/MessageBoy/internal/relay"
)

type Client struct {
	cfg        *ClientConfig
	clientID   string
	secretKey  string
	wsEndpoint string

	wsConn         *relay.WSClientConn
	forwarders     map[string]ForwarderInterface
	mu             sync.RWMutex
	trafficCounter *TrafficCounter

	stopCh chan struct{}
}

// ForwarderInterface 转发器接口
type ForwarderInterface interface {
	Start() error
	Stop()
}

func New(cfg *ClientConfig) *Client {
	return &Client{
		cfg:            cfg,
		forwarders:     make(map[string]ForwarderInterface),
		trafficCounter: NewTrafficCounter(),
		stopCh:         make(chan struct{}),
	}
}

func (c *Client) Run() error {
	log.Info().Str("server", c.cfg.Client.ServerURL).Msg("Starting MessageBoy Client")

	// 注册到服务器
	if err := c.register(); err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}

	log.Info().Str("client_id", c.clientID).Msg("Registered successfully")

	// 建立 WebSocket 连接
	if err := c.connectWebSocket(); err != nil {
		log.Warn().Err(err).Msg("Failed to connect WebSocket, relay forwarding disabled")
	} else {
		// 启动隧道消息处理
		go c.handleTunnelMessages()
	}

	// 获取初始规则
	if err := c.fetchAndApplyRules(); err != nil {
		log.Warn().Err(err).Msg("Failed to fetch initial rules")
	}

	// 启动心跳
	go c.heartbeatLoop()

	// 启动流量上报
	go c.trafficReportLoop()

	// 等待停止信号
	<-c.stopCh
	return nil
}

func (c *Client) Stop() {
	close(c.stopCh)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, f := range c.forwarders {
		f.Stop()
	}
	if c.wsConn != nil {
		c.wsConn.Close()
	}
}

func (c *Client) register() error {
	hostname, _ := os.Hostname()

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "register",
		"method":  "clientRegister",
		"params": map[string]interface{}{
			"token":    c.cfg.Client.Token,
			"hostname": hostname,
			"version":  "2.0.0",
		},
	}

	resp, err := c.rpcCall(req)
	if err != nil {
		return err
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		if errObj, ok := resp["error"].(map[string]interface{}); ok {
			return fmt.Errorf("register failed: %v", errObj["message"])
		}
		return fmt.Errorf("invalid response")
	}

	c.clientID = result["client_id"].(string)
	c.secretKey = result["secret_key"].(string)
	c.wsEndpoint = result["ws_endpoint"].(string)

	return nil
}

func (c *Client) connectWebSocket() error {
	wsConn, err := relay.NewWSClientConn(c.wsEndpoint, c.clientID, c.secretKey)
	if err != nil {
		return err
	}

	if err := wsConn.Connect(); err != nil {
		return err
	}

	c.wsConn = wsConn
	log.Info().Str("endpoint", c.wsEndpoint).Msg("WebSocket connected")
	return nil
}

func (c *Client) handleTunnelMessages() {
	for {
		msg := c.wsConn.Recv()
		if msg == nil {
			return
		}

		switch msg.Type {
		case relay.MsgTypeConnect:
			// 作为出口节点，需要连接目标
			go c.handleIncomingConnect(msg)

		case relay.MsgTypeConnAck:
			// 连接确认，通知等待的 stream
			stream := c.wsConn.GetStreams().GetStream(msg.StreamID)
			if stream != nil {
				stream.Write([]byte{relay.MsgTypeConnAck})
			}

		case relay.MsgTypeData:
			// 数据消息，转发到对应的 stream
			stream := c.wsConn.GetStreams().GetStream(msg.StreamID)
			if stream != nil {
				stream.Write(msg.Payload)
			}

		case relay.MsgTypeClose:
			// 关闭消息
			c.wsConn.GetStreams().RemoveStream(msg.StreamID)

		case relay.MsgTypeError:
			// 错误消息，通知等待的 stream
			stream := c.wsConn.GetStreams().GetStream(msg.StreamID)
			if stream != nil {
				stream.Write([]byte{relay.MsgTypeError})
				stream.Close()
			}

		case relay.MsgTypeRuleUpdate:
			// 规则更新通知，重新获取规则
			log.Info().Msg("Received rule update notification, fetching new rules...")
			go func() {
				if err := c.fetchAndApplyRules(); err != nil {
					log.Warn().Err(err).Msg("Failed to fetch updated rules")
				}
			}()
		}
	}
}

// handleIncomingConnect 处理入站的连接请求 (作为出口节点)
func (c *Client) handleIncomingConnect(msg *relay.TunnelMessage) {
	target := msg.Target
	log.Debug().
		Uint32("stream_id", msg.StreamID).
		Str("target", target).
		Msg("Handling incoming connect request")

	// 连接目标
	targetConn, err := net.DialTimeout("tcp", target, time.Duration(c.cfg.Forwarder.ConnectTimeout)*time.Second)
	if err != nil {
		log.Warn().Err(err).Str("target", target).Msg("Failed to connect to target")
		// 发送错误响应
		errMsg := &relay.TunnelMessage{
			Type:     relay.MsgTypeError,
			StreamID: msg.StreamID,
			Error:    err.Error(),
		}
		c.wsConn.Send(errMsg)
		return
	}

	// 创建一个 stream 用于跟踪此连接
	stream := &relay.Stream{
		ID:      msg.StreamID,
		Target:  target,
		DataCh:  make(chan []byte, 100),
		CloseCh: make(chan struct{}),
	}

	// 手动添加到 streams 管理器
	c.wsConn.GetStreams().AddStream(stream)
	defer c.wsConn.GetStreams().RemoveStream(msg.StreamID)
	defer targetConn.Close()

	// 发送 ConnAck
	ackMsg := &relay.TunnelMessage{
		Type:     relay.MsgTypeConnAck,
		StreamID: msg.StreamID,
	}
	if err := c.wsConn.Send(ackMsg); err != nil {
		return
	}

	log.Debug().Uint32("stream_id", msg.StreamID).Msg("Tunnel connected to target")

	// 双向转发
	done := make(chan struct{}, 2)

	// 目标 -> 隧道
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, c.cfg.Forwarder.BufferSize)
		for {
			n, err := targetConn.Read(buf)
			if err != nil {
				return
			}

			dataMsg := &relay.TunnelMessage{
				Type:     relay.MsgTypeData,
				StreamID: msg.StreamID,
				Payload:  make([]byte, n),
			}
			copy(dataMsg.Payload, buf[:n])

			if err := c.wsConn.Send(dataMsg); err != nil {
				return
			}
		}
	}()

	// 隧道 -> 目标
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			select {
			case data := <-stream.DataCh:
				if _, err := targetConn.Write(data); err != nil {
					return
				}
			case <-stream.CloseCh:
				return
			}
		}
	}()

	// 等待任一方向完成
	<-done

	// 发送关闭消息
	closeMsg := &relay.TunnelMessage{
		Type:     relay.MsgTypeClose,
		StreamID: msg.StreamID,
	}
	c.wsConn.Send(closeMsg)
}

func (c *Client) heartbeatLoop() {
	ticker := time.NewTicker(time.Duration(c.cfg.Connection.HeartbeatInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(); err != nil {
				log.Warn().Err(err).Msg("Heartbeat failed")
			}
		}
	}
}

func (c *Client) sendHeartbeat() error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "heartbeat",
		"method":  "clientHeartbeat",
		"params": map[string]interface{}{
			"client_id": c.clientID,
		},
	}

	_, err := c.rpcCall(req)
	return err
}

func (c *Client) fetchAndApplyRules() error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "getRules",
		"method":  "clientGetRules",
		"params": map[string]interface{}{
			"client_id": c.clientID,
		},
	}

	resp, err := c.rpcCall(req)
	if err != nil {
		return err
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid response")
	}

	rules, ok := result["rules"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid rules format")
	}

	c.applyRules(rules)
	return nil
}

func (c *Client) applyRules(rules []interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 停止不再需要的 forwarder
	newRuleIDs := make(map[string]bool)
	for _, r := range rules {
		rule := r.(map[string]interface{})
		newRuleIDs[rule["id"].(string)] = true
	}

	for id, f := range c.forwarders {
		if !newRuleIDs[id] {
			f.Stop()
			delete(c.forwarders, id)
			log.Info().Str("rule_id", id).Msg("Stopped forwarder")
		}
	}

	// 启动新的 forwarder
	for _, r := range rules {
		rule := r.(map[string]interface{})
		id := rule["id"].(string)

		if _, exists := c.forwarders[id]; exists {
			continue
		}

		ruleType := rule["type"].(string)

		switch ruleType {
		case "direct":
			f := NewForwarder(
				id,
				rule["listen_addr"].(string),
				rule["target_addr"].(string),
				c.cfg.Forwarder,
				c.trafficCounter,
			)
			c.forwarders[id] = f
			go f.Start()
			log.Info().
				Str("rule_id", id).
				Str("listen", rule["listen_addr"].(string)).
				Str("target", rule["target_addr"].(string)).
				Msg("Started direct forwarder")

		case "relay":
			if c.wsConn == nil {
				log.Warn().Str("rule_id", id).Msg("Cannot start relay forwarder: WebSocket not connected")
				continue
			}

			// 解析中继链
			var relayChain []string
			if chain, ok := rule["relay_chain"].([]interface{}); ok {
				for _, v := range chain {
					relayChain = append(relayChain, v.(string))
				}
			}

			exitAddr := ""
			if ea, ok := rule["exit_addr"].(string); ok {
				exitAddr = ea
			}

			f := NewRelayForwarder(
				id,
				rule["listen_addr"].(string),
				exitAddr,
				relayChain,
				c.cfg.Forwarder,
				c.wsConn,
			)
			c.forwarders[id] = f
			go f.Start()
			log.Info().
				Str("rule_id", id).
				Str("listen", rule["listen_addr"].(string)).
				Str("exit", exitAddr).
				Strs("relay_chain", relayChain).
				Msg("Started relay forwarder")
		}
	}
}

func (c *Client) trafficReportLoop() {
	// 每 30 秒上报一次流量
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			// 最后一次上报
			c.reportTraffic()
			return
		case <-ticker.C:
			c.reportTraffic()
		}
	}
}

func (c *Client) reportTraffic() {
	reports := c.trafficCounter.GetAndReset()
	if len(reports) == 0 {
		return
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "reportTraffic",
		"method":  "clientReportTraffic",
		"params": map[string]interface{}{
			"client_id": c.clientID,
			"reports":   reports,
		},
	}

	if _, err := c.rpcCall(req); err != nil {
		log.Warn().Err(err).Msg("Failed to report traffic")
	} else {
		log.Debug().Int("rules", len(reports)).Msg("Traffic reported")
	}
}

func (c *Client) rpcCall(request map[string]interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(
		c.cfg.Client.ServerURL+"/api/rpc",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}
