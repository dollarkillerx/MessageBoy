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
