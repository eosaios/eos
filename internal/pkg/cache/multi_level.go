package cache

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"sync"
	"time"
)

// CacheLevel 缓存级别
type CacheLevel int

const (
	// CacheLevelL1 一级缓存：内存中最热数据，快速访问
	CacheLevelL1 CacheLevel = iota
	// CacheLevelL2 二级缓存：内存中较热数据
	CacheLevelL2
	// CacheLevelL3 三级缓存：持久化或大容量数据
	CacheLevelL3
)

func (l CacheLevel) String() string {
	switch l {
	case CacheLevelL1:
		return "L1"
	case CacheLevelL2:
		return "L2"
	case CacheLevelL3:
		return "L3"
	default:
		return "unknown"
	}
}

// LevelConfig 缓存级别配置
type LevelConfig struct {
	Capacity    int           // 容量
	TTL         time.Duration // 过期时间
	Description string        // 描述
}

// DefaultLevelConfig 默认缓存级别配置
var DefaultLevelConfig = map[CacheLevel]LevelConfig{
	CacheLevelL1: {
		Capacity:    100,
		TTL:         5 * time.Minute,
		Description: "Hot data cache",
	},
	CacheLevelL2: {
		Capacity:    500,
		TTL:         30 * time.Minute,
		Description: "Warm data cache",
	},
	CacheLevelL3: {
		Capacity:    2000,
		TTL:         2 * time.Hour,
		Description: "Cold data cache",
	},
}

// MultiLevelCache 多级缓存
type MultiLevelCache[K comparable, V any] struct {
	mu     sync.RWMutex
	levels [3]*LRUCache[K, V]
	config map[CacheLevel]LevelConfig
	stats  MultiLevelStats
}

// MultiLevelStats 多级缓存统计
type MultiLevelStats struct {
	L1Hits   int64
	L2Hits   int64
	L3Hits   int64
	Misses   int64
	Promotes int64 // L1->L2, L2->L3 晋升次数
	Demotes  int64 // L3->L2, L2->L1 降级次数
	Puts     int64
	Gets     int64
}

// NewMultiLevelCache 创建多级缓存
func NewMultiLevelCache[K comparable, V any](config map[CacheLevel]LevelConfig) *MultiLevelCache[K, V] {
	if config == nil {
		config = DefaultLevelConfig
	}

	mlc := &MultiLevelCache[K, V]{
		config: config,
	}

	// 初始化各级缓存
	for level := CacheLevelL1; level <= CacheLevelL3; level++ {
		cfg := config[level]
		mlc.levels[level] = NewLRUCacheWithTTL[K, V](cfg.Capacity, cfg.TTL)
	}

	return mlc
}

// Get 获取值，从 L1 到 L3 依次查找
func (m *MultiLevelCache[K, V]) Get(key K) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats.Gets++

	// L1 缓存
	if val, ok := m.levels[CacheLevelL1].Get(key); ok {
		m.stats.L1Hits++
		return val, true
	}

	// L2 缓存
	if val, ok := m.levels[CacheLevelL2].Get(key); ok {
		m.stats.L2Hits++
		// 晋升到 L1
		m.levels[CacheLevelL1].Put(key, val)
		m.stats.Promotes++
		return val, true
	}

	// L3 缓存
	if val, ok := m.levels[CacheLevelL3].Get(key); ok {
		m.stats.L3Hits++
		// 晋升到 L2
		m.levels[CacheLevelL2].Put(key, val)
		m.stats.Promotes++
		return val, true
	}

	m.stats.Misses++
	var zero V
	return zero, false
}

// GetOrElse 获取值，不存在时执行提供的函数
func (m *MultiLevelCache[K, V]) GetOrElse(key K, fn func() (V, error)) (V, error) {
	if val, ok := m.Get(key); ok {
		return val, nil
	}
	val, err := fn()
	if err != nil {
		var zero V
		return zero, err
	}
	m.Put(key, val, CacheLevelL1) // 新数据优先放入 L1
	return val, nil
}

// Put 写入值到指定级别
func (m *MultiLevelCache[K, V]) Put(key K, value V, level CacheLevel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats.Puts++

	if level >= CacheLevelL1 && level <= CacheLevelL3 {
		m.levels[level].Put(key, value)
	}
}

// PutL1 写入 L1 缓存
func (m *MultiLevelCache[K, V]) PutL1(key K, value V) {
	m.Put(key, value, CacheLevelL1)
}

// PutL2 写入 L2 缓存
func (m *MultiLevelCache[K, V]) PutL2(key K, value V) {
	m.Put(key, value, CacheLevelL2)
}

// PutL3 写入 L3 缓存
func (m *MultiLevelCache[K, V]) PutL3(key K, value V) {
	m.Put(key, value, CacheLevelL3)
}

// Delete 从所有级别删除值
func (m *MultiLevelCache[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for level := CacheLevelL1; level <= CacheLevelL3; level++ {
		m.levels[level].Delete(key)
	}
}

// Clear 清空所有级别缓存
func (m *MultiLevelCache[K, V]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for level := CacheLevelL1; level <= CacheLevelL3; level++ {
		m.levels[level].Clear()
	}
	m.stats = MultiLevelStats{}
}

// Has 检查键是否存在于任何级别
func (m *MultiLevelCache[K, V]) Has(key K) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for level := CacheLevelL1; level <= CacheLevelL3; level++ {
		if m.levels[level].Has(key) {
			return true
		}
	}
	return false
}

// Stats 获取多级缓存统计信息
func (m *MultiLevelCache[K, V]) Stats() MultiLevelStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

// LevelStats 获取指定级别的统计信息
func (m *MultiLevelCache[K, V]) LevelStats(level CacheLevel) CacheStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if level >= CacheLevelL1 && level <= CacheLevelL3 {
		return m.levels[level].Stats()
	}
	return CacheStats{}
}

// TotalHits 获取总命中次数
func (m *MultiLevelCache[K, V]) TotalHits() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats.L1Hits + m.stats.L2Hits + m.stats.L3Hits
}

// HitRate 计算总命中率
func (m *MultiLevelCache[K, V]) HitRate() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := m.TotalHits() + m.stats.Misses
	if total == 0 {
		return 0
	}
	return float64(m.TotalHits()) / float64(total)
}

// LevelHitRate 计算指定级别的命中率
func (m *MultiLevelCache[K, V]) LevelHitRate(level CacheLevel) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if level >= CacheLevelL1 && level <= CacheLevelL3 {
		return m.levels[level].HitRate()
	}
	return 0
}

// ResetStats 重置统计信息
func (m *MultiLevelCache[K, V]) ResetStats() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stats = MultiLevelStats{}
	for level := CacheLevelL1; level <= CacheLevelL3; level++ {
		m.levels[level].ResetStats()
	}
}

// Size 获取指定级别的大小
func (m *MultiLevelCache[K, V]) Size(level CacheLevel) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if level >= CacheLevelL1 && level <= CacheLevelL3 {
		return m.levels[level].Size()
	}
	return 0
}

// TotalSize 获取所有级别的总大小
func (m *MultiLevelCache[K, V]) TotalSize() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := 0
	for level := CacheLevelL1; level <= CacheLevelL3; level++ {
		total += m.levels[level].Size()
	}
	return total
}

// CleanExpired 清理所有级别的过期条目
func (m *MultiLevelCache[K, V]) CleanExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	total := 0
	for level := CacheLevelL1; level <= CacheLevelL3; level++ {
		total += m.levels[level].CleanExpired()
	}
	return total
}

// ResizeLevel 调整指定级别的容量
func (m *MultiLevelCache[K, V]) ResizeLevel(level CacheLevel, newCapacity int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if level >= CacheLevelL1 && level <= CacheLevelL3 {
		m.levels[level].Resize(newCapacity)
		cfg := m.config[level]
		cfg.Capacity = newCapacity
		m.config[level] = cfg
	}
}

// GetLevel 获取指定级别的缓存
func (m *MultiLevelCache[K, V]) GetLevel(level CacheLevel) *LRUCache[K, V] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if level >= CacheLevelL1 && level <= CacheLevelL3 {
		return m.levels[level]
	}
	return nil
}

// SetConfig 更新级别配置
func (m *MultiLevelCache[K, V]) SetConfig(level CacheLevel, config LevelConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if level >= CacheLevelL1 && level <= CacheLevelL3 {
		m.config[level] = config
		m.levels[level].Resize(config.Capacity)
		m.levels[level].SetTTL(config.TTL)
	}
}

// GetConfig 获取级别配置
func (m *MultiLevelCache[K, V]) GetConfig(level CacheLevel) (LevelConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if level >= CacheLevelL1 && level <= CacheLevelL3 {
		return m.config[level], true
	}
	return LevelConfig{}, false
}

// WarmUp 预热缓存（从数据源加载到指定级别）
func (m *MultiLevelCache[K, V]) WarmUp(level CacheLevel, entries map[K]V) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if level < CacheLevelL1 || level > CacheLevelL3 {
		return
	}

	cache := m.levels[level]
	for key, value := range entries {
		cache.Put(key, value)
	}
}

// Promote 晋升条目到更高级别
func (m *MultiLevelCache[K, V]) Promote(key K, fromLevel, toLevel CacheLevel) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if fromLevel < CacheLevelL1 || fromLevel > CacheLevelL3 ||
		toLevel < CacheLevelL1 || toLevel > CacheLevelL3 ||
		fromLevel >= toLevel {
		return false
	}

	if val, ok := m.levels[fromLevel].Get(key); ok {
		m.levels[toLevel].Put(key, val)
		m.stats.Promotes++
		return true
	}
	return false
}

// Keys 获取指定级别的所有键
func (m *MultiLevelCache[K, V]) Keys(level CacheLevel) []K {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if level >= CacheLevelL1 && level <= CacheLevelL3 {
		return m.levels[level].Keys()
	}
	return nil
}
