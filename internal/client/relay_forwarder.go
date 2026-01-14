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

	wsConn   *relay.WSClientConn
	listener net.Listener
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewRelayForwarder(id, listenAddr, exitAddr string, relayChain []string, cfg ForwarderSection, wsConn *relay.WSClientConn) *RelayForwarder {
	return &RelayForwarder{
		id:         id,
		listenAddr: listenAddr,
		exitAddr:   exitAddr,
		relayChain: relayChain,
		cfg:        cfg,
		wsConn:     wsConn,
		stopCh:     make(chan struct{}),
	}
}

func (f *RelayForwarder) Start() error {
	listener, err := net.Listen("tcp", f.listenAddr)
	if err != nil {
		return err
	}
	f.listener = listener

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

func (f *RelayForwarder) Stop() {
	close(f.stopCh)
	if f.listener != nil {
		f.listener.Close()
	}
	f.wg.Wait()
}

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
	ackReceived := false
	timeout := time.After(time.Duration(f.cfg.ConnectTimeout) * time.Second)

AckLoop:
	for {
		select {
		case <-timeout:
			log.Warn().Uint32("stream_id", stream.ID).Msg("Connect timeout")
			return
		case <-stream.CloseCh:
			return
		case data := <-stream.DataCh:
			// 这里收到的是来自 handleTunnelMessage 的信号
			if len(data) == 1 && data[0] == relay.MsgTypeConnAck {
				ackReceived = true
				break AckLoop
			} else if len(data) == 1 && data[0] == relay.MsgTypeError {
				log.Warn().Uint32("stream_id", stream.ID).Msg("Connect rejected")
				return
			}
		}
	}

	if !ackReceived {
		return
	}

	log.Debug().Uint32("stream_id", stream.ID).Msg("Relay tunnel established")

	// 双向转发
	done := make(chan struct{}, 2)

	// 客户端 -> 隧道
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, f.cfg.BufferSize)
		for {
			n, err := clientConn.Read(buf)
			if err != nil {
				return
			}

			dataMsg := &relay.TunnelMessage{
				Type:     relay.MsgTypeData,
				StreamID: stream.ID,
				Payload:  make([]byte, n),
			}
			copy(dataMsg.Payload, buf[:n])

			if err := f.wsConn.Send(dataMsg); err != nil {
				return
			}
		}
	}()

	// 隧道 -> 客户端
	go func() {
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
	}()

	// 等待任一方向完成
	<-done

	// 发送关闭消息
	closeMsg := &relay.TunnelMessage{
		Type:     relay.MsgTypeClose,
		StreamID: stream.ID,
	}
	f.wsConn.Send(closeMsg)
}
