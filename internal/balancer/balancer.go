package balancer

import (
	"fmt"

	"resilient-reverse-proxy/internal/backend"
)

// Algorithm defines backend selection behavior.
type Algorithm interface {
	Next(backends []*backend.Backend) *backend.Backend
	Name() string
}

// NewAlgorithm initializes the configured load balancing strategy.
func NewAlgorithm(name string, pool *backend.Pool) (Algorithm, error) {
	switch name {
	case "round-robin":
		return NewRoundRobin(), nil
	case "weighted-round-robin":
		return NewWeightedRoundRobin(pool), nil
	default:
		return nil, fmt.Errorf("unsupported balancing algorithm: %s", name)
	}
}
