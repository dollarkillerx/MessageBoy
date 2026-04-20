package client

import (
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/dollarkillerx/MessageBoy/internal/relay"
)

// WSConnProvider 返回当前 WebSocket 连接。重连时提供者会返回新的连接对象。
type WSConnProvider func() *relay.WSClientConn

// RelayForwarder 处理中继转发
// 监听本地端口，将流量通过 WebSocket 隧道转发到远端
type RelayForwarder struct {
	id         string
	listenAddr string
	exitAddr   string
	relayChain []string
	cfg        ForwarderSection

	// wsConnProvider 每次调用返回最新的 wsConn，避免持有过期引用
	wsConnProvider WSConnProvider
	listener       net.Listener
	listenerMu     sync.Mutex
	stopCh         chan struct{}
	wg             sync.WaitGroup
	trafficCounter *TrafficCounter
	statusCallback StatusCallback
}

// NewRelayForwarder 创建中继转发器
func NewRelayForwarder(id, listenAddr, exitAddr string, relayChain []string, cfg ForwarderSection, provider WSConnProvider, tc *TrafficCounter, cb StatusCallback) *RelayForwarder {
	return &RelayForwarder{
		id:             id,
		listenAddr:     listenAddr,
		exitAddr:       exitAddr,
		relayChain:     relayChain,
		cfg:            cfg,
		wsConnProvider: provider,
		stopCh:         make(chan struct{}),
		trafficCounter: tc,
		statusCallback: cb,
	}
}

// Start 启动转发器
func (f *RelayForwarder) Start() error {
	listener, err := net.Listen("tcp", f.listenAddr)
	if err != nil {
		// 上报错误状态
		if f.statusCallback != nil {
			f.statusCallback(f.id, "error", err.Error())
		}
		return err
	}
	f.listenerMu.Lock()
	f.listener = listener
	f.listenerMu.Unlock()

	// 上报运行状态
	if f.statusCallback != nil {
		f.statusCallback(f.id, "running", "")
	}

	log.Info().
		Str("id", f.id).
		Str("listen", f.listenAddr).
		Str("exit", f.exitAddr).
		Strs("relay_chain", f.relayChain).
		Msg("Relay forwarder started")

	for {
		select {
		case <-f.stopCh:
			return nil
		default:
		}

		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-f.stopCh:
				return nil
			default:
				log.Warn().Err(err).Msg("Accept error")
				continue
			}
		}

		f.wg.Add(1)
		go f.handleConnection(conn)
	}
}

// Stop 停止转发器
func (f *RelayForwarder) Stop() {
	close(f.stopCh)
	f.listenerMu.Lock()
	if f.listener != nil {
		f.listener.Close()
	}
	f.listenerMu.Unlock()
	f.wg.Wait()
}

// GetConfigHash 返回配置的哈希值，用于比较配置是否变化
func (f *RelayForwarder) GetConfigHash() string {
	hash := "relay:" + f.listenAddr + ":" + f.exitAddr + ":"
	for _, r := range f.relayChain {
		hash += r + ","
	}
	return hash
}

// GetListenAddr 返回监听地址
func (f *RelayForwarder) GetListenAddr() string {
	return f.listenAddr
}

// handleConnection 处理单个连接（零拷贝优化）
func (f *RelayForwarder) handleConnection(clientConn net.Conn) {
	defer f.wg.Done()
	defer clientConn.Close()

	tuneTCPConn(clientConn)

	// 统计连接数
	if f.trafficCounter != nil {
		f.trafficCounter.IncrementConn(f.id)
		defer f.trafficCounter.DecrementConn(f.id)
	}

	// 连接生命周期内锁定一个 wsConn 快照（stream 归属于该连接）
	ws := f.wsConnProvider()
	if ws == nil {
		log.Warn().Str("rule_id", f.id).Msg("Relay forwarder dropping connection: wsConn unavailable")
		return
	}

	// 创建一个新的流
	stream := ws.GetStreams().NewStream(f.exitAddr)
	defer ws.GetStreams().RemoveStream(stream.ID)

	log.Debug().
		Uint32("stream_id", stream.ID).
		Str("exit", f.exitAddr).
		Msg("Creating relay tunnel")

	// 发送 Connect 请求
	connectMsg := &relay.TunnelMessage{
		Type:     relay.MsgTypeConnect,
		StreamID: stream.ID,
		Target:   f.exitAddr,
		RuleID:   f.id, // 用于服务端流量统计
	}

	if len(f.relayChain) > 0 {
		// 如果有中继链，payload 中携带下一跳信息
		connectMsg.Payload = []byte(f.relayChain[0])
	}

	if err := ws.Send(connectMsg); err != nil {
		log.Warn().Err(err).Msg("Failed to send connect message")
		return
	}

	// 等待 ConnAck 或 Error
	if !f.waitForConnAck(stream) {
		return
	}

	log.Debug().Uint32("stream_id", stream.ID).Msg("Relay tunnel established")

	// 双向转发：任一方向结束都关闭对端并等待两侧都退出
	var wg sync.WaitGroup
	wg.Add(2)
	go f.forwardToTunnel(ws, clientConn, stream, &wg)
	go f.forwardFromTunnel(clientConn, stream, &wg)
	wg.Wait()

	// 发送关闭消息
	closeMsg := &relay.TunnelMessage{
		Type:     relay.MsgTypeClose,
		StreamID: stream.ID,
	}
	ws.Send(closeMsg)
}

// waitForConnAck 等待连接确认
func (f *RelayForwarder) waitForConnAck(stream *relay.Stream) bool {
	timeout := time.After(time.Duration(f.cfg.ConnectTimeout) * time.Second)

	for {
		select {
		case <-timeout:
			log.Warn().
				Uint32("stream_id", stream.ID).
				Str("exit_addr", f.exitAddr).
				Strs("relay_chain", f.relayChain).
				Int("timeout_seconds", f.cfg.ConnectTimeout).
				Msg("Connect timeout")
			return false
		case <-stream.CloseCh:
			return false
		case data := <-stream.DataCh:
			// 这里收到的是来自 handleTunnelMessage 的信号
			if len(data) == 1 && data[0] == relay.MsgTypeConnAck {
				return true
			} else if len(data) == 1 && data[0] == relay.MsgTypeError {
				log.Warn().
					Uint32("stream_id", stream.ID).
					Str("exit_addr", f.exitAddr).
					Strs("relay_chain", f.relayChain).
					Msg("Connect rejected")
				return false
			}
		}
	}
}

// forwardToTunnel 从客户端转发到隧道（使用 buffer pool 优化）
func (f *RelayForwarder) forwardToTunnel(ws *relay.WSClientConn, clientConn net.Conn, stream *relay.Stream, wg *sync.WaitGroup) {
	defer wg.Done()
	// 一端结束时关闭 stream 与 conn，驱使另一端退出
	defer stream.Close()
	defer clientConn.Close()

	buf := relay.GetBuffer()
	defer relay.PutBuffer(buf)

	for {
		n, err := clientConn.Read((*buf)[relay.HeaderSize:])
		if err != nil {
			return
		}

		if f.trafficCounter != nil {
			f.trafficCounter.AddBytesOut(f.id, int64(n))
		}

		msg := &relay.TunnelMessage{
			Type:     relay.MsgTypeData,
			StreamID: stream.ID,
			Payload:  (*buf)[relay.HeaderSize : relay.HeaderSize+n],
		}

		if err := ws.Send(msg); err != nil {
			return
		}
	}
}

// forwardFromTunnel 从隧道转发到客户端
func (f *RelayForwarder) forwardFromTunnel(clientConn net.Conn, stream *relay.Stream, wg *sync.WaitGroup) {
	defer wg.Done()
	defer stream.Close()
	defer clientConn.Close()

	for {
		select {
		case data, ok := <-stream.DataCh:
			if !ok {
				return
			}
			if f.trafficCounter != nil {
				f.trafficCounter.AddBytesIn(f.id, int64(len(data)))
			}
			if _, err := clientConn.Write(data); err != nil {
				return
			}
		case <-stream.CloseCh:
			return
		}
	}
}

// tuneTCPConn 调校 TCP 连接：开启 KeepAlive（感知半开连接）和 NoDelay（关闭 Nagle，降低小包延迟）
func tuneTCPConn(conn net.Conn) {
	tc, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	_ = tc.SetKeepAlive(true)
	_ = tc.SetKeepAlivePeriod(30 * time.Second)
	_ = tc.SetNoDelay(true)
}
