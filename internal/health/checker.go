package health

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"resilient-reverse-proxy/internal/backend"
	"time"
)

// Checker periodically probes backend health endpoints and updates backend availability state.
type Checker struct {
	pool     *backend.Pool
	interval time.Duration
	timeout  time.Duration
	path     string
	client   *http.Client
}

// NewChecker creates a health checker with its own HTTP client.
func NewChecker(pool *backend.Pool, interval time.Duration, timeout time.Duration, path string) *Checker {
	return &Checker{
		pool:     pool,
		interval: interval,
		timeout:  timeout,
		path:     path,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Start begins the background health check loop. It stops automatically when the context is cancelled.
func (c *Checker) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Initial backend validation on startup.
	c.checkAll()

	log.Printf("[health] checker started (interval=%s)", c.interval)

	for {
		select {
		case <-ticker.C:
			c.checkAll()
		case <-ctx.Done():
			log.Printf("[health] checker stopped")
			return
		}
	}
}

// checkAll checks the health of every backend in the pool. It iterates through ALL backends (alive AND dead) and pings each one.
func (c *Checker) checkAll() {
	backends := c.pool.GetAllBackends()

	for _, b := range backends {
		currentStatus := c.checkOne(b)
		previousStatus := b.IsAlive()

		b.SetAlive(currentStatus)

		// Log only status transitions to avoid noisy logs.
		if previousStatus && !currentStatus {
			log.Printf("[health] backend DOWN: %s", b.RawURL)
		}
		if !previousStatus && currentStatus {
			log.Printf("[health] backend UP: %s", b.RawURL)
		}
	}
}

// checkOne probes a backend health endpoint. Returns true only for HTTP 200 responses.
func (c *Checker) checkOne(b *backend.Backend) bool {
	healthURL := fmt.Sprintf("%s%s", b.RawURL, c.path)

	resp, err := c.client.Get(healthURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
