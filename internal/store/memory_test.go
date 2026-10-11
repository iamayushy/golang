package store

import (
	"sync"
	"testing"
)

func TestMemory(t *testing.T) {
	m := NewMemory[string, int]()
	m.Set("a", 1)

	if v, ok := m.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = %v, %v; want 1, true", v, ok)
	}
	if !m.Delete("a") {
		t.Fatal("Delete(a) = false, want true")
	}
	if _, ok := m.Get("a"); ok {
		t.Fatal("Get(a) after delete found a value")
	}
}

func TestMemoryConcurrent(t *testing.T) {
	m := NewMemory[int, int]()
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Set(i, i)
			m.Get(i)
		}()
	}
	wg.Wait()
	if n := len(m.List()); n != 100 {
		t.Fatalf("len = %d, want 100", n)
	}
}
