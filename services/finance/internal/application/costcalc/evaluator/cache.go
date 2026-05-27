package evaluator

import (
	"sync"
)

// Cache memoises compiled evaluators keyed by (formulaCode, expression).
// Safe for concurrent use by many workers.
type Cache struct {
	mu sync.RWMutex
	m  map[string]*Evaluator
}

// NewCache constructs an empty cache.
func NewCache() *Cache {
	return &Cache{m: make(map[string]*Evaluator)}
}

// GetOrCompile returns the cached evaluator or compiles + caches a new one.
// Compilation errors do NOT poison the cache — a subsequent call with a fixed
// expression will succeed.
func (c *Cache) GetOrCompile(formulaCode, expression string) (*Evaluator, error) {
	key := formulaCode + "|" + expression
	c.mu.RLock()
	if ev, ok := c.m[key]; ok {
		c.mu.RUnlock()
		return ev, nil
	}
	c.mu.RUnlock()

	ev, err := Compile(formulaCode, expression)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.m[key] = ev
	c.mu.Unlock()
	return ev, nil
}

// Size returns the number of cached evaluators (mostly for observability/tests).
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.m)
}
