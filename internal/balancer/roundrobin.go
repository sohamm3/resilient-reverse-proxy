package balancer

import (
	"sync/atomic"

	"resilient-reverse-proxy/internal/backend"
)

// RoundRobin distributes requests sequentially across healthy backends.
type RoundRobin struct {
	counter uint64
}

// NewRoundRobin initializes a round-robin balancer.
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

// Next selects the next backend in rotation.
func (rr *RoundRobin) Next(backends []*backend.Backend) *backend.Backend {
	if len(backends) == 0 {
		return nil
	}
	next := atomic.AddUint64(&rr.counter, 1)
	index := (next - 1) % uint64(len(backends))
	return backends[index]
}

// Name returns the balancing strategy identifier.
func (rr *RoundRobin) Name() string {
	return "round-robin"
}
