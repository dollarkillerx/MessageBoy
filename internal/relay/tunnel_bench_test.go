package relay

import (
	"sync"
	"testing"
)

// BenchmarkBufferPool_Small tests small buffer pool get/put
func BenchmarkBufferPool_Small(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := GetBufferForSize(100) // small payload
			PutBuffer(buf)
		}
	})
}

// BenchmarkBufferPool_Medium tests medium buffer pool get/put
func BenchmarkBufferPool_Medium(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := GetBufferForSize(8 * 1024) // 8KB payload
			PutBuffer(buf)
		}
	})
}

// BenchmarkBufferPool_Large tests large buffer pool get/put
func BenchmarkBufferPool_Large(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := GetBufferForSize(32 * 1024) // 32KB payload
			PutBuffer(buf)
		}
	})
}

// BenchmarkBufferPool_Mixed tests mixed-size buffer pool under contention
func BenchmarkBufferPool_Mixed(b *testing.B) {
	sizes := []int{100, 4000, 8000, 16000, 32000, 60000}
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			buf := GetBufferForSize(sizes[i%len(sizes)])
			PutBuffer(buf)
			i++
		}
	})
}

// BenchmarkMarshalData benchmarks marshaling data messages of various sizes
func BenchmarkMarshalData_64B(b *testing.B) {
	benchmarkMarshalData(b, 64)
}

func BenchmarkMarshalData_1KB(b *testing.B) {
	benchmarkMarshalData(b, 1024)
}

func BenchmarkMarshalData_32KB(b *testing.B) {
	benchmarkMarshalData(b, 32*1024)
}

func benchmarkMarshalData(b *testing.B, size int) {
	payload := make([]byte, size)
	msg := &TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 12345,
		Payload:  payload,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, _, err := msg.MarshalBinary()
		if err != nil {
			b.Fatal(err)
		}
		PutBuffer(buf)
	}
}

// BenchmarkUnmarshalData benchmarks unmarshaling data messages
func BenchmarkUnmarshalData_64B(b *testing.B) {
	benchmarkUnmarshalData(b, 64)
}

func BenchmarkUnmarshalData_1KB(b *testing.B) {
	benchmarkUnmarshalData(b, 1024)
}

func BenchmarkUnmarshalData_32KB(b *testing.B) {
	benchmarkUnmarshalData(b, 32*1024)
}

func benchmarkUnmarshalData(b *testing.B, size int) {
	payload := make([]byte, size)
	msg := &TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 12345,
		Payload:  payload,
	}
	data, _ := msg.Marshal()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := UnmarshalBinary(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMarshalConnect benchmarks connect message marshaling
func BenchmarkMarshalConnect(b *testing.B) {
	msg := &TunnelMessage{
		Type:     MsgTypeConnect,
		StreamID: 12345,
		Target:   "192.168.1.100:8080",
		RuleID:   "rule-abc-123",
		Payload:  []byte("client-xyz"),
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, _, err := msg.MarshalBinary()
		if err != nil {
			b.Fatal(err)
		}
		PutBuffer(buf)
	}
}

// BenchmarkStreamWrite benchmarks stream write throughput
func BenchmarkStreamWrite(b *testing.B) {
	sm := NewStreamManager()
	stream := sm.NewStream("target")
	data := make([]byte, 1024)

	// Drain goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case _, ok := <-stream.DataCh:
				if !ok {
					return
				}
			}
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream.Write(data)
	}
	b.StopTimer()

	stream.Close()
	<-done
}

// BenchmarkStreamWrite_Parallel benchmarks concurrent stream writes
func BenchmarkStreamWrite_Parallel(b *testing.B) {
	sm := NewStreamManager()
	stream := sm.NewStream("target")
	data := make([]byte, 1024)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case _, ok := <-stream.DataCh:
				if !ok {
					return
				}
			}
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stream.Write(data)
		}
	})
	b.StopTimer()

	stream.Close()
	<-done
}

// BenchmarkMarshalUnmarshalRoundTrip benchmarks full roundtrip
func BenchmarkMarshalUnmarshalRoundTrip_1KB(b *testing.B) {
	payload := make([]byte, 1024)
	msg := &TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 12345,
		Payload:  payload,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		data, err := msg.Marshal()
		if err != nil {
			b.Fatal(err)
		}
		_, err = UnmarshalBinary(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkConcurrentBufferPool tests buffer pool under high contention
func BenchmarkConcurrentBufferPool(b *testing.B) {
	var wg sync.WaitGroup
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		wg.Add(1)
		defer wg.Done()
		for pb.Next() {
			buf := GetBuffer()
			// Simulate some work
			(*buf)[0] = 1
			PutBuffer(buf)
		}
	})

	wg.Wait()
}
