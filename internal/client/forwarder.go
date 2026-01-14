package client

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type Forwarder struct {
	id         string
	listenAddr string
	targetAddr string
	cfg        ForwarderSection

	listener net.Listener
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewForwarder(id, listenAddr, targetAddr string, cfg ForwarderSection) *Forwarder {
	return &Forwarder{
		id:         id,
		listenAddr: listenAddr,
		targetAddr: targetAddr,
		cfg:        cfg,
		stopCh:     make(chan struct{}),
	}
}

func (f *Forwarder) Start() error {
	listener, err := net.Listen("tcp", f.listenAddr)
	if err != nil {
		return err
	}
	f.listener = listener

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

	// 连接目标
	targetConn, err := net.DialTimeout("tcp", f.targetAddr, time.Duration(f.cfg.ConnectTimeout)*time.Second)
	if err != nil {
		log.Warn().Err(err).Str("target", f.targetAddr).Msg("Failed to connect to target")
		return
	}
	defer targetConn.Close()

	// 双向转发
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(targetConn, clientConn)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(clientConn, targetConn)
		done <- struct{}{}
	}()

	// 等待任一方向完成
	<-done
}
