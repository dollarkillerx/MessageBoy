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
