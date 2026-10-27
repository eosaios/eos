package monitor

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

// MemoryStats 内存统计信息
type MemoryStats struct {
	Timestamp    time.Time
	Alloc        uint64        // 当前分配的内存（字节）
	TotalAlloc   uint64        // 累计分配的内存（字节）
	Sys          uint64        // 从系统获取的内存（字节）
	HeapAlloc    uint64        // 堆分配的内存（字节）
	HeapSys      uint64        // 堆从系统获取的内存（字节）
	HeapIdle     uint64        // 堆空闲的内存（字节）
	HeapInuse    uint64        // 堆正在使用的内存（字节）
	HeapReleased uint64        // 堆释放给系统的内存（字节）
	HeapObjects  uint64        // 堆对象数量
	StackInuse   uint64        // 栈使用的内存（字节）
	StackSys     uint64        // 栈从系统获取的内存（字节）
	MSpanInuse   uint64        // MSpan 使用的内存（字节）
	MSpanSys     uint64        // MSpan 从系统获取的内存（字节）
	MCacheInuse  uint64        // MCache 使用的内存（字节）
	MCacheSys    uint64        // MCache 从系统获取的内存（字节）
	BuckHashSys  uint64        // Bucket hash 使用的内存（字节）
	GCSys        uint64        // GC 使用的内存（字节）
	OtherSys     uint64        // 其他系统内存（字节）
	NextGC       uint64        // 下次 GC 目标（字节）
	LastGC       time.Time     // 上次 GC 时间
	NumGC        uint32        // GC 次数
	NumForcedGC  uint32        // 强制 GC 次数
	GCPause      time.Duration // 上次 GC 暂停时间
	GCPauseTotal time.Duration // 总 GC 暂停时间
}

// MemoryMonitor 内存监控器
type MemoryMonitor struct {
	mu                  sync.RWMutex
	stats               []MemoryStats
	maxStats            int
	thresholds          MemoryThresholds
	onThresholdExceeded func(MemoryStats)
	gcStats             GCStats
	sampleInterval      time.Duration
	stopCh              chan struct{}
	started             bool
	stopped             bool
}

// MemoryThresholds 内存阈值配置
type MemoryThresholds struct {
	AllocWarning     uint64  // 分配内存警告阈值
	AllocCritical    uint64  // 分配内存严重阈值
	HeapWarning      uint64  // 堆内存警告阈值
	HeapCritical     uint64  // 堆内存严重阈值
	GCTriggerPercent float64 // GC 触发百分比（相对于上次 GC 后的堆大小）
}

// GCStats GC 统计信息
type GCStats struct {
	LastGCTime      time.Time
	TotalGCPause    time.Duration
	TotalGC         uint32
	ForcedGC        uint32
	GCPauseHistory  []time.Duration
	MaxPauseHistory int
}

// DefaultThresholds 默认内存阈值
var DefaultThresholds = MemoryThresholds{
	AllocWarning:     100 * 1024 * 1024,  // 100MB
	AllocCritical:    500 * 1024 * 1024,  // 500MB
	HeapWarning:      200 * 1024 * 1024,  // 200MB
	HeapCritical:     1024 * 1024 * 1024, // 1GB
	GCTriggerPercent: 0.8,                // 80%
}

// NewMemoryMonitor 创建内存监控器
func NewMemoryMonitor() *MemoryMonitor {
	return &MemoryMonitor{
		stats:          make([]MemoryStats, 0, 1000),
		maxStats:       1000,
		thresholds:     DefaultThresholds,
		sampleInterval: 10 * time.Second,
		stopCh:         make(chan struct{}),
		gcStats: GCStats{
			MaxPauseHistory: 100,
		},
	}
}

// SetMaxStats 设置最大统计数量
func (m *MemoryMonitor) SetMaxStats(max int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxStats = max
	if len(m.stats) > max {
		m.stats = m.stats[len(m.stats)-max:]
	}
}

// SetThresholds 设置内存阈值
func (m *MemoryMonitor) SetThresholds(t MemoryThresholds) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.thresholds = t
}

// SetOnThresholdExceeded 设置阈值超出回调
func (m *MemoryMonitor) SetOnThresholdExceeded(fn func(MemoryStats)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onThresholdExceeded = fn
}

// SetSampleInterval 设置采样间隔
func (m *MemoryMonitor) SetSampleInterval(interval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sampleInterval = interval
}

// Start 启动监控（幂等：重复 Start 不会起多个循环）。
func (m *MemoryMonitor) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()
	go m.monitorLoop()
}

// Stop 停止监控（幂等：重复 Stop 不会 double-close panic）。
func (m *MemoryMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return
	}
	m.stopped = true
	close(m.stopCh)
}

// monitorLoop 监控循环
func (m *MemoryMonitor) monitorLoop() {
	ticker := time.NewTicker(m.sampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.Sample()
		case <-m.stopCh:
			return
		}
	}
}

// Sample 采集内存统计
func (m *MemoryMonitor) Sample() MemoryStats {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	stats := MemoryStats{
		Timestamp:    time.Now(),
		Alloc:        ms.Alloc,
		TotalAlloc:   ms.TotalAlloc,
		Sys:          ms.Sys,
		HeapAlloc:    ms.HeapAlloc,
		HeapSys:      ms.HeapSys,
		HeapIdle:     ms.HeapIdle,
		HeapInuse:    ms.HeapInuse,
		HeapReleased: ms.HeapReleased,
		HeapObjects:  ms.HeapObjects,
		StackInuse:   ms.StackInuse,
		StackSys:     ms.StackSys,
		MSpanInuse:   ms.MSpanInuse,
		MSpanSys:     ms.MSpanSys,
		MCacheInuse:  ms.MCacheInuse,
		MCacheSys:    ms.MCacheSys,
		BuckHashSys:  ms.BuckHashSys,
		GCSys:        ms.GCSys,
		OtherSys:     ms.OtherSys,
		NextGC:       ms.NextGC,
		LastGC:       time.Unix(0, int64(ms.LastGC)),
		NumGC:        ms.NumGC,
		NumForcedGC:  ms.NumForcedGC,
		GCPause:      time.Duration(ms.PauseNs[(ms.NumGC+255)%256]),
		GCPauseTotal: time.Duration(ms.PauseTotalNs),
	}

	m.mu.Lock()
	m.stats = append(m.stats, stats)
	if len(m.stats) > m.maxStats {
		m.stats = m.stats[1:]
	}

	// 更新 GC 统计
	if ms.NumGC > m.gcStats.TotalGC {
		m.gcStats.LastGCTime = time.Now()
		m.gcStats.TotalGC = ms.NumGC
		m.gcStats.GCPauseHistory = append(m.gcStats.GCPauseHistory, stats.GCPause)
		if len(m.gcStats.GCPauseHistory) > m.gcStats.MaxPauseHistory {
			m.gcStats.GCPauseHistory = m.gcStats.GCPauseHistory[1:]
		}
	}

	// 检查阈值
	m.checkThresholds(stats)
	m.mu.Unlock()

	return stats
}

// checkThresholds 检查内存阈值
func (m *MemoryMonitor) checkThresholds(stats MemoryStats) {
	if m.onThresholdExceeded == nil {
		return
	}

	// 检查分配内存
	if m.thresholds.AllocCritical > 0 && stats.Alloc >= m.thresholds.AllocCritical {
		m.onThresholdExceeded(stats)
		return
	}
	if m.thresholds.AllocWarning > 0 && stats.Alloc >= m.thresholds.AllocWarning {
		m.onThresholdExceeded(stats)
		return
	}

	// 检查堆内存
	if m.thresholds.HeapCritical > 0 && stats.HeapAlloc >= m.thresholds.HeapCritical {
		m.onThresholdExceeded(stats)
		return
	}
	if m.thresholds.HeapWarning > 0 && stats.HeapAlloc >= m.thresholds.HeapWarning {
		m.onThresholdExceeded(stats)
		return
	}
}

// GetStats 获取所有统计信息
func (m *MemoryMonitor) GetStats() []MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make([]MemoryStats, len(m.stats))
	copy(stats, m.stats)
	return stats
}

// GetLatestStats 获取最新的统计信息
func (m *MemoryMonitor) GetLatestStats() MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.stats) == 0 {
		return MemoryStats{}
	}
	return m.stats[len(m.stats)-1]
}

// GetStatsInRange 获取指定时间范围内的统计信息
func (m *MemoryMonitor) GetStatsInRange(start, end time.Time) []MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []MemoryStats
	for _, stats := range m.stats {
		if stats.Timestamp.After(start) && stats.Timestamp.Before(end) {
			result = append(result, stats)
		}
	}
	return result
}

// GetGCStats 获取 GC 统计信息
func (m *MemoryMonitor) GetGCStats() GCStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	gcStats := m.gcStats
	gcStats.GCPauseHistory = make([]time.Duration, len(m.gcStats.GCPauseHistory))
	copy(gcStats.GCPauseHistory, m.gcStats.GCPauseHistory)
	return gcStats
}

// GetAverageAlloc 获取平均分配内存
func (m *MemoryMonitor) GetAverageAlloc() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.stats) == 0 {
		return 0
	}

	total := uint64(0)
	for _, stats := range m.stats {
		total += stats.Alloc
	}
	return total / uint64(len(m.stats))
}

// GetMaxAlloc 获取最大分配内存
func (m *MemoryMonitor) GetMaxAlloc() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	max := uint64(0)
	for _, stats := range m.stats {
		if stats.Alloc > max {
			max = stats.Alloc
		}
	}
	return max
}

// GetMinAlloc 获取最小分配内存
func (m *MemoryMonitor) GetMinAlloc() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.stats) == 0 {
		return 0
	}

	min := m.stats[0].Alloc
	for _, stats := range m.stats {
		if stats.Alloc < min {
			min = stats.Alloc
		}
	}
	return min
}

// GetAllocTrend 获取内存分配趋势（最近的 N 个样本）
func (m *MemoryMonitor) GetAllocTrend(n int) []uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.stats) == 0 {
		return nil
	}

	start := 0
	if len(m.stats) > n {
		start = len(m.stats) - n
	}

	trend := make([]uint64, 0, len(m.stats)-start)
	for i := start; i < len(m.stats); i++ {
		trend = append(trend, m.stats[i].Alloc)
	}
	return trend
}

// Clear 清空统计信息
func (m *MemoryMonitor) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats = make([]MemoryStats, 0, m.maxStats)
}

// ForceGC 强制执行垃圾回收
func ForceGC() {
	runtime.GC()
}

// ReadMemStats 读取内存统计信息
func ReadMemStats() MemoryStats {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	return MemoryStats{
		Timestamp:    time.Now(),
		Alloc:        ms.Alloc,
		TotalAlloc:   ms.TotalAlloc,
		Sys:          ms.Sys,
		HeapAlloc:    ms.HeapAlloc,
		HeapSys:      ms.HeapSys,
		HeapIdle:     ms.HeapIdle,
		HeapInuse:    ms.HeapInuse,
		HeapReleased: ms.HeapReleased,
		HeapObjects:  ms.HeapObjects,
		NextGC:       ms.NextGC,
		LastGC:       time.Unix(0, int64(ms.LastGC)),
		NumGC:        ms.NumGC,
		GCPauseTotal: time.Duration(ms.PauseTotalNs),
	}
}

// GetMemoryUsage 获取当前内存使用情况
func GetMemoryUsage() uint64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Alloc
}

// FormatBytes 格式化字节数
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// GetGoroutineCount 获取当前 goroutine 数量
func GetGoroutineCount() int {
	return runtime.NumGoroutine()
}

// GetCPUCount 获取 CPU 核心数
func GetCPUCount() int {
	return runtime.NumCPU()
}
