package relay

import (
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
)

// 消息类型
const (
	MsgTypeConnect         byte = 0x01
	MsgTypeConnAck         byte = 0x02
	MsgTypeData            byte = 0x03
	MsgTypeClose           byte = 0x04
	MsgTypeError           byte = 0x05
	MsgTypeRuleUpdate      byte = 0x06 // 规则更新通知
	MsgTypeCheckPort       byte = 0x07 // 端口检查请求
	MsgTypeCheckPortResult byte = 0x08 // 端口检查结果
)

// 协议常量
const (
	HeaderSize     = 9              // Type(1) + StreamID(4) + PayloadLen(4)
	MaxPayloadSize = 64 * 1024      // 64KB 最大 payload
	DefaultBufSize = 32 * 1024      // 32KB 默认 buffer
	MaxStringLen   = 4 * 1024       // 4KB 最大字符串长度
)

// 错误定义
var (
	ErrInvalidHeader  = errors.New("invalid message header")
	ErrPayloadTooLarge = errors.New("payload too large")
	ErrInvalidPayload = errors.New("invalid payload format")
	ErrBufferTooSmall = errors.New("buffer too small")
)

// BufferPool 全局 buffer 池
var BufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, DefaultBufSize+HeaderSize)
		return &buf
	},
}

// GetBuffer 从池中获取 buffer
func GetBuffer() *[]byte {
	return BufferPool.Get().(*[]byte)
}

// PutBuffer 归还 buffer 到池中
func PutBuffer(buf *[]byte) {
	if buf != nil && cap(*buf) >= DefaultBufSize {
		BufferPool.Put(buf)
	}
}

// TunnelMessage 隧道消息
type TunnelMessage struct {
	Type     byte
	StreamID uint32
	Target   string // 用于 Connect, CheckPort
	Payload  []byte // 用于 Data, Connect(携带下一跳)
	Error    string // 用于 Error, CheckPortResult
	RuleID   string // 用于流量统计, CheckPort
}

// MarshalBinary 二进制序列化
// 返回的 buffer 来自 pool，调用方需要在使用完后调用 PutBuffer 归还
func (m *TunnelMessage) MarshalBinary() (*[]byte, int, error) {
	buf := GetBuffer()
	n, err := m.MarshalTo(*buf)
	if err != nil {
		PutBuffer(buf)
		return nil, 0, err
	}
	return buf, n, nil
}

// MarshalTo 序列化到指定 buffer
// 返回写入的字节数
func (m *TunnelMessage) MarshalTo(buf []byte) (int, error) {
	// 先计算 payload 大小
	payloadSize := m.calcPayloadSize()
	totalSize := HeaderSize + payloadSize

	if len(buf) < totalSize {
		return 0, ErrBufferTooSmall
	}
	if payloadSize > MaxPayloadSize {
		return 0, ErrPayloadTooLarge
	}

	// 写入 header
	buf[0] = m.Type
	binary.BigEndian.PutUint32(buf[1:5], m.StreamID)
	binary.BigEndian.PutUint32(buf[5:9], uint32(payloadSize))

	// 写入 payload
	offset := HeaderSize
	switch m.Type {
	case MsgTypeData:
		// Data: 直接是 payload
		copy(buf[offset:], m.Payload)

	case MsgTypeConnect, MsgTypeCheckPort:
		// Connect/CheckPort: Target + RuleID + Payload(下一跳)
		offset += writeString(buf[offset:], m.Target)
		offset += writeString(buf[offset:], m.RuleID)
		if len(m.Payload) > 0 {
			copy(buf[offset:], m.Payload)
		}

	case MsgTypeError, MsgTypeCheckPortResult:
		// Error: Error string
		writeString(buf[offset:], m.Error)

	case MsgTypeConnAck, MsgTypeClose, MsgTypeRuleUpdate:
		// 无 payload
	}

	return totalSize, nil
}

// calcPayloadSize 计算 payload 大小
func (m *TunnelMessage) calcPayloadSize() int {
	switch m.Type {
	case MsgTypeData:
		return len(m.Payload)

	case MsgTypeConnect, MsgTypeCheckPort:
		// Target(2+len) + RuleID(2+len) + Payload
		return 2 + len(m.Target) + 2 + len(m.RuleID) + len(m.Payload)

	case MsgTypeError, MsgTypeCheckPortResult:
		return 2 + len(m.Error)

	default:
		return 0
	}
}

// UnmarshalBinary 二进制反序列化
func UnmarshalBinary(data []byte) (*TunnelMessage, error) {
	if len(data) < HeaderSize {
		return nil, ErrInvalidHeader
	}

	msg := &TunnelMessage{
		Type:     data[0],
		StreamID: binary.BigEndian.Uint32(data[1:5]),
	}

	payloadLen := binary.BigEndian.Uint32(data[5:9])
	if payloadLen > MaxPayloadSize {
		return nil, ErrPayloadTooLarge
	}

	if len(data) < HeaderSize+int(payloadLen) {
		return nil, ErrInvalidPayload
	}

	payload := data[HeaderSize : HeaderSize+int(payloadLen)]

	switch msg.Type {
	case MsgTypeData:
		// 直接引用，避免拷贝（调用方需要注意生命周期）
		msg.Payload = payload

	case MsgTypeConnect, MsgTypeCheckPort:
		offset := 0
		msg.Target, offset = readString(payload, offset)
		msg.RuleID, offset = readString(payload, offset)
		if offset < len(payload) {
			msg.Payload = payload[offset:]
		}

	case MsgTypeError, MsgTypeCheckPortResult:
		msg.Error, _ = readString(payload, 0)

	case MsgTypeConnAck, MsgTypeClose, MsgTypeRuleUpdate:
		// 无 payload
	}

	return msg, nil
}

// writeString 写入字符串 (2字节长度 + 数据)
func writeString(buf []byte, s string) int {
	l := len(s)
	if l > MaxStringLen {
		l = MaxStringLen
	}
	binary.BigEndian.PutUint16(buf[0:2], uint16(l))
	copy(buf[2:], s[:l])
	return 2 + l
}

// readString 读取字符串
func readString(data []byte, offset int) (string, int) {
	if offset+2 > len(data) {
		return "", offset
	}
	l := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+l > len(data) {
		l = len(data) - offset
	}
	return string(data[offset : offset+l]), offset + l
}

// ========== 兼容旧接口 (逐步废弃) ==========

// Marshal 兼容旧接口，内部使用二进制
func (m *TunnelMessage) Marshal() ([]byte, error) {
	buf, n, err := m.MarshalBinary()
	if err != nil {
		return nil, err
	}
	// 需要拷贝出来，因为调用方不知道要归还 buffer
	result := make([]byte, n)
	copy(result, (*buf)[:n])
	PutBuffer(buf)
	return result, nil
}

// UnmarshalTunnelMessage 兼容旧接口
func UnmarshalTunnelMessage(data []byte) (*TunnelMessage, error) {
	return UnmarshalBinary(data)
}

// ========== Stream 管理 ==========

// StreamManager 管理隧道中的多路复用流
type StreamManager struct {
	streams map[uint32]*Stream
	mu      sync.RWMutex
	nextID  uint32
}

// Stream 表示一个多路复用流
type Stream struct {
	ID      uint32
	Target  string
	DataCh  chan []byte
	CloseCh chan struct{}
	closed  int32
}

// NewStreamManager 创建流管理器
func NewStreamManager() *StreamManager {
	return &StreamManager{
		streams: make(map[uint32]*Stream),
		nextID:  1,
	}
}

// NewStream 创建新流
func (sm *StreamManager) NewStream(target string) *Stream {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	id := atomic.AddUint32(&sm.nextID, 1)
	stream := &Stream{
		ID:      id,
		Target:  target,
		DataCh:  make(chan []byte, 256), // 增大 channel 缓冲
		CloseCh: make(chan struct{}),
	}
	sm.streams[id] = stream
	return stream
}

// GetStream 获取流
func (sm *StreamManager) GetStream(id uint32) *Stream {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.streams[id]
}

// AddStream 添加流
func (sm *StreamManager) AddStream(stream *Stream) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.streams[stream.ID] = stream
}

// RemoveStream 移除流
func (sm *StreamManager) RemoveStream(id uint32) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if stream, ok := sm.streams[id]; ok {
		stream.Close()
		delete(sm.streams, id)
	}
}

// CloseAll 关闭所有流
func (sm *StreamManager) CloseAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for _, stream := range sm.streams {
		stream.Close()
	}
	sm.streams = make(map[uint32]*Stream)
}

// Close 关闭流
func (s *Stream) Close() {
	if atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		close(s.CloseCh)
	}
}

// IsClosed 检查流是否已关闭
func (s *Stream) IsClosed() bool {
	return atomic.LoadInt32(&s.closed) == 1
}

// Write 写入数据到流
func (s *Stream) Write(data []byte) bool {
	if s.IsClosed() {
		return false
	}
	select {
	case s.DataCh <- data:
		return true
	case <-s.CloseCh:
		return false
	default:
		// channel 满了，尝试非阻塞写入
		select {
		case s.DataCh <- data:
			return true
		case <-s.CloseCh:
			return false
		}
	}
}
