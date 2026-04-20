package client

import (
	"io"
	"net"
	"testing"
	"time"
)

// BenchmarkDirectCopy benchmarks raw io.Copy (baseline, no relay overhead)
func BenchmarkDirectCopy(b *testing.B) {
	// Start an echo server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go io.Copy(conn, conn)
		}
	}()

	data := make([]byte, 1024)
	buf := make([]byte, 1024)

	b.ResetTimer()
	b.SetBytes(1024)

	for i := 0; i < b.N; i++ {
		conn, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
		if err != nil {
			b.Fatal(err)
		}
		conn.Write(data)
		io.ReadFull(conn, buf)
		conn.Close()
	}
}

// BenchmarkDirectCopy_Sustained benchmarks sustained throughput via direct copy
func BenchmarkDirectCopy_Sustained(b *testing.B) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go io.Copy(conn, conn)
		}
	}()

	conn, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	data := make([]byte, 1024)
	buf := make([]byte, 1024)

	b.ResetTimer()
	b.SetBytes(1024)

	for i := 0; i < b.N; i++ {
		if _, err := conn.Write(data); err != nil {
			b.Fatal(err)
		}
		if _, err := io.ReadFull(conn, buf); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCountingCopy benchmarks io.Copy with counting wrapper
func BenchmarkCountingCopy_Sustained(b *testing.B) {
	tc := NewTrafficCounter()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go io.Copy(conn, conn)
		}
	}()

	conn, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	data := make([]byte, 1024)
	buf := make([]byte, 1024)

	cr := NewCountingReader(conn, tc, "rule1", true)

	b.ResetTimer()
	b.SetBytes(1024)

	for i := 0; i < b.N; i++ {
		if _, err := conn.Write(data); err != nil {
			b.Fatal(err)
		}
		if _, err := io.ReadFull(cr, buf); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCopyAndCount 对比 copyAndCount 和 io.Copy+CountingReader，不经过 socket
// 使用 net.Pipe 构造内存管道，排除网络因素，只测转发热路径的 CPU/分配开销。
func BenchmarkCopyAndCount(b *testing.B) {
	payload := make([]byte, 32*1024)

	b.Run("io.Copy+CountingReader/new", func(b *testing.B) {
		tc := NewTrafficCounter()
		b.ResetTimer()
		b.SetBytes(int64(len(payload)))
		for i := 0; i < b.N; i++ {
			r, w := net.Pipe()
			go func() {
				w.Write(payload)
				w.Close()
			}()
			cr := NewCountingReader(r, tc, "rule-a", true)
			io.Copy(io.Discard, cr)
			r.Close()
		}
	})

	b.Run("copyAndCount/stat-cached", func(b *testing.B) {
		tc := NewTrafficCounter()
		stat := tc.GetOrCreateStat("rule-a")
		b.ResetTimer()
		b.SetBytes(int64(len(payload)))
		for i := 0; i < b.N; i++ {
			r, w := net.Pipe()
			go func() {
				w.Write(payload)
				w.Close()
			}()
			copyAndCount(io.Discard, r, stat, true)
			r.Close()
		}
	})

	b.Run("copyAndCount/nil-stat (splice-eligible)", func(b *testing.B) {
		b.ResetTimer()
		b.SetBytes(int64(len(payload)))
		for i := 0; i < b.N; i++ {
			r, w := net.Pipe()
			go func() {
				w.Write(payload)
				w.Close()
			}()
			copyAndCount(io.Discard, r, nil, true)
			r.Close()
		}
	})
}
