package relay

import (
	"sync"
	"testing"
	"time"
)

func TestTunnelMessageMarshal(t *testing.T) {
	testCases := []struct {
		name string
		msg  TunnelMessage
	}{
		{
			name: "connect",
			msg: TunnelMessage{
				Type:     MsgTypeConnect,
				StreamID: 12345,
				Target:   "192.168.1.1:80",
			},
		},
		{
			name: "connect_with_ruleid",
			msg: TunnelMessage{
				Type:     MsgTypeConnect,
				StreamID: 12345,
				Target:   "192.168.1.1:80",
				RuleID:   "rule-123",
				Payload:  []byte("next-hop-client"),
			},
		},
		{
			name: "connack",
			msg: TunnelMessage{
				Type:     MsgTypeConnAck,
				StreamID: 12345,
			},
		},
		{
			name: "data",
			msg: TunnelMessage{
				Type:     MsgTypeData,
				StreamID: 12345,
				Payload:  []byte("hello world"),
			},
		},
		{
			name: "data_large",
			msg: TunnelMessage{
				Type:     MsgTypeData,
				StreamID: 12345,
				Payload:  make([]byte, 32*1024), // 32KB
			},
		},
		{
			name: "close",
			msg: TunnelMessage{
				Type:     MsgTypeClose,
				StreamID: 12345,
			},
		},
		{
			name: "error",
			msg: TunnelMessage{
				Type:     MsgTypeError,
				StreamID: 12345,
				Error:    "connection refused",
			},
		},
		{
			name: "rule_update",
			msg: TunnelMessage{
				Type: MsgTypeRuleUpdate,
			},
		},
		{
			name: "check_port",
			msg: TunnelMessage{
				Type:     MsgTypeCheckPort,
				StreamID: 1,
				Target:   "0.0.0.0:8080",
				RuleID:   "rule-456",
			},
		},
		{
			name: "check_port_result_ok",
			msg: TunnelMessage{
				Type:     MsgTypeCheckPortResult,
				StreamID: 1,
				Error:    "",
			},
		},
		{
			name: "check_port_result_error",
			msg: TunnelMessage{
				Type:     MsgTypeCheckPortResult,
				StreamID: 1,
				Error:    "port already in use",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := tc.msg.Marshal()
			if err != nil {
				t.Fatalf("Marshal() error: %v", err)
			}

			unmarshaled, err := UnmarshalTunnelMessage(data)
			if err != nil {
				t.Fatalf("UnmarshalTunnelMessage() error: %v", err)
			}

			if unmarshaled.Type != tc.msg.Type {
				t.Errorf("Type mismatch: got %v, want %v", unmarshaled.Type, tc.msg.Type)
			}
			if unmarshaled.StreamID != tc.msg.StreamID {
				t.Errorf("StreamID mismatch: got %v, want %v", unmarshaled.StreamID, tc.msg.StreamID)
			}
			if unmarshaled.Target != tc.msg.Target {
				t.Errorf("Target mismatch: got %v, want %v", unmarshaled.Target, tc.msg.Target)
			}
			if unmarshaled.Error != tc.msg.Error {
				t.Errorf("Error mismatch: got %v, want %v", unmarshaled.Error, tc.msg.Error)
			}
			if unmarshaled.RuleID != tc.msg.RuleID {
				t.Errorf("RuleID mismatch: got %v, want %v", unmarshaled.RuleID, tc.msg.RuleID)
			}
			// Payload 比较
			if len(unmarshaled.Payload) != len(tc.msg.Payload) {
				t.Errorf("Payload length mismatch: got %v, want %v", len(unmarshaled.Payload), len(tc.msg.Payload))
			}
		})
	}
}

func TestUnmarshalInvalidData(t *testing.T) {
	// 数据太短
	_, err := UnmarshalTunnelMessage([]byte{0x01, 0x02})
	if err == nil {
		t.Error("Expected error for data too short")
	}

	// 空数据
	_, err = UnmarshalTunnelMessage([]byte{})
	if err == nil {
		t.Error("Expected error for empty data")
	}
}

func TestMarshalBinary(t *testing.T) {
	msg := &TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 12345,
		Payload:  []byte("test payload"),
	}

	buf, size, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}
	defer PutBuffer(buf)

	if size != HeaderSize+len(msg.Payload) {
		t.Errorf("Size mismatch: got %v, want %v", size, HeaderSize+len(msg.Payload))
	}

	// 验证 header
	if (*buf)[0] != MsgTypeData {
		t.Errorf("Type mismatch in buffer")
	}
}

func TestBufferPool(t *testing.T) {
	// 获取 buffer
	buf1 := GetBuffer()
	if buf1 == nil {
		t.Fatal("GetBuffer() returned nil")
	}
	if len(*buf1) < DefaultBufSize {
		t.Errorf("Buffer too small: got %v, want at least %v", len(*buf1), DefaultBufSize)
	}

	// 归还并重新获取
	PutBuffer(buf1)
	buf2 := GetBuffer()
	if buf2 == nil {
		t.Fatal("GetBuffer() returned nil after put")
	}

	PutBuffer(buf2)
}

func TestStreamManager(t *testing.T) {
	sm := NewStreamManager()

	// 创建新流
	stream1 := sm.NewStream("target1")
	if stream1 == nil {
		t.Fatal("NewStream() returned nil")
	}
	if stream1.Target != "target1" {
		t.Errorf("Target mismatch: got %v, want target1", stream1.Target)
	}

	stream2 := sm.NewStream("target2")
	if stream1.ID == stream2.ID {
		t.Error("Stream IDs should be unique")
	}

	// 获取流
	retrieved := sm.GetStream(stream1.ID)
	if retrieved == nil {
		t.Error("GetStream() returned nil for existing stream")
	}
	if retrieved.ID != stream1.ID {
		t.Error("Retrieved stream ID mismatch")
	}

	// 获取不存在的流
	notFound := sm.GetStream(99999)
	if notFound != nil {
		t.Error("GetStream() should return nil for non-existent stream")
	}

	// 删除流
	sm.RemoveStream(stream1.ID)
	removed := sm.GetStream(stream1.ID)
	if removed != nil {
		t.Error("Stream should be removed")
	}

	// stream2 应该仍然存在
	if sm.GetStream(stream2.ID) == nil {
		t.Error("Other streams should not be affected")
	}
}

func TestStreamManagerAddStream(t *testing.T) {
	sm := NewStreamManager()

	// 手动添加流
	stream := &Stream{
		ID:      12345,
		Target:  "test-target",
		DataCh:  make(chan []byte, 100),
		CloseCh: make(chan struct{}),
	}

	sm.AddStream(stream)

	retrieved := sm.GetStream(12345)
	if retrieved == nil {
		t.Fatal("AddStream() failed to add stream")
	}
	if retrieved.Target != "test-target" {
		t.Error("Stream target mismatch")
	}
}

func TestStreamManagerCloseAll(t *testing.T) {
	sm := NewStreamManager()

	// 创建多个流
	stream1 := sm.NewStream("target1")
	stream2 := sm.NewStream("target2")
	stream3 := sm.NewStream("target3")

	// 关闭所有
	sm.CloseAll()

	// 所有流应该被关闭
	if !stream1.IsClosed() {
		t.Error("stream1 should be closed")
	}
	if !stream2.IsClosed() {
		t.Error("stream2 should be closed")
	}
	if !stream3.IsClosed() {
		t.Error("stream3 should be closed")
	}

	// 获取应该返回 nil
	if sm.GetStream(stream1.ID) != nil {
		t.Error("Streams should be cleared")
	}
}

func TestStreamWrite(t *testing.T) {
	stream := &Stream{
		ID:      1,
		Target:  "test",
		DataCh:  make(chan []byte, 10),
		CloseCh: make(chan struct{}),
	}

	// 写入数据
	data := []byte("test data")
	ok := stream.Write(data)
	if !ok {
		t.Error("Write() should return true")
	}

	// 读取数据
	select {
	case received := <-stream.DataCh:
		if string(received) != string(data) {
			t.Error("Data mismatch")
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for data")
	}

	// 关闭后写入应该失败
	stream.Close()
	ok = stream.Write([]byte("more data"))
	if ok {
		t.Error("Write() should return false after close")
	}
}

func TestStreamConcurrentAccess(t *testing.T) {
	sm := NewStreamManager()
	var wg sync.WaitGroup

	// 并发创建和删除流
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			stream := sm.NewStream("target")
			time.Sleep(time.Millisecond)
			sm.RemoveStream(stream.ID)
		}()
		go func() {
			defer wg.Done()
			stream := sm.NewStream("target2")
			sm.GetStream(stream.ID)
		}()
	}

	wg.Wait()
}

func TestMsgTypes(t *testing.T) {
	// 确保消息类型常量正确
	if MsgTypeConnect != 0x01 {
		t.Error("MsgTypeConnect should be 0x01")
	}
	if MsgTypeConnAck != 0x02 {
		t.Error("MsgTypeConnAck should be 0x02")
	}
	if MsgTypeData != 0x03 {
		t.Error("MsgTypeData should be 0x03")
	}
	if MsgTypeClose != 0x04 {
		t.Error("MsgTypeClose should be 0x04")
	}
	if MsgTypeError != 0x05 {
		t.Error("MsgTypeError should be 0x05")
	}
	if MsgTypeRuleUpdate != 0x06 {
		t.Error("MsgTypeRuleUpdate should be 0x06")
	}
	if MsgTypeCheckPort != 0x07 {
		t.Error("MsgTypeCheckPort should be 0x07")
	}
	if MsgTypeCheckPortResult != 0x08 {
		t.Error("MsgTypeCheckPortResult should be 0x08")
	}
}

// ============================================================
// Multi-level Buffer Pool (GetBufferForSize / PutBuffer)
// ============================================================

func TestGetBufferForSize_Small(t *testing.T) {
	buf := GetBufferForSize(100) // 100 bytes payload -> Small pool
	if buf == nil {
		t.Fatal("GetBufferForSize returned nil")
	}
	// Small pool returns SmallBufSize + HeaderSize
	if cap(*buf) < SmallBufSize+HeaderSize {
		t.Errorf("cap = %d, want >= %d", cap(*buf), SmallBufSize+HeaderSize)
	}
	PutBuffer(buf)
}

func TestGetBufferForSize_Medium(t *testing.T) {
	buf := GetBufferForSize(SmallBufSize + 1) // exceeds Small -> Medium pool
	if buf == nil {
		t.Fatal("GetBufferForSize returned nil")
	}
	if cap(*buf) < MediumBufSize+HeaderSize {
		t.Errorf("cap = %d, want >= %d", cap(*buf), MediumBufSize+HeaderSize)
	}
	PutBuffer(buf)
}

func TestGetBufferForSize_Large(t *testing.T) {
	buf := GetBufferForSize(MediumBufSize + 1) // exceeds Medium -> Large pool
	if buf == nil {
		t.Fatal("GetBufferForSize returned nil")
	}
	if cap(*buf) < LargeBufSize+HeaderSize {
		t.Errorf("cap = %d, want >= %d", cap(*buf), LargeBufSize+HeaderSize)
	}
	PutBuffer(buf)
}

func TestGetBufferForSize_ZeroPayload(t *testing.T) {
	buf := GetBufferForSize(0)
	if buf == nil {
		t.Fatal("GetBufferForSize(0) returned nil")
	}
	// Should use small pool
	if cap(*buf) < SmallBufSize+HeaderSize {
		t.Errorf("cap = %d, want >= %d", cap(*buf), SmallBufSize+HeaderSize)
	}
	PutBuffer(buf)
}

func TestGetBufferForSize_ExactBoundary(t *testing.T) {
	// Exactly SmallBufSize payload => total = SmallBufSize + HeaderSize => should fit in Small pool
	buf := GetBufferForSize(SmallBufSize)
	if buf == nil {
		t.Fatal("GetBufferForSize returned nil")
	}
	if cap(*buf) < SmallBufSize+HeaderSize {
		t.Errorf("cap = %d, want >= %d", cap(*buf), SmallBufSize+HeaderSize)
	}
	PutBuffer(buf)
}

func TestPutBuffer_Nil(t *testing.T) {
	// Should not panic
	PutBuffer(nil)
}

func TestPutBuffer_TooSmall(t *testing.T) {
	// A buffer smaller than SmallBufSize+HeaderSize should be silently discarded
	buf := make([]byte, 10)
	PutBuffer(&buf) // should not panic, just not returned to any pool
}

func TestPutBuffer_CorrectPoolRouting(t *testing.T) {
	// Get from each pool, put back, get again — verify sizes are consistent
	tests := []struct {
		name        string
		payloadSize int
		minCap      int
	}{
		{"small", 100, SmallBufSize + HeaderSize},
		{"medium", SmallBufSize + 1, MediumBufSize + HeaderSize},
		{"large", MediumBufSize + 1, LargeBufSize + HeaderSize},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := GetBufferForSize(tc.payloadSize)
			if cap(*buf) < tc.minCap {
				t.Errorf("cap = %d, want >= %d", cap(*buf), tc.minCap)
			}
			PutBuffer(buf)

			// Get again — should still be correct size
			buf2 := GetBufferForSize(tc.payloadSize)
			if cap(*buf2) < tc.minCap {
				t.Errorf("after put/get: cap = %d, want >= %d", cap(*buf2), tc.minCap)
			}
			PutBuffer(buf2)
		})
	}
}

func TestMarshalBinary_UsesCorrectPool(t *testing.T) {
	tests := []struct {
		name        string
		payloadSize int
		minCap      int
	}{
		{"small_payload", 100, SmallBufSize + HeaderSize},
		{"medium_payload", SmallBufSize + 1, MediumBufSize + HeaderSize},
		{"large_payload", MediumBufSize + 1, LargeBufSize + HeaderSize},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := &TunnelMessage{
				Type:     MsgTypeData,
				StreamID: 1,
				Payload:  make([]byte, tc.payloadSize),
			}
			buf, _, err := msg.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary: %v", err)
			}
			if cap(*buf) < tc.minCap {
				t.Errorf("MarshalBinary returned cap = %d, want >= %d", cap(*buf), tc.minCap)
			}
			PutBuffer(buf)
		})
	}
}

// ============================================================
// Stream Drop Counting
// ============================================================

func TestStream_DroppedMessages_InitiallyZero(t *testing.T) {
	s := &Stream{
		ID:      1,
		DataCh:  make(chan []byte, 10),
		CloseCh: make(chan struct{}),
	}
	if s.DroppedMessages() != 0 {
		t.Errorf("initial DroppedMessages = %d, want 0", s.DroppedMessages())
	}
}

func TestStream_Write_DropsWhenFull(t *testing.T) {
	s := &Stream{
		ID:      1,
		DataCh:  make(chan []byte, 2),
		CloseCh: make(chan struct{}),
	}

	// Fill the channel
	s.Write([]byte("1"))
	s.Write([]byte("2"))

	// Third write should be dropped
	ok := s.Write([]byte("3"))
	if ok {
		t.Error("Write should return false when channel is full")
	}

	if s.DroppedMessages() != 1 {
		t.Errorf("DroppedMessages = %d, want 1", s.DroppedMessages())
	}

	// Drop a few more
	s.Write([]byte("4"))
	s.Write([]byte("5"))

	if s.DroppedMessages() != 3 {
		t.Errorf("DroppedMessages = %d, want 3", s.DroppedMessages())
	}
}

func TestStream_Write_DropsConcurrent(t *testing.T) {
	s := &Stream{
		ID:      1,
		DataCh:  make(chan []byte, 1),
		CloseCh: make(chan struct{}),
	}

	// Fill channel
	s.Write([]byte("fill"))

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Write([]byte("data"))
		}()
	}
	wg.Wait()

	dropped := s.DroppedMessages()
	if dropped != 100 {
		t.Errorf("DroppedMessages = %d, want 100", dropped)
	}
}

func TestStream_Write_ClosedChannelDrop(t *testing.T) {
	s := &Stream{
		ID:      1,
		DataCh:  make(chan []byte, 10),
		CloseCh: make(chan struct{}),
	}

	s.Close()

	ok := s.Write([]byte("after close"))
	if ok {
		t.Error("Write should return false after Close")
	}

	// Drop should NOT be counted for closed stream (returns early via IsClosed check)
	if s.DroppedMessages() != 0 {
		t.Errorf("DroppedMessages = %d, want 0 (closed stream should not count drops)", s.DroppedMessages())
	}
}

// ===== Benchmarks =====

func BenchmarkTunnelMessageMarshal(b *testing.B) {
	msg := TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 12345,
		Payload:  make([]byte, 1024),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.Marshal()
	}
}

func BenchmarkTunnelMessageMarshalBinary(b *testing.B) {
	msg := TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 12345,
		Payload:  make([]byte, 1024),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, _, _ := msg.MarshalBinary()
		PutBuffer(buf)
	}
}

func BenchmarkTunnelMessageMarshalTo(b *testing.B) {
	msg := TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 12345,
		Payload:  make([]byte, 1024),
	}
	buf := make([]byte, 2048)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.MarshalTo(buf)
	}
}

func BenchmarkTunnelMessageUnmarshal(b *testing.B) {
	msg := TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 12345,
		Payload:  make([]byte, 1024),
	}
	data, _ := msg.Marshal()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		UnmarshalTunnelMessage(data)
	}
}

func BenchmarkBufferPool(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := GetBuffer()
		PutBuffer(buf)
	}
}
