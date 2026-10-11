package shortener

import (
	"context"
	"sync"
)

// MemoryRepository is an in-memory Repository, useful for tests.
type MemoryRepository struct {
	mu    sync.Mutex
	links map[string]Link
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{links: make(map[string]Link)}
}

func (m *MemoryRepository) Create(_ context.Context, link Link) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.links[link.Code]; ok {
		return ErrCodeExists
	}
	m.links[link.Code] = link
	return nil
}

func (m *MemoryRepository) Get(_ context.Context, code string) (Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	link, ok := m.links[code]
	if !ok {
		return Link{}, ErrNotFound
	}
	return link, nil
}

func (m *MemoryRepository) Resolve(_ context.Context, code string) (Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	link, ok := m.links[code]
	if !ok {
		return Link{}, ErrNotFound
	}
	link.Clicks++
	m.links[code] = link
	return link, nil
}

func (m *MemoryRepository) Delete(_ context.Context, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.links[code]; !ok {
		return ErrNotFound
	}
	delete(m.links, code)
	return nil
}
