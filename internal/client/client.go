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

	stopCh      chan struct{}
	reconnectCh chan struct{} // 触发重连
}

// ForwarderInterface 转发器接口
type ForwarderInterface interface {
	Start() error
	Stop()
	GetConfigHash() string
	GetListenAddr() string
}

func New(cfg *ClientConfig) *Client {
	return &Client{
		cfg:            cfg,
		forwarders:     make(map[string]ForwarderInterface),
		trafficCounter: NewTrafficCounter(),
		stopCh:         make(chan struct{}),
		reconnectCh:    make(chan struct{}, 1),
	}
}

func (c *Client) Run() error {
	log.Info().Str("server", c.cfg.Client.ServerURL).Msg("Starting MessageBoy Client")

	// 首次注册，带重试
	if err := c.registerWithRetry(); err != nil {
		return fmt.Errorf("failed to register after retries: %w", err)
	}

	log.Info().Str("client_id", c.clientID).Msg("Registered successfully")

	// 启动主循环
	go c.mainLoop()

	// 启动心跳
	go c.heartbeatLoop()

	// 启动流量上报
	go c.trafficReportLoop()

	// 等待停止信号
	<-c.stopCh
	return nil
}

// registerWithRetry 带重试的注册
func (c *Client) registerWithRetry() error {
	maxRetries := 10
	baseDelay := time.Second

	for i := 0; i < maxRetries; i++ {
		select {
		case <-c.stopCh:
			return fmt.Errorf("stopped")
		default:
		}

		if err := c.register(); err != nil {
			delay := baseDelay * time.Duration(1<<uint(i)) // 指数退避
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
			log.Warn().Err(err).Int("attempt", i+1).Dur("retry_in", delay).Msg("Register failed, retrying...")
			time.Sleep(delay)
			continue
		}
		return nil
	}
	return fmt.Errorf("max retries exceeded")
}

// mainLoop 主循环，负责连接和重连
func (c *Client) mainLoop() {
	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		// 建立 WebSocket 连接
		if err := c.connectWebSocket(); err != nil {
			log.Warn().Err(err).Msg("Failed to connect WebSocket, will retry...")
			c.waitBeforeReconnect(5 * time.Second)
			continue
		}

		// 获取初始规则
		if err := c.fetchAndApplyRules(); err != nil {
			log.Warn().Err(err).Msg("Failed to fetch initial rules")
		}

		// 处理隧道消息（阻塞直到连接断开）
		c.handleTunnelMessages()

		log.Warn().Msg("WebSocket disconnected, reconnecting...")
		c.waitBeforeReconnect(3 * time.Second)
	}
}

// waitBeforeReconnect 等待一段时间后重连
func (c *Client) waitBeforeReconnect(delay time.Duration) {
	select {
	case <-c.stopCh:
	case <-time.After(delay):
	}
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

	params := map[string]interface{}{
		"token":    c.cfg.Client.Token,
		"hostname": hostname,
		"version":  "2.0.0",
	}
	// 如果配置了 ReportIP，则上报（用于显示）
	if c.cfg.Client.ReportIP != "" {
		params["report_ip"] = c.cfg.Client.ReportIP
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "register",
		"method":  "clientRegister",
		"params":  params,
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
				log.Debug().Uint32("stream_id", msg.StreamID).Msg("Received ConnAck, notifying stream")
				stream.Write([]byte{relay.MsgTypeConnAck})
			} else {
				log.Warn().Uint32("stream_id", msg.StreamID).Msg("Received ConnAck but stream not found")
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
			log.Debug().
				Uint32("stream_id", msg.StreamID).
				Str("error", msg.Error).
				Msg("Received error message")
			stream := c.wsConn.GetStreams().GetStream(msg.StreamID)
			if stream != nil {
				stream.Write([]byte{relay.MsgTypeError})
				stream.Close()
			} else {
				log.Warn().Uint32("stream_id", msg.StreamID).Msg("Received Error but stream not found")
			}

		case relay.MsgTypeRuleUpdate:
			// 规则更新通知，重新获取规则
			log.Info().Str("client_id", c.clientID).Msg("=== Received MsgTypeRuleUpdate from server ===")
			go func() {
				log.Debug().Msg("Starting to fetch and apply new rules...")
				if err := c.fetchAndApplyRules(); err != nil {
					log.Warn().Err(err).Msg("Failed to fetch updated rules")
				} else {
					log.Info().Msg("Successfully fetched and applied new rules")
				}
			}()

		case relay.MsgTypeCheckPort:
			// 端口检查请求
			go c.handleCheckPort(msg)
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
		log.Warn().Err(err).Uint32("stream_id", msg.StreamID).Msg("Failed to send ConnAck")
		return
	}

	log.Debug().
		Uint32("stream_id", msg.StreamID).
		Str("target", target).
		Msg("ConnAck sent, tunnel connected to target")

	// 双向转发（使用 buffer pool 优化）
	done := make(chan struct{}, 2)

	// 目标 -> 隧道（零拷贝优化）
	go func() {
		defer func() { done <- struct{}{} }()
		// 使用 buffer pool
		buf := relay.GetBuffer()
		defer relay.PutBuffer(buf)

		for {
			// 直接读取到 buffer 的 payload 区域
			n, err := targetConn.Read((*buf)[relay.HeaderSize:])
			if err != nil {
				return
			}

			dataMsg := &relay.TunnelMessage{
				Type:     relay.MsgTypeData,
				StreamID: msg.StreamID,
				Payload:  (*buf)[relay.HeaderSize : relay.HeaderSize+n],
			}

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

// handleCheckPort 处理端口检查请求
func (c *Client) handleCheckPort(msg *relay.TunnelMessage) {
	addr := msg.Target
	currentRuleID := msg.RuleID

	log.Info().
		Str("addr", addr).
		Str("current_rule_id", currentRuleID).
		Uint32("request_id", msg.StreamID).
		Msg("=== Received port check request ===")

	var errMsg string

	// 检查是否是当前规则正在使用的端口（如果是，则允许更新）
	c.mu.RLock()
	if currentRuleID != "" {
		if f, exists := c.forwarders[currentRuleID]; exists {
			if f.GetListenAddr() == addr {
				log.Info().
					Str("addr", addr).
					Str("rule_id", currentRuleID).
					Msg("Port is used by current rule, will be restarted - available")
				c.mu.RUnlock()
				// 发送成功响应
				c.sendPortCheckResult(msg.StreamID, "")
				return
			}
		}
	}
	c.mu.RUnlock()

	// 尝试监听端口
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		errMsg = "该端口已被其他程序占用"
		log.Warn().Str("addr", addr).Err(err).Msg("Port check failed - port not available")
	} else {
		listener.Close()
		log.Info().Str("addr", addr).Msg("Port check passed - port available")
	}

	// 发送检查结果
	c.sendPortCheckResult(msg.StreamID, errMsg)
}

// sendPortCheckResult 发送端口检查结果
func (c *Client) sendPortCheckResult(requestID uint32, errMsg string) {
	resultMsg := &relay.TunnelMessage{
		Type:     relay.MsgTypeCheckPortResult,
		StreamID: requestID,
		Error:    errMsg,
	}

	if err := c.wsConn.Send(resultMsg); err != nil {
		log.Warn().Err(err).Uint32("request_id", requestID).Msg("Failed to send port check result")
	} else {
		log.Info().
			Uint32("request_id", requestID).
			Bool("available", errMsg == "").
			Str("error", errMsg).
			Msg("Port check result sent")
	}
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

// computeRuleConfigHash 计算规则配置的哈希值
func computeRuleConfigHash(rule map[string]interface{}) string {
	ruleType := rule["type"].(string)
	listenAddr := rule["listen_addr"].(string)

	if ruleType == "direct" {
		targetAddr := ""
		if ta, ok := rule["target_addr"].(string); ok {
			targetAddr = ta
		}
		return "direct:" + listenAddr + ":" + targetAddr
	}

	// relay type
	exitAddr := ""
	if ea, ok := rule["exit_addr"].(string); ok {
		exitAddr = ea
	}
	hash := "relay:" + listenAddr + ":" + exitAddr + ":"
	if chain, ok := rule["relay_chain"].([]interface{}); ok {
		for _, v := range chain {
			hash += v.(string) + ","
		}
	}
	return hash
}

func (c *Client) applyRules(rules []interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Info().Int("rule_count", len(rules)).Int("current_forwarders", len(c.forwarders)).Msg("Applying rules")

	// 停止不再需要的 forwarder
	newRuleIDs := make(map[string]bool)
	for _, r := range rules {
		rule := r.(map[string]interface{})
		newRuleIDs[rule["id"].(string)] = true
	}

	for id, f := range c.forwarders {
		if !newRuleIDs[id] {
			log.Info().Str("rule_id", id).Msg("Stopping forwarder (rule removed)")
			f.Stop()
			delete(c.forwarders, id)
			log.Info().Str("rule_id", id).Msg("Stopped forwarder (rule removed)")
		}
	}

	// 状态回调
	statusCallback := func(ruleID, status, errMsg string) {
		go c.reportRuleStatus(ruleID, status, errMsg)
	}

	// 启动新的或更新的 forwarder
	for _, r := range rules {
		rule := r.(map[string]interface{})
		id := rule["id"].(string)
		listenAddr := rule["listen_addr"].(string)

		log.Debug().
			Str("rule_id", id).
			Str("listen_addr", listenAddr).
			Str("type", rule["type"].(string)).
			Msg("Processing rule")

		// 检查是否需要更新已存在的 forwarder
		if existingF, exists := c.forwarders[id]; exists {
			newConfigHash := computeRuleConfigHash(rule)
			oldConfigHash := existingF.GetConfigHash()
			log.Debug().
				Str("rule_id", id).
				Str("old_hash", oldConfigHash).
				Str("new_hash", newConfigHash).
				Bool("changed", oldConfigHash != newConfigHash).
				Msg("Comparing config hash")
			if oldConfigHash == newConfigHash {
				// 配置未变化，跳过
				log.Debug().Str("rule_id", id).Msg("Config unchanged, skipping")
				continue
			}
			// 配置已变化，停止旧的
			log.Info().
				Str("rule_id", id).
				Str("old_hash", oldConfigHash).
				Str("new_hash", newConfigHash).
				Msg("Config changed, restarting forwarder")
			existingF.Stop()
			delete(c.forwarders, id)
		} else {
			log.Debug().Str("rule_id", id).Msg("New rule, will create forwarder")
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
				statusCallback,
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
				go c.reportRuleStatus(id, "error", "WebSocket not connected")
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
				statusCallback,
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
	// 每 1 秒上报一次流量 (用于实时带宽显示)
	ticker := time.NewTicker(1 * time.Second)
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

func (c *Client) reportRuleStatus(ruleID, status, errMsg string) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "reportRuleStatus",
		"method":  "clientReportRuleStatus",
		"params": map[string]interface{}{
			"client_id": c.clientID,
			"reports": []map[string]interface{}{
				{
					"rule_id": ruleID,
					"status":  status,
					"error":   errMsg,
				},
			},
		},
	}

	if _, err := c.rpcCall(req); err != nil {
		log.Warn().Err(err).Str("rule_id", ruleID).Msg("Failed to report rule status")
	} else {
		log.Debug().Str("rule_id", ruleID).Str("status", status).Msg("Rule status reported")
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
