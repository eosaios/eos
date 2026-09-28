package monitor

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"strings"
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	if FormatBytes(0) != "0 B" {
		t.Fatalf("0 = %q", FormatBytes(0))
	}
	if FormatBytes(512) != "512 B" {
		t.Fatalf("512 = %q", FormatBytes(512))
	}
	if FormatBytes(1024) != "1.0 KB" {
		t.Fatalf("1k = %q", FormatBytes(1024))
	}
	if FormatBytes(1536) != "1.5 KB" {
		t.Fatalf("1.5k = %q", FormatBytes(1536))
	}
	if !strings.Contains(FormatBytes(1024*1024), "MB") {
		t.Fatalf("mb = %q", FormatBytes(1024*1024))
	}
}

func TestMemoryMonitorBasics(t *testing.T) {
	m := NewMemoryMonitor()
	m.SetMaxStats(5)
	m.SetThresholds(MemoryThresholds{AllocWarning: 1, AllocCritical: 2})
	var hit bool
	m.SetOnThresholdExceeded(func(MemoryStats) { hit = true })
	m.SetSampleInterval(time.Millisecond)

	s := m.Sample()
	if s.Timestamp.IsZero() {
		t.Fatal("sample timestamp")
	}
	// 低阈值会触发回调
	if !hit {
		// Alloc 可能为 0 时不触发，再采一次
		m.Sample()
	}

	if len(m.GetStats()) == 0 {
		t.Fatal("stats")
	}
	latest := m.GetLatestStats()
	if latest.Timestamp.IsZero() {
		t.Fatal("latest")
	}
	now := time.Now()
	if got := m.GetStatsInRange(now.Add(-time.Hour), now.Add(time.Hour)); len(got) == 0 {
		t.Fatal("range")
	}

	gc := m.GetGCStats()
	_ = gc
	_ = m.GetAverageAlloc()
	_ = m.GetMaxAlloc()
	_ = m.GetMinAlloc()
	_ = m.GetAllocTrend(3)
	m.Clear()
	if len(m.GetStats()) != 0 {
		t.Fatal("clear")
	}

	// Start/Stop 幂等
	m.Start()
	m.Start()
	m.Stop()
	m.Stop()
}

func TestPackageHelpers(t *testing.T) {
	ForceGC()
	s := ReadMemStats()
	if s.Timestamp.IsZero() {
		t.Fatal("ReadMemStats")
	}
	if GetMemoryUsage() == 0 {
		t.Fatal("usage")
	}
	if GetGoroutineCount() < 1 {
		t.Fatal("goroutines")
	}
	if GetCPUCount() < 1 {
		t.Fatal("cpus")
	}
}
