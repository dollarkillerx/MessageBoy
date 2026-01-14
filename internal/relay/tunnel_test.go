package relay

import (
	"encoding/json"
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
				Nonce:    []byte("123456789012"),
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
		})
	}
}

func TestUnmarshalInvalidJSON(t *testing.T) {
	_, err := UnmarshalTunnelMessage([]byte("invalid json"))
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
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
}

func BenchmarkTunnelMessageMarshal(b *testing.B) {
	msg := TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 12345,
		Payload:  make([]byte, 1024),
		Nonce:    make([]byte, 12),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.Marshal()
	}
}

func BenchmarkTunnelMessageUnmarshal(b *testing.B) {
	msg := TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 12345,
		Payload:  make([]byte, 1024),
		Nonce:    make([]byte, 12),
	}
	data, _ := json.Marshal(msg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		UnmarshalTunnelMessage(data)
	}
}
