package backend

import (
	"net/url"
	"sync"
)

// Backend represents an upstream server instance.
type Backend struct {
	URL    *url.URL
	RawURL string
	Weight int
	alive  bool
	mu     sync.RWMutex
}

// NewBackend initializes a backend server instance.
func NewBackend(rawURL string, weight int) (*Backend, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	return &Backend{
		URL:    parsedURL,
		RawURL: rawURL,
		Weight: weight,
		alive:  true,
	}, nil
}

// IsAlive returns current backend health status.
func (b *Backend) IsAlive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.alive
}

// SetAlive updates backend availability state.
func (b *Backend) SetAlive(alive bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.alive = alive
}
