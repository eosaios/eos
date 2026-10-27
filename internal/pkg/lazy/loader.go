package lazy

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// LoadState 加载状态
type LoadState int

const (
	// LoadStateNotLoaded 未加载
	LoadStateNotLoaded LoadState = iota
	// LoadStateLoading 正在加载
	LoadStateLoading
	// LoadStateLoaded 已加载
	LoadStateLoaded
	// LoadStateFailed 加载失败
	LoadStateFailed
)

func (s LoadState) String() string {
	switch s {
	case LoadStateNotLoaded:
		return "not_loaded"
	case LoadStateLoading:
		return "loading"
	case LoadStateLoaded:
		return "loaded"
	case LoadStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// LoadResult 加载结果
type LoadResult[T any] struct {
	Value    T
	Err      error
	LoadedAt time.Time
	Duration time.Duration
}

// Lazy 懒加载器
type Lazy[T any] struct {
	mu       sync.RWMutex
	state    atomic.Value // LoadState
	value    atomic.Value // T
	err      atomic.Value // error
	loader   func() (T, error)
	onLoad   func(T)
	onError  func(error)
	loadedAt atomic.Value // time.Time
	duration atomic.Value // time.Duration
	loadOnce sync.Once
}

// New 创建新的懒加载器
func New[T any](loader func() (T, error)) *Lazy[T] {
	l := &Lazy[T]{
		loader: loader,
	}
	l.state.Store(LoadStateNotLoaded)
	return l
}

// NewWithCallbacks 创建带回调的懒加载器
func NewWithCallbacks[T any](
	loader func() (T, error),
	onLoad func(T),
	onError func(error),
) *Lazy[T] {
	l := &Lazy[T]{
		loader:  loader,
		onLoad:  onLoad,
		onError: onError,
	}
	l.state.Store(LoadStateNotLoaded)
	return l
}

// Get 获取值，如果未加载则触发加载
func (l *Lazy[T]) Get() (T, error) {
	if l.IsLoaded() {
		return l.value.Load().(T), nil
	}

	// 使用 loadOnce 确保并发安全
	l.loadOnce.Do(func() {
		l.load()
	})

	if l.IsLoaded() {
		return l.value.Load().(T), nil
	}
	var zero T
	return zero, l.err.Load().(error)
}

// GetWithContext 带上下文的获取（支持取消）
func (l *Lazy[T]) GetWithContext(ctx context.Context) (T, error) {
	if l.IsLoaded() {
		return l.value.Load().(T), nil
	}

	resultChan := make(chan LoadResult[T], 1)

	go func() {
		value, err := l.Get()
		resultChan <- LoadResult[T]{
			Value: value,
			Err:   err,
		}
	}()

	select {
	case res := <-resultChan:
		return res.Value, res.Err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// Load 显式加载
func (l *Lazy[T]) Load() error {
	_, err := l.Get()
	return err
}

// load 内部加载方法
func (l *Lazy[T]) load() {
	l.state.Store(LoadStateLoading)

	start := time.Now()
	value, err := l.loader()
	duration := time.Since(start)

	l.loadedAt.Store(time.Now())
	l.duration.Store(duration)

	if err != nil {
		l.err.Store(err)
		l.state.Store(LoadStateFailed)
		if l.onError != nil {
			l.onError(err)
		}
		return
	}

	l.value.Store(value)
	l.state.Store(LoadStateLoaded)
	if l.onLoad != nil {
		l.onLoad(value)
	}
}

// Reload 重新加载
func (l *Lazy[T]) Reload() error {
	l.mu.Lock()
	l.loadOnce = sync.Once{}
	l.state.Store(LoadStateNotLoaded)
	clearValue[T](&l.value)
	l.err.Store(errNotLoaded)
	l.mu.Unlock()

	return l.Load()
}

// IsLoaded 检查是否已加载
func (l *Lazy[T]) IsLoaded() bool {
	return l.state.Load().(LoadState) == LoadStateLoaded
}

// IsLoading 检查是否正在加载
func (l *Lazy[T]) IsLoading() bool {
	return l.state.Load().(LoadState) == LoadStateLoading
}

// IsFailed 检查是否加载失败
func (l *Lazy[T]) IsFailed() bool {
	return l.state.Load().(LoadState) == LoadStateFailed
}

// State 获取当前状态
func (l *Lazy[T]) State() LoadState {
	return l.state.Load().(LoadState)
}

// ValueOrZero 获取值或零值（不触发加载）
func (l *Lazy[T]) ValueOrZero() T {
	val := l.value.Load()
	if val == nil {
		var zero T
		return zero
	}
	return val.(T)
}

// MustGet 获取值，失败时 panic
func (l *Lazy[T]) MustGet() T {
	val, err := l.Get()
	if err != nil {
		panic(fmt.Sprintf("lazy load failed: %v", err))
	}
	return val
}

// Peek 查看当前值而不触发加载
func (l *Lazy[T]) Peek() (T, bool) {
	if l.IsLoaded() {
		return l.value.Load().(T), true
	}
	var zero T
	return zero, false
}

// LoadedAt 获取加载时间
func (l *Lazy[T]) LoadedAt() time.Time {
	t := l.loadedAt.Load()
	if t == nil {
		return time.Time{}
	}
	return t.(time.Time)
}

// LoadDuration 获取加载耗时
func (l *Lazy[T]) LoadDuration() time.Duration {
	d := l.duration.Load()
	if d == nil {
		return 0
	}
	return d.(time.Duration)
}

// errNotLoaded 是 atomic.Value 的占位错误：atomic.Value 不允许 Store(nil)，
// Reset/Reload 时用它清空错误槽，Get 在未加载时走 load() 不会读到它。
var errNotLoaded = errors.New("lazy: not loaded")

// clearValue 往 atomic.Value 写入类型正确的零值（不能写 nil，也不能写 *T）。
func clearValue[T any](slot *atomic.Value) {
	var zero T
	slot.Store(zero)
}

// Reset 重置懒加载器
func (l *Lazy[T]) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.loadOnce = sync.Once{}
	l.state.Store(LoadStateNotLoaded)
	clearValue[T](&l.value)
	l.err.Store(errNotLoaded)
	l.loadedAt.Store(time.Time{})
	l.duration.Store(time.Duration(0))
}

// SetLoader 设置新的加载器
func (l *Lazy[T]) SetLoader(loader func() (T, error)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loader = loader
}

// SetOnLoad 设置加载成功回调
func (l *Lazy[T]) SetOnLoad(onLoad func(T)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onLoad = onLoad
}

// SetOnError 设置加载失败回调
func (l *Lazy[T]) SetOnError(onError func(error)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onError = onError
}

// GetResult 获取完整的加载结果
func (l *Lazy[T]) GetResult() LoadResult[T] {
	val, err := l.Get()
	return LoadResult[T]{
		Value:    val,
		Err:      err,
		LoadedAt: l.LoadedAt(),
		Duration: l.LoadDuration(),
	}
}

// LazyMap 懒加载映射
type LazyMap[K comparable, V any] struct {
	mu     sync.RWMutex
	items  map[K]*Lazy[V]
	loader func(K) (V, error)
}

// NewLazyMap 创建懒加载映射
func NewLazyMap[K comparable, V any](loader func(K) (V, error)) *LazyMap[K, V] {
	return &LazyMap[K, V]{
		items:  make(map[K]*Lazy[V]),
		loader: loader,
	}
}

// Get 获取键对应的值
func (lm *LazyMap[K, V]) Get(key K) (V, error) {
	lm.mu.RLock()
	lazy, exists := lm.items[key]
	lm.mu.RUnlock()

	if !exists {
		lm.mu.Lock()
		// 双重检查
		if lazy, exists = lm.items[key]; !exists {
			lazy = New(func() (V, error) {
				return lm.loader(key)
			})
			lm.items[key] = lazy
		}
		lm.mu.Unlock()
	}

	return lazy.Get()
}

// Delete 删除键
func (lm *LazyMap[K, V]) Delete(key K) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	delete(lm.items, key)
}

// Has 检查键是否存在
func (lm *LazyMap[K, V]) Has(key K) bool {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	_, exists := lm.items[key]
	return exists
}

// IsLoaded 检查键是否已加载
func (lm *LazyMap[K, V]) IsLoaded(key K) bool {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	if lazy, exists := lm.items[key]; exists {
		return lazy.IsLoaded()
	}
	return false
}

// Clear 清空所有条目
func (lm *LazyMap[K, V]) Clear() {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.items = make(map[K]*Lazy[V])
}

// Size 获取条目数量
func (lm *LazyMap[K, V]) Size() int {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return len(lm.items)
}

// Keys 获取所有键
func (lm *LazyMap[K, V]) Keys() []K {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	keys := make([]K, 0, len(lm.items))
	for k := range lm.items {
		keys = append(keys, k)
	}
	return keys
}

// Range 遍历所有条目
func (lm *LazyMap[K, V]) Range(fn func(key K, lazy *Lazy[V]) bool) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	for k, v := range lm.items {
		if !fn(k, v) {
			break
		}
	}
}

// Preload 预加载指定键
func (lm *LazyMap[K, V]) Preload(keys ...K) []error {
	lm.mu.Lock()
	// 创建 Lazy 实例但不加载
	for _, key := range keys {
		if _, exists := lm.items[key]; !exists {
			lm.items[key] = New(func() (V, error) {
				return lm.loader(key)
			})
		}
	}
	lazies := make([]*Lazy[V], 0, len(keys))
	for _, key := range keys {
		lazies = append(lazies, lm.items[key])
	}
	lm.mu.Unlock()

	// 并发加载
	errs := make([]error, len(keys))
	var wg sync.WaitGroup
	for i, lazy := range lazies {
		wg.Add(1)
		go func(idx int, l *Lazy[V]) {
			defer wg.Done()
			_, errs[idx] = l.Get()
		}(i, lazy)
	}
	wg.Wait()

	return errs
}

// GetOrCreate 获取或创建 Lazy 实例（不加载）
func (lm *LazyMap[K, V]) GetOrCreate(key K) *Lazy[V] {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if lazy, exists := lm.items[key]; exists {
		return lazy
	}

	lazy := New(func() (V, error) {
		return lm.loader(key)
	})
	lm.items[key] = lazy
	return lazy
}

// Evict 驱逐未使用的条目
func (lm *LazyMap[K, V]) Evict(maxIdle time.Duration) int {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	now := time.Now()
	evicted := 0

	for key, lazy := range lm.items {
		if lazy.IsLoaded() {
			if idle := now.Sub(lazy.LoadedAt()); idle > maxIdle {
				delete(lm.items, key)
				evicted++
			}
		}
	}

	return evicted
}
