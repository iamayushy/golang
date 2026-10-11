// Package store provides a thread-safe in-memory store, used in place of a database.
package store

import "sync"

type Memory[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]V
}

func NewMemory[K comparable, V any]() *Memory[K, V] {
	return &Memory[K, V]{items: make(map[K]V)}
}

func (m *Memory[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.items[key]
	return v, ok
}

func (m *Memory[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = value
}

func (m *Memory[K, V]) Delete(key K) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.items[key]
	delete(m.items, key)
	return ok
}

func (m *Memory[K, V]) List() []V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]V, 0, len(m.items))
	for _, v := range m.items {
		out = append(out, v)
	}
	return out
}
