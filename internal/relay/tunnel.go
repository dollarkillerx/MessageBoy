package relay

import (
	"encoding/json"
	"sync"
	"sync/atomic"
)

// 消息类型
const (
	MsgTypeConnect    byte = 0x01
	MsgTypeConnAck    byte = 0x02
	MsgTypeData       byte = 0x03
	MsgTypeClose      byte = 0x04
	MsgTypeError      byte = 0x05
	MsgTypeRuleUpdate byte = 0x06 // 规则更新通知
)

type TunnelMessage struct {
	Type     byte   `json:"type"`
	StreamID uint32 `json:"stream_id"`
	Target   string `json:"target,omitempty"`
	Payload  []byte `json:"payload,omitempty"`
	Nonce    []byte `json:"nonce,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (m *TunnelMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

func UnmarshalTunnelMessage(data []byte) (*TunnelMessage, error) {
	var msg TunnelMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// StreamManager 管理隧道中的多路复用流
type StreamManager struct {
	streams   map[uint32]*Stream
	mu        sync.RWMutex
	nextID    uint32
}

type Stream struct {
	ID       uint32
	Target   string
	DataCh   chan []byte
	CloseCh  chan struct{}
	closed   int32
}

func NewStreamManager() *StreamManager {
	return &StreamManager{
		streams: make(map[uint32]*Stream),
		nextID:  1,
	}
}

func (sm *StreamManager) NewStream(target string) *Stream {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	id := atomic.AddUint32(&sm.nextID, 1)
	stream := &Stream{
		ID:      id,
		Target:  target,
		DataCh:  make(chan []byte, 100),
		CloseCh: make(chan struct{}),
	}
	sm.streams[id] = stream
	return stream
}

func (sm *StreamManager) GetStream(id uint32) *Stream {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.streams[id]
}

func (sm *StreamManager) AddStream(stream *Stream) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.streams[stream.ID] = stream
}

func (sm *StreamManager) RemoveStream(id uint32) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if stream, ok := sm.streams[id]; ok {
		stream.Close()
		delete(sm.streams, id)
	}
}

func (sm *StreamManager) CloseAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for _, stream := range sm.streams {
		stream.Close()
	}
	sm.streams = make(map[uint32]*Stream)
}

func (s *Stream) Close() {
	if atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		close(s.CloseCh)
	}
}

func (s *Stream) IsClosed() bool {
	return atomic.LoadInt32(&s.closed) == 1
}

func (s *Stream) Write(data []byte) bool {
	if s.IsClosed() {
		return false
	}
	select {
	case s.DataCh <- data:
		return true
	case <-s.CloseCh:
		return false
	}
}
