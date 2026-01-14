package client

import (
	"bytes"
	"io"
	"sync"
	"testing"
)

func TestTrafficCounter_AddBytes(t *testing.T) {
	tc := NewTrafficCounter()

	// 测试添加入站流量
	tc.AddBytesIn("rule1", 100)
	tc.AddBytesIn("rule1", 200)
	tc.AddBytesOut("rule1", 50)

	// 获取并重置
	reports := tc.GetAndReset()

	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}

	report := reports[0]
	if report.RuleID != "rule1" {
		t.Errorf("expected rule_id 'rule1', got '%s'", report.RuleID)
	}
	if report.BytesIn != 300 {
		t.Errorf("expected bytes_in 300, got %d", report.BytesIn)
	}
	if report.BytesOut != 50 {
		t.Errorf("expected bytes_out 50, got %d", report.BytesOut)
	}
}

func TestTrafficCounter_Connections(t *testing.T) {
	tc := NewTrafficCounter()

	// 测试连接计数
	tc.IncrementConn("rule1")
	tc.IncrementConn("rule1")
	tc.IncrementConn("rule1")
	tc.DecrementConn("rule1") // 一个连接关闭

	reports := tc.GetAndReset()

	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}

	if reports[0].Connections != 3 {
		t.Errorf("expected 3 connections, got %d", reports[0].Connections)
	}
}

func TestTrafficCounter_MultipleRules(t *testing.T) {
	tc := NewTrafficCounter()

	tc.AddBytesIn("rule1", 100)
	tc.AddBytesIn("rule2", 200)
	tc.AddBytesOut("rule1", 50)
	tc.IncrementConn("rule1")
	tc.IncrementConn("rule2")

	reports := tc.GetAndReset()

	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}

	// 验证每个规则的统计
	ruleStats := make(map[string]TrafficReport)
	for _, r := range reports {
		ruleStats[r.RuleID] = r
	}

	if ruleStats["rule1"].BytesIn != 100 {
		t.Errorf("rule1 bytes_in: expected 100, got %d", ruleStats["rule1"].BytesIn)
	}
	if ruleStats["rule1"].BytesOut != 50 {
		t.Errorf("rule1 bytes_out: expected 50, got %d", ruleStats["rule1"].BytesOut)
	}
	if ruleStats["rule2"].BytesIn != 200 {
		t.Errorf("rule2 bytes_in: expected 200, got %d", ruleStats["rule2"].BytesIn)
	}
}

func TestTrafficCounter_GetAndReset(t *testing.T) {
	tc := NewTrafficCounter()

	tc.AddBytesIn("rule1", 100)

	// 第一次获取
	reports1 := tc.GetAndReset()
	if len(reports1) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports1))
	}
	if reports1[0].BytesIn != 100 {
		t.Errorf("expected 100, got %d", reports1[0].BytesIn)
	}

	// 第二次获取应该为空（已重置）
	reports2 := tc.GetAndReset()
	if len(reports2) != 0 {
		t.Errorf("expected 0 reports after reset, got %d", len(reports2))
	}
}

func TestTrafficCounter_Concurrent(t *testing.T) {
	tc := NewTrafficCounter()
	var wg sync.WaitGroup

	// 并发添加流量
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tc.AddBytesIn("rule1", 10)
			tc.AddBytesOut("rule1", 5)
			tc.IncrementConn("rule1")
		}()
	}

	wg.Wait()

	reports := tc.GetAndReset()
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}

	if reports[0].BytesIn != 1000 {
		t.Errorf("expected bytes_in 1000, got %d", reports[0].BytesIn)
	}
	if reports[0].BytesOut != 500 {
		t.Errorf("expected bytes_out 500, got %d", reports[0].BytesOut)
	}
	if reports[0].Connections != 100 {
		t.Errorf("expected 100 connections, got %d", reports[0].Connections)
	}
}

func TestCountingReader(t *testing.T) {
	tc := NewTrafficCounter()
	data := []byte("hello world")
	reader := bytes.NewReader(data)

	countingReader := NewCountingReader(reader, tc, "rule1", true)

	// 读取数据
	buf := make([]byte, 5)
	n, err := countingReader.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected to read 5 bytes, got %d", n)
	}

	// 再读取剩余数据
	buf2 := make([]byte, 10)
	n2, err := countingReader.Read(buf2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n2 != 6 {
		t.Errorf("expected to read 6 bytes, got %d", n2)
	}

	// 验证统计
	reports := tc.GetAndReset()
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].BytesIn != 11 {
		t.Errorf("expected bytes_in 11, got %d", reports[0].BytesIn)
	}
}

func TestCountingReader_EOF(t *testing.T) {
	tc := NewTrafficCounter()
	data := []byte("test")
	reader := bytes.NewReader(data)

	countingReader := NewCountingReader(reader, tc, "rule1", false)

	// 读取全部数据
	result, err := io.ReadAll(countingReader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != "test" {
		t.Errorf("expected 'test', got '%s'", string(result))
	}

	// 验证统计 (isIn=false 表示出站)
	reports := tc.GetAndReset()
	if reports[0].BytesOut != 4 {
		t.Errorf("expected bytes_out 4, got %d", reports[0].BytesOut)
	}
}

func TestCountingWriter(t *testing.T) {
	tc := NewTrafficCounter()
	var buf bytes.Buffer

	countingWriter := NewCountingWriter(&buf, tc, "rule1", true)

	// 写入数据
	n, err := countingWriter.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected to write 5 bytes, got %d", n)
	}

	n, err = countingWriter.Write([]byte(" world"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 6 {
		t.Errorf("expected to write 6 bytes, got %d", n)
	}

	// 验证写入内容
	if buf.String() != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", buf.String())
	}

	// 验证统计
	reports := tc.GetAndReset()
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].BytesIn != 11 {
		t.Errorf("expected bytes_in 11, got %d", reports[0].BytesIn)
	}
}

func TestTrafficCounter_EmptyReport(t *testing.T) {
	tc := NewTrafficCounter()

	// 没有任何流量时应该返回空
	reports := tc.GetAndReset()
	if len(reports) != 0 {
		t.Errorf("expected 0 reports, got %d", len(reports))
	}
}

func TestTrafficCounter_ZeroTrafficNotReported(t *testing.T) {
	tc := NewTrafficCounter()

	// 只增加连接但没有流量
	tc.IncrementConn("rule1")
	tc.DecrementConn("rule1")

	// 应该报告连接数
	reports := tc.GetAndReset()
	if len(reports) != 1 {
		t.Fatalf("expected 1 report for connections, got %d", len(reports))
	}
	if reports[0].Connections != 1 {
		t.Errorf("expected 1 connection, got %d", reports[0].Connections)
	}
}
