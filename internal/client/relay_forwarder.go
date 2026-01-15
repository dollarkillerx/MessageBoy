package client

import (
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/dollarkillerx/MessageBoy/internal/relay"
)

// RelayForwarder 处理中继转发
// 监听本地端口，将流量通过 WebSocket 隧道转发到远端
type RelayForwarder struct {
	id         string
	listenAddr string
	exitAddr   string
	relayChain []string
	cfg        ForwarderSection

	wsConn         *relay.WSClientConn
	listener       net.Listener
	stopCh         chan struct{}
	wg             sync.WaitGroup
	statusCallback StatusCallback
}

// NewRelayForwarder 创建中继转发器
func NewRelayForwarder(id, listenAddr, exitAddr string, relayChain []string, cfg ForwarderSection, wsConn *relay.WSClientConn, cb StatusCallback) *RelayForwarder {
	return &RelayForwarder{
		id:             id,
		listenAddr:     listenAddr,
		exitAddr:       exitAddr,
		relayChain:     relayChain,
		cfg:            cfg,
		wsConn:         wsConn,
		stopCh:         make(chan struct{}),
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
	f.listener = listener

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
	if f.listener != nil {
		f.listener.Close()
	}
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

	// 创建一个新的流
	stream := f.wsConn.GetStreams().NewStream(f.exitAddr)
	defer f.wsConn.GetStreams().RemoveStream(stream.ID)

	log.Debug().
		Uint32("stream_id", stream.ID).
		Str("exit", f.exitAddr).
		Msg("Creating relay tunnel")

	// 发送 Connect 请求
	connectMsg := &relay.TunnelMessage{
		Type:     relay.MsgTypeConnect,
		StreamID: stream.ID,
		Target:   f.exitAddr,
	}

	if len(f.relayChain) > 0 {
		// 如果有中继链，payload 中携带下一跳信息
		connectMsg.Payload = []byte(f.relayChain[0])
	}

	if err := f.wsConn.Send(connectMsg); err != nil {
		log.Warn().Err(err).Msg("Failed to send connect message")
		return
	}

	// 等待 ConnAck 或 Error
	if !f.waitForConnAck(stream) {
		return
	}

	log.Debug().Uint32("stream_id", stream.ID).Msg("Relay tunnel established")

	// 双向转发（使用零拷贝优化）
	done := make(chan struct{}, 2)

	// 客户端 -> 隧道（使用 buffer pool）
	go f.forwardToTunnel(clientConn, stream, done)

	// 隧道 -> 客户端
	go f.forwardFromTunnel(clientConn, stream, done)

	// 等待任一方向完成
	<-done

	// 发送关闭消息
	closeMsg := &relay.TunnelMessage{
		Type:     relay.MsgTypeClose,
		StreamID: stream.ID,
	}
	f.wsConn.Send(closeMsg)
}

// waitForConnAck 等待连接确认
func (f *RelayForwarder) waitForConnAck(stream *relay.Stream) bool {
	timeout := time.After(time.Duration(f.cfg.ConnectTimeout) * time.Second)

	for {
		select {
		case <-timeout:
			log.Warn().Uint32("stream_id", stream.ID).Msg("Connect timeout")
			return false
		case <-stream.CloseCh:
			return false
		case data := <-stream.DataCh:
			// 这里收到的是来自 handleTunnelMessage 的信号
			if len(data) == 1 && data[0] == relay.MsgTypeConnAck {
				return true
			} else if len(data) == 1 && data[0] == relay.MsgTypeError {
				log.Warn().Uint32("stream_id", stream.ID).Msg("Connect rejected")
				return false
			}
		}
	}
}

// forwardToTunnel 从客户端转发到隧道（使用 buffer pool 优化）
func (f *RelayForwarder) forwardToTunnel(clientConn net.Conn, stream *relay.Stream, done chan struct{}) {
	defer func() { done <- struct{}{} }()

	// 使用 buffer pool
	buf := relay.GetBuffer()
	defer relay.PutBuffer(buf)

	for {
		// 直接读取到 buffer 的 payload 区域
		n, err := clientConn.Read((*buf)[relay.HeaderSize:])
		if err != nil {
			return
		}

		// 构建消息并发送
		msg := &relay.TunnelMessage{
			Type:     relay.MsgTypeData,
			StreamID: stream.ID,
			Payload:  (*buf)[relay.HeaderSize : relay.HeaderSize+n],
		}

		if err := f.wsConn.Send(msg); err != nil {
			return
		}
	}
}

// forwardFromTunnel 从隧道转发到客户端
func (f *RelayForwarder) forwardFromTunnel(clientConn net.Conn, stream *relay.Stream, done chan struct{}) {
	defer func() { done <- struct{}{} }()

	for {
		select {
		case data := <-stream.DataCh:
			if _, err := clientConn.Write(data); err != nil {
				return
			}
		case <-stream.CloseCh:
			return
		}
	}
}
