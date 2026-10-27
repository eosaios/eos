package cache

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"container/list"
	"sync"
	"time"
)

// Entry LRU 缓存条目
type Entry[K comparable, V any] struct {
	key        K
	value      V
	expiration time.Time
	accessAt   time.Time
}

// LRUCache 线程安全的 LRU 缓存
type LRUCache[K comparable, V any] struct {
	mu       sync.RWMutex
	capacity int
	items    map[K]*list.Element
	lruList  *list.List
	onEvict  func(K, V)
	ttl      time.Duration // 条目生存时间
	stats    CacheStats
}

// CacheStats 缓存统计信息
type CacheStats struct {
	Hits      int64 // 缓存命中次数
	Misses    int64 // 缓存未命中次数
	Evictions int64 // 驱逐次数
	Puts      int64 // 写入次数
	Gets      int64 // 读取次数
}

// NewLRUCache 创建新的 LRU 缓存
func NewLRUCache[K comparable, V any](capacity int) *LRUCache[K, V] {
	return &LRUCache[K, V]{
		capacity: capacity,
		items:    make(map[K]*list.Element),
		lruList:  list.New(),
		ttl:      0, // 默认无过期时间
	}
}

// NewLRUCacheWithTTL 创建带 TTL 的 LRU 缓存
func NewLRUCacheWithTTL[K comparable, V any](capacity int, ttl time.Duration) *LRUCache[K, V] {
	return &LRUCache[K, V]{
		capacity: capacity,
		items:    make(map[K]*list.Element),
		lruList:  list.New(),
		ttl:      ttl,
	}
}

// SetOnEvict 设置驱逐回调
func (c *LRUCache[K, V]) SetOnEvict(fn func(K, V)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onEvict = fn
}

// Put 添加或更新条目
func (c *LRUCache[K, V]) Put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putLocked(key, value)
}

// PutWithExpiration 添加带过期时间的条目
func (c *LRUCache[K, V]) PutWithExpiration(key K, value V, expiration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		c.lruList.MoveToFront(elem)
		entry := elem.Value.(*Entry[K, V])
		entry.value = value
		entry.expiration = time.Now().Add(expiration)
		entry.accessAt = time.Now()
		return
	}

	entry := &Entry[K, V]{
		key:        key,
		value:      value,
		expiration: time.Now().Add(expiration),
		accessAt:   time.Now(),
	}
	elem := c.lruList.PushFront(entry)
	c.items[key] = elem
	c.stats.Puts++

	c.evictIfNeededLocked()
}

// putLocked 内部添加方法（调用前需持有锁）
func (c *LRUCache[K, V]) putLocked(key K, value V) {
	if elem, exists := c.items[key]; exists {
		c.lruList.MoveToFront(elem)
		entry := elem.Value.(*Entry[K, V])
		entry.value = value
		entry.accessAt = time.Now()
		if c.ttl > 0 {
			entry.expiration = time.Now().Add(c.ttl)
		}
		return
	}

	entry := &Entry[K, V]{
		key:      key,
		value:    value,
		accessAt: time.Now(),
	}
	if c.ttl > 0 {
		entry.expiration = time.Now().Add(c.ttl)
	}

	elem := c.lruList.PushFront(entry)
	c.items[key] = elem
	c.stats.Puts++

	c.evictIfNeededLocked()
}

// Get 获取条目
func (c *LRUCache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats.Gets++

	elem, exists := c.items[key]
	if !exists {
		c.stats.Misses++
		var zero V
		return zero, false
	}

	entry := elem.Value.(*Entry[K, V])

	// 检查是否过期
	if !entry.expiration.IsZero() && time.Now().After(entry.expiration) {
		c.removeElementLocked(elem)
		c.stats.Misses++
		var zero V
		return zero, false
	}

	c.lruList.MoveToFront(elem)
	entry.accessAt = time.Now()
	c.stats.Hits++
	return entry.value, true
}

// GetOrElse 获取条目，不存在时执行提供的函数
func (c *LRUCache[K, V]) GetOrElse(key K, fn func() (V, error)) (V, error) {
	if value, ok := c.Get(key); ok {
		return value, nil
	}
	value, err := fn()
	if err != nil {
		var zero V
		return zero, err
	}
	c.Put(key, value)
	return value, nil
}

// Has 检查键是否存在
func (c *LRUCache[K, V]) Has(key K) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, exists := c.items[key]
	if !exists {
		return false
	}

	entry := elem.Value.(*Entry[K, V])
	if !entry.expiration.IsZero() && time.Now().After(entry.expiration) {
		return false
	}
	return true
}

// Delete 删除条目
func (c *LRUCache[K, V]) Delete(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, exists := c.items[key]
	if !exists {
		return false
	}
	c.removeElementLocked(elem)
	return true
}

// Clear 清空缓存
func (c *LRUCache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[K]*list.Element)
	c.lruList.Init()
}

// Size 获取缓存大小
func (c *LRUCache[K, V]) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Capacity 获取缓存容量
func (c *LRUCache[K, V]) Capacity() int {
	return c.capacity
}

// Stats 获取缓存统计信息
func (c *LRUCache[K, V]) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// HitRate 计算缓存命中率
func (c *LRUCache[K, V]) HitRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.stats.Hits + c.stats.Misses
	if total == 0 {
		return 0
	}
	return float64(c.stats.Hits) / float64(total)
}

// ResetStats 重置统计信息
func (c *LRUCache[K, V]) ResetStats() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats = CacheStats{}
}

// Keys 获取所有键
func (c *LRUCache[K, V]) Keys() []K {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]K, 0, len(c.items))
	for elem := c.lruList.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*Entry[K, V])
		keys = append(keys, entry.key)
	}
	return keys
}

// Values 获取所有值
func (c *LRUCache[K, V]) Values() []V {
	c.mu.RLock()
	defer c.mu.RUnlock()

	values := make([]V, 0, len(c.items))
	for elem := c.lruList.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*Entry[K, V])
		values = append(values, entry.value)
	}
	return values
}

// evictIfNeededLocked 驱逐条目如果需要（调用前需持有锁）
func (c *LRUCache[K, V]) evictIfNeededLocked() {
	for len(c.items) > c.capacity {
		elem := c.lruList.Back()
		if elem != nil {
			c.removeElementLocked(elem)
		}
	}
}

// removeElementLocked 移除元素（调用前需持有锁）
func (c *LRUCache[K, V]) removeElementLocked(elem *list.Element) {
	c.lruList.Remove(elem)
	entry := elem.Value.(*Entry[K, V])
	delete(c.items, entry.key)
	c.stats.Evictions++
	if c.onEvict != nil {
		c.onEvict(entry.key, entry.value)
	}
}

// CleanExpired 清理过期条目
func (c *LRUCache[K, V]) CleanExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expired := 0

	for elem := c.lruList.Back(); elem != nil; {
		prev := elem.Prev()
		entry := elem.Value.(*Entry[K, V])
		if !entry.expiration.IsZero() && now.After(entry.expiration) {
			c.removeElementLocked(elem)
			expired++
		}
		elem = prev
	}

	return expired
}

// Resize 调整缓存容量
func (c *LRUCache[K, V]) Resize(newCapacity int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.capacity = newCapacity
	c.evictIfNeededLocked()
}

// UpdateCapacity 更新缓存容量（如果新容量更小则驱逐）
func (c *LRUCache[K, V]) UpdateCapacity(newCapacity int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if newCapacity < c.capacity {
		c.capacity = newCapacity
		c.evictIfNeededLocked()
	}
	c.capacity = newCapacity
}

// Peek 查看条目但不更新 LRU
func (c *LRUCache[K, V]) Peek(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, exists := c.items[key]
	if !exists {
		var zero V
		return zero, false
	}

	entry := elem.Value.(*Entry[K, V])
	if !entry.expiration.IsZero() && time.Now().After(entry.expiration) {
		var zero V
		return zero, false
	}
	return entry.value, true
}

// Len 返回缓存条目数量
func (c *LRUCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

// IsEmpty 检查缓存是否为空
func (c *LRUCache[K, V]) IsEmpty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len() == 0
}

// IsFull 检查缓存是否已满
func (c *LRUCache[K, V]) IsFull() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len() >= c.capacity
}

// Contains 检查是否包含键
func (c *LRUCache[K, V]) Contains(key K) bool {
	return c.Has(key)
}

// Range 遍历缓存中的所有条目
func (c *LRUCache[K, V]) Range(fn func(key K, value V) bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for elem := c.lruList.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*Entry[K, V])
		if !entry.expiration.IsZero() && time.Now().After(entry.expiration) {
			continue
		}
		if !fn(entry.key, entry.value) {
			break
		}
	}
}

// GetAccessTime 获取键的访问时间
func (c *LRUCache[K, V]) GetAccessTime(key K) (time.Time, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, exists := c.items[key]
	if !exists {
		return time.Time{}, false
	}
	entry := elem.Value.(*Entry[K, V])
	return entry.accessAt, true
}

// SetTTL 设置全局 TTL
func (c *LRUCache[K, V]) SetTTL(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttl = ttl
}

// GetTTL 获取全局 TTL
func (c *LRUCache[K, V]) GetTTL() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ttl
}
