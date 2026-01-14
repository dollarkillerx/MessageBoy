package relay

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// LoadBalancerInterface 负载均衡器接口 (避免循环依赖)
type LoadBalancerInterface interface {
	ResolveTarget(target string, clientIP string) (clientID string, nodeID string, err error)
	IncrementConnections(nodeID string) error
	DecrementConnections(nodeID string) error
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSServer struct {
	clients map[string]*WSClient
	mu      sync.RWMutex

	// 路由表: streamID -> 路由信息
	routes   map[uint32]*RouteInfo
	routesMu sync.RWMutex

	// 负载均衡器
	loadBalancer LoadBalancerInterface
}

// RouteInfo 中继路由信息
type RouteInfo struct {
	SourceClientID string // 源 Client ID
	TargetClientID string // 目标 Client ID (下一跳或出口)
	StreamID       uint32 // 流 ID
	ExitAddr       string // 最终目标地址
	NodeID         string // 代理组节点 ID (用于连接统计)
}

// SetLoadBalancer 设置负载均衡器
func (s *WSServer) SetLoadBalancer(lb LoadBalancerInterface) {
	s.loadBalancer = lb
}

type WSClient struct {
	ID       string
	Conn     *websocket.Conn
	SendCh   chan []byte
	CloseCh  chan struct{}
	closed   bool
	mu       sync.Mutex
}

func NewWSServer() *WSServer {
	return &WSServer{
		clients: make(map[string]*WSClient),
		routes:  make(map[uint32]*RouteInfo),
	}
}

func (s *WSServer) HandleConnection(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		http.Error(w, "client_id required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("WebSocket upgrade failed")
		return
	}

	client := &WSClient{
		ID:      clientID,
		Conn:    conn,
		SendCh:  make(chan []byte, 256),
		CloseCh: make(chan struct{}),
	}

	s.mu.Lock()
	// 关闭旧连接
	if old, ok := s.clients[clientID]; ok {
		old.Close()
	}
	s.clients[clientID] = client
	s.mu.Unlock()

	log.Info().Str("client_id", clientID).Msg("WebSocket client connected")

	go client.writePump()
	client.readPump(s)

	s.mu.Lock()
	delete(s.clients, clientID)
	s.mu.Unlock()

	// 清理该 client 相关的路由
	s.cleanupRoutesForClient(clientID)

	log.Info().Str("client_id", clientID).Msg("WebSocket client disconnected")
}

func (s *WSServer) GetClient(clientID string) *WSClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clients[clientID]
}

func (s *WSServer) SendToClient(clientID string, data []byte) bool {
	client := s.GetClient(clientID)
	if client == nil {
		return false
	}
	return client.Send(data)
}

func (s *WSServer) cleanupRoutesForClient(clientID string) {
	s.routesMu.Lock()
	defer s.routesMu.Unlock()

	for streamID, route := range s.routes {
		if route.SourceClientID == clientID || route.TargetClientID == clientID {
			delete(s.routes, streamID)
		}
	}
}

func (c *WSClient) readPump(server *WSServer) {
	defer c.Close()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Warn().Err(err).Str("client_id", c.ID).Msg("WebSocket read error")
			}
			return
		}

		msg, err := UnmarshalTunnelMessage(message)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to unmarshal tunnel message")
			continue
		}

		// 根据消息类型处理
		switch msg.Type {
		case MsgTypeConnect:
			server.handleConnect(c.ID, msg)

		case MsgTypeConnAck:
			server.handleConnAck(c.ID, msg)

		case MsgTypeData:
			server.handleData(c.ID, msg)

		case MsgTypeClose:
			server.handleClose(c.ID, msg)

		case MsgTypeError:
			server.handleError(c.ID, msg)
		}
	}
}

// handleConnect 处理连接请求 - 路由到目标 Client
func (s *WSServer) handleConnect(sourceClientID string, msg *TunnelMessage) {
	log.Debug().
		Str("source", sourceClientID).
		Uint32("stream_id", msg.StreamID).
		Str("target", msg.Target).
		Msg("Handling connect request")

	// Payload 中携带下一跳 Client ID 或代理组引用 (@group_name)
	var targetRef string
	if len(msg.Payload) > 0 {
		targetRef = string(msg.Payload)
	}

	if targetRef == "" {
		log.Warn().Msg("No target client specified in relay connect")
		s.sendError(sourceClientID, msg.StreamID, "no target client specified")
		return
	}

	// 解析目标：支持 @group_name 或直接 client_id
	var targetClientID, nodeID string
	var err error

	if s.loadBalancer != nil && len(targetRef) > 0 && targetRef[0] == '@' {
		// 代理组引用，使用负载均衡器选择节点
		targetClientID, nodeID, err = s.loadBalancer.ResolveTarget(targetRef, sourceClientID)
		if err != nil {
			log.Warn().Err(err).Str("target_ref", targetRef).Msg("Failed to resolve proxy group")
			s.sendError(sourceClientID, msg.StreamID, "proxy group resolution failed: "+err.Error())
			return
		}
		log.Debug().
			Str("group_ref", targetRef).
			Str("selected_client", targetClientID).
			Str("node_id", nodeID).
			Msg("Resolved proxy group to client")
	} else {
		// 直接是 client ID
		targetClientID = targetRef
	}

	// 检查目标 Client 是否在线
	targetClient := s.GetClient(targetClientID)
	if targetClient == nil {
		log.Warn().Str("target_client", targetClientID).Msg("Target client not online")
		s.sendError(sourceClientID, msg.StreamID, "target client not online")
		return
	}

	// 增加节点连接计数
	if nodeID != "" && s.loadBalancer != nil {
		s.loadBalancer.IncrementConnections(nodeID)
	}

	// 保存路由信息
	s.routesMu.Lock()
	s.routes[msg.StreamID] = &RouteInfo{
		SourceClientID: sourceClientID,
		TargetClientID: targetClientID,
		StreamID:       msg.StreamID,
		ExitAddr:       msg.Target,
		NodeID:         nodeID,
	}
	s.routesMu.Unlock()

	// 转发 Connect 消息到目标 Client
	// 清除 payload 中的下一跳信息，保留 target 地址
	forwardMsg := &TunnelMessage{
		Type:     MsgTypeConnect,
		StreamID: msg.StreamID,
		Target:   msg.Target,
	}

	data, err := forwardMsg.Marshal()
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal forward message")
		s.sendError(sourceClientID, msg.StreamID, "internal error")
		s.cleanupRoute(msg.StreamID)
		return
	}

	if !targetClient.Send(data) {
		log.Warn().Str("target", targetClientID).Msg("Failed to send to target client")
		s.sendError(sourceClientID, msg.StreamID, "failed to send to target")
		s.cleanupRoute(msg.StreamID)
	}
}

// cleanupRoute 清理路由并减少节点连接计数
func (s *WSServer) cleanupRoute(streamID uint32) {
	s.routesMu.Lock()
	route, ok := s.routes[streamID]
	if ok {
		delete(s.routes, streamID)
	}
	s.routesMu.Unlock()

	// 减少节点连接计数
	if ok && route.NodeID != "" && s.loadBalancer != nil {
		s.loadBalancer.DecrementConnections(route.NodeID)
	}
}

// handleConnAck 处理连接确认 - 路由回源 Client
func (s *WSServer) handleConnAck(fromClientID string, msg *TunnelMessage) {
	s.routesMu.RLock()
	route, ok := s.routes[msg.StreamID]
	s.routesMu.RUnlock()

	if !ok {
		log.Warn().Uint32("stream_id", msg.StreamID).Msg("No route found for ConnAck")
		return
	}

	// ConnAck 应该从 Target 发往 Source
	if fromClientID != route.TargetClientID {
		log.Warn().
			Str("from", fromClientID).
			Str("expected", route.TargetClientID).
			Msg("ConnAck from unexpected client")
		return
	}

	// 转发到源 Client
	data, _ := msg.Marshal()
	s.SendToClient(route.SourceClientID, data)
}

// handleData 处理数据消息 - 双向路由
func (s *WSServer) handleData(fromClientID string, msg *TunnelMessage) {
	s.routesMu.RLock()
	route, ok := s.routes[msg.StreamID]
	s.routesMu.RUnlock()

	if !ok {
		log.Debug().Uint32("stream_id", msg.StreamID).Msg("No route found for data")
		return
	}

	// 确定转发目标
	var targetClientID string
	if fromClientID == route.SourceClientID {
		targetClientID = route.TargetClientID
	} else if fromClientID == route.TargetClientID {
		targetClientID = route.SourceClientID
	} else {
		log.Warn().
			Str("from", fromClientID).
			Uint32("stream_id", msg.StreamID).
			Msg("Data from unexpected client")
		return
	}

	// 转发数据
	data, _ := msg.Marshal()
	if !s.SendToClient(targetClientID, data) {
		log.Debug().
			Str("target", targetClientID).
			Uint32("stream_id", msg.StreamID).
			Msg("Failed to forward data")
	}
}

// handleClose 处理关闭消息
func (s *WSServer) handleClose(fromClientID string, msg *TunnelMessage) {
	s.routesMu.RLock()
	route, ok := s.routes[msg.StreamID]
	s.routesMu.RUnlock()

	if !ok {
		return
	}

	// 转发关闭消息到对端
	var targetClientID string
	if fromClientID == route.SourceClientID {
		targetClientID = route.TargetClientID
	} else {
		targetClientID = route.SourceClientID
	}

	data, _ := msg.Marshal()
	s.SendToClient(targetClientID, data)

	// 清理路由 (包括减少节点连接计数)
	s.cleanupRoute(msg.StreamID)

	log.Debug().Uint32("stream_id", msg.StreamID).Msg("Route closed")
}

// handleError 处理错误消息
func (s *WSServer) handleError(fromClientID string, msg *TunnelMessage) {
	s.routesMu.RLock()
	route, ok := s.routes[msg.StreamID]
	s.routesMu.RUnlock()

	if !ok {
		return
	}

	// 转发错误消息到对端
	var targetClientID string
	if fromClientID == route.SourceClientID {
		targetClientID = route.TargetClientID
	} else {
		targetClientID = route.SourceClientID
	}

	data, _ := msg.Marshal()
	s.SendToClient(targetClientID, data)

	// 清理路由 (包括减少节点连接计数)
	s.cleanupRoute(msg.StreamID)
}

func (s *WSServer) sendError(clientID string, streamID uint32, errMsg string) {
	msg := &TunnelMessage{
		Type:     MsgTypeError,
		StreamID: streamID,
		Error:    errMsg,
	}
	data, _ := msg.Marshal()
	s.SendToClient(clientID, data)
}

// NotifyRuleUpdate 通知 Client 规则已更新
func (s *WSServer) NotifyRuleUpdate(clientID string) bool {
	msg := &TunnelMessage{
		Type: MsgTypeRuleUpdate,
	}
	data, err := msg.Marshal()
	if err != nil {
		return false
	}
	return s.SendToClient(clientID, data)
}

// NotifyRuleUpdateToAll 通知所有 Client 规则已更新
func (s *WSServer) NotifyRuleUpdateToAll() {
	s.mu.RLock()
	clientIDs := make([]string, 0, len(s.clients))
	for id := range s.clients {
		clientIDs = append(clientIDs, id)
	}
	s.mu.RUnlock()

	msg := &TunnelMessage{
		Type: MsgTypeRuleUpdate,
	}
	data, err := msg.Marshal()
	if err != nil {
		return
	}

	for _, clientID := range clientIDs {
		s.SendToClient(clientID, data)
	}
}

// IsClientOnline 检查 Client 是否在线
func (s *WSServer) IsClientOnline(clientID string) bool {
	return s.GetClient(clientID) != nil
}

func (c *WSClient) writePump() {
	defer c.Close()

	for {
		select {
		case message, ok := <-c.SendCh:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.BinaryMessage, message); err != nil {
				log.Warn().Err(err).Str("client_id", c.ID).Msg("WebSocket write error")
				return
			}
		case <-c.CloseCh:
			return
		}
	}
}

func (c *WSClient) Send(data []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return false
	}

	select {
	case c.SendCh <- data:
		return true
	default:
		return false
	}
}

func (c *WSClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.closed {
		c.closed = true
		close(c.CloseCh)
		c.Conn.Close()
	}
}
