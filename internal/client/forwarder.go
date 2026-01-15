package client

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// StatusCallback 状态回调函数类型
type StatusCallback func(ruleID, status, errMsg string)

type Forwarder struct {
	id         string
	listenAddr string
	targetAddr string
	cfg        ForwarderSection

	listener       net.Listener
	stopCh         chan struct{}
	wg             sync.WaitGroup
	trafficCounter *TrafficCounter
	statusCallback StatusCallback
}

func NewForwarder(id, listenAddr, targetAddr string, cfg ForwarderSection, tc *TrafficCounter, cb StatusCallback) *Forwarder {
	return &Forwarder{
		id:             id,
		listenAddr:     listenAddr,
		targetAddr:     targetAddr,
		cfg:            cfg,
		stopCh:         make(chan struct{}),
		trafficCounter: tc,
		statusCallback: cb,
	}
}

func (f *Forwarder) Start() error {
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
		Str("target", f.targetAddr).
		Msg("Forwarder started")

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

func (f *Forwarder) Stop() {
	close(f.stopCh)
	if f.listener != nil {
		f.listener.Close()
	}
	f.wg.Wait()
}

func (f *Forwarder) handleConnection(clientConn net.Conn) {
	defer f.wg.Done()
	defer clientConn.Close()

	// 统计连接数
	if f.trafficCounter != nil {
		f.trafficCounter.IncrementConn(f.id)
		defer f.trafficCounter.DecrementConn(f.id)
	}

	// 连接目标
	targetConn, err := net.DialTimeout("tcp", f.targetAddr, time.Duration(f.cfg.ConnectTimeout)*time.Second)
	if err != nil {
		log.Warn().Err(err).Str("target", f.targetAddr).Msg("Failed to connect to target")
		return
	}
	defer targetConn.Close()

	// 双向转发
	done := make(chan struct{}, 2)

	// 客户端 -> 目标 (出站流量)
	go func() {
		if f.trafficCounter != nil {
			countingReader := NewCountingReader(clientConn, f.trafficCounter, f.id, false)
			io.Copy(targetConn, countingReader)
		} else {
			io.Copy(targetConn, clientConn)
		}
		done <- struct{}{}
	}()

	// 目标 -> 客户端 (入站流量)
	go func() {
		if f.trafficCounter != nil {
			countingReader := NewCountingReader(targetConn, f.trafficCounter, f.id, true)
			io.Copy(clientConn, countingReader)
		} else {
			io.Copy(clientConn, targetConn)
		}
		done <- struct{}{}
	}()

	// 等待任一方向完成
	<-done
}
