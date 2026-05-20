package balancer

import (
	"sync/atomic"

	"resilient-reverse-proxy/internal/backend"
)

// WeightedRoundRobin distributes requests proportionally based on backend weights.
type WeightedRoundRobin struct {
	counter  uint64
	schedule []int
}

// NewWeightedRoundRobin builds the weighted backend schedule. Backend indices are repeated according to configured weights.
func NewWeightedRoundRobin(pool *backend.Pool) *WeightedRoundRobin {
	backends := pool.GetAllBackends()
	schedule := make([]int, 0)

	for i, b := range backends {
		// For each backend, append its index to the schedule 'weight' times.
		for w := 0; w < b.Weight; w++ {
			schedule = append(schedule, i)
		}
	}

	// Fallback to regular round-robin behavior if weights are not configured.
	if len(schedule) == 0 {
		for i := range backends {
			schedule = append(schedule, i)
		}
	}

	return &WeightedRoundRobin{
		schedule: schedule,
	}
}

// Next selects the next backend using the weighted schedule.
func (wrr *WeightedRoundRobin) Next(backends []*backend.Backend) *backend.Backend {
	if len(backends) == 0 {
		return nil
	}
	if len(backends) == 1 {
		return backends[0]
	}

	next := atomic.AddUint64(&wrr.counter, 1)
	scheduleIndex := (next - 1) % uint64(len(wrr.schedule))
	backendIndex := wrr.schedule[scheduleIndex]

	// Alive backends may differ from original schedule size when unhealthy nodes are removed.
	actualIndex := backendIndex % len(backends)

	return backends[actualIndex]
}

// Name returns the balancing strategy identifier.
func (wrr *WeightedRoundRobin) Name() string {
	return "weighted-round-robin"
}
