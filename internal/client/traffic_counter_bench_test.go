package client

import (
	"fmt"
	"sync"
	"testing"
)

// BenchmarkTrafficCounter_AddBytes benchmarks single-rule byte counting
func BenchmarkTrafficCounter_AddBytes(b *testing.B) {
	tc := NewTrafficCounter()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.AddBytesIn("rule1", 1024)
	}
}

// BenchmarkTrafficCounter_Parallel benchmarks concurrent byte counting
func BenchmarkTrafficCounter_Parallel(b *testing.B) {
	tc := NewTrafficCounter()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tc.AddBytesIn("rule1", 1024)
			tc.AddBytesOut("rule1", 512)
		}
	})
}

// BenchmarkTrafficCounter_MultiRule benchmarks concurrent multi-rule counting
func BenchmarkTrafficCounter_MultiRule(b *testing.B) {
	tc := NewTrafficCounter()
	rules := make([]string, 100)
	for i := range rules {
		rules[i] = fmt.Sprintf("rule-%d", i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ruleID := rules[i%len(rules)]
			tc.AddBytesIn(ruleID, 1024)
			tc.AddBytesOut(ruleID, 512)
			i++
		}
	})
}

// BenchmarkTrafficCounter_MixedReadWrite benchmarks concurrent counting + periodic GetAndReset
func BenchmarkTrafficCounter_MixedReadWrite(b *testing.B) {
	tc := NewTrafficCounter()
	rules := make([]string, 50)
	for i := range rules {
		rules[i] = fmt.Sprintf("rule-%d", i)
	}

	// Pre-populate
	for _, r := range rules {
		tc.AddBytesIn(r, 100)
	}

	b.ResetTimer()

	var wg sync.WaitGroup

	// Writers
	b.RunParallel(func(pb *testing.PB) {
		wg.Add(1)
		defer wg.Done()
		i := 0
		for pb.Next() {
			ruleID := rules[i%len(rules)]
			tc.AddBytesIn(ruleID, 1024)
			tc.AddBytesOut(ruleID, 512)

			// Periodic reset (every 1000 ops)
			if i%1000 == 0 {
				tc.GetAndReset()
			}
			i++
		}
	})

	wg.Wait()
}

// BenchmarkTrafficCounter_GetAndReset benchmarks the reset path
func BenchmarkTrafficCounter_GetAndReset(b *testing.B) {
	tc := NewTrafficCounter()
	for i := 0; i < 100; i++ {
		tc.AddBytesIn(fmt.Sprintf("rule-%d", i), int64(i*1000))
		tc.AddBytesOut(fmt.Sprintf("rule-%d", i), int64(i*500))
		tc.IncrementConn(fmt.Sprintf("rule-%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.GetAndReset()
	}
}

// BenchmarkTrafficCounter_IncrementDecrement benchmarks connection counting
func BenchmarkTrafficCounter_IncrementDecrement(b *testing.B) {
	tc := NewTrafficCounter()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tc.IncrementConn("rule1")
			tc.DecrementConn("rule1")
		}
	})
}

// BenchmarkCountingReader benchmarks the counting reader wrapper
func BenchmarkCountingReader(b *testing.B) {
	tc := NewTrafficCounter()
	data := make([]byte, 4096)
	reader := &repeatReader{data: data}
	cr := NewCountingReader(reader, tc, "rule1", true)
	buf := make([]byte, 4096)

	b.ResetTimer()
	b.SetBytes(4096)
	for i := 0; i < b.N; i++ {
		cr.Read(buf)
	}
}

// repeatReader always returns the same data
type repeatReader struct {
	data []byte
}

func (r *repeatReader) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	return n, nil
}
