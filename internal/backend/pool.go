package backend

import (
	"sync"
)

// Pool manages registered backend servers.
type Pool struct {
	backends []*Backend
	mu       sync.RWMutex
}

// NewPool initializes an empty backend pool.
func NewPool() *Pool {
	return &Pool{
		backends: make([]*Backend, 0),
	}
}

// AddBackend registers a backend server into the pool.
func (p *Pool) AddBackend(backend *Backend) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.backends = append(p.backends, backend)
}

// GetAliveBackends returns all healthy backend instances.
func (p *Pool) GetAliveBackends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	alive := make([]*Backend, 0)

	for _, b := range p.backends {
		if b.IsAlive() {
			alive = append(alive, b)
		}
	}
	return alive
}

// GetAllBackends returns a snapshot of all configured backends.
func (p *Pool) GetAllBackends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Create a copy of the slice to return bec we don't want external code to be able to add/remove servers from it
	result := make([]*Backend, len(p.backends))
	copy(result, p.backends)

	return result
}

// Len returns the total backend count in the pool (alive + dead).
func (p *Pool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.backends)
}

// AliveCount returns number of healthy backends.
func (p *Pool) AliveCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, b := range p.backends {
		if b.IsAlive() {
			count++
		}
	}
	return count
}
