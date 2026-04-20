package client

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// copyBufferSize: io.Copy 内部默认也是 32KB，这里复用同样大小，兼顾吞吐和内存
const copyBufferSize = 32 * 1024

var copyBufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, copyBufferSize)
		return &b
	},
}

// copyAndCount 在 dst/src 之间复制，并按 isIn 把字节数累加到 stat。
// 当 stat == nil 时退化为 io.Copy，让内核在可能时走 splice/sendfile 零拷贝路径。
func copyAndCount(dst io.Writer, src io.Reader, stat *RuleTraffic, isIn bool) (int64, error) {
	if stat == nil {
		return io.Copy(dst, src)
	}

	bufp := copyBufferPool.Get().(*[]byte)
	defer copyBufferPool.Put(bufp)
	buf := *bufp

	var total int64
	for {
		nr, rerr := src.Read(buf)
		if nr > 0 {
			nw, werr := dst.Write(buf[:nr])
			if nw > 0 {
				total += int64(nw)
				if isIn {
					atomic.AddInt64(&stat.BytesIn, int64(nw))
				} else {
					atomic.AddInt64(&stat.BytesOut, int64(nw))
				}
			}
			if werr != nil {
				return total, werr
			}
			if nw != nr {
				return total, io.ErrShortWrite
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				return total, nil
			}
			return total, rerr
		}
	}
}

// StatusCallback 状态回调函数类型
type StatusCallback func(ruleID, status, errMsg string)

type Forwarder struct {
	id         string
	listenAddr string
	targetAddr string
	cfg        ForwarderSection

	listener       net.Listener
	listenerMu     sync.Mutex
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
	f.listenerMu.Lock()
	if f.listener != nil {
		f.listener.Close()
	}
	f.listenerMu.Unlock()
	f.wg.Wait()
}

// GetConfigHash 返回配置的哈希值，用于比较配置是否变化
func (f *Forwarder) GetConfigHash() string {
	return "direct:" + f.listenAddr + ":" + f.targetAddr
}

// GetListenAddr 返回监听地址
func (f *Forwarder) GetListenAddr() string {
	return f.listenAddr
}

func (f *Forwarder) handleConnection(clientConn net.Conn) {
	defer f.wg.Done()
	defer clientConn.Close()

	tuneTCPConn(clientConn)

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
	tuneTCPConn(targetConn)

	// 预解析 *RuleTraffic，省掉每次 copyAndCount 的 map lookup；nil counter 触发 splice 快路径
	var stat *RuleTraffic
	if f.trafficCounter != nil {
		stat = f.trafficCounter.GetOrCreateStat(f.id)
	}

	// 双向转发：任一方向结束时 close 双端触发对端退出，两侧都退出后才返回
	var wg sync.WaitGroup
	wg.Add(2)

	// 客户端 -> 目标 (出站流量)
	go func() {
		defer wg.Done()
		defer targetConn.Close()
		defer clientConn.Close()
		copyAndCount(targetConn, clientConn, stat, false)
	}()

	// 目标 -> 客户端 (入站流量)
	go func() {
		defer wg.Done()
		defer clientConn.Close()
		defer targetConn.Close()
		copyAndCount(clientConn, targetConn, stat, true)
	}()

	wg.Wait()
}
