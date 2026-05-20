package ratelimit

import (
	"sync"
	"time"
)

// tokenBucket stores rate limiting state for a single IP.
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
}

/*
Limiter implements per-IP token bucket rate limiting.
Flow:
1. Find/create bucket for IP
2. Refill tokens based on elapsed time
3. Consume 1 token
4. Reject request if bucket is empty
*/
type Limiter struct {
	buckets map[string]*tokenBucket
	mu      sync.RWMutex

	maxTokens  float64
	refillRate float64
	enabled    bool
}

// NewLimiter initializes the rate limiter.
func NewLimiter(enabled bool, requestsPerSecond float64, burst int) *Limiter {
	return &Limiter{
		buckets:    make(map[string]*tokenBucket),
		maxTokens:  float64(burst),
		refillRate: requestsPerSecond,
		enabled:    enabled,
	}
}

/*
Allow checks whether a request from the given IP is permitted.
Uses:
  - RLock for fast bucket lookups
  - Lock only when bucket creation/update is needed
*/
func (l *Limiter) Allow(ip string) bool {
	// If rate limiting is disabled in config, always allow.
	if !l.enabled {
		return true
	}

	// Try to find existing bucket (read lock, fast path)
	l.mu.RLock()
	bucket, exists := l.buckets[ip]
	l.mu.RUnlock()

	// If no bucket exists, create one (write lock, slow path)
	if !exists {
		l.mu.Lock()
		// Double-check in case another goroutine created it
		bucket, exists = l.buckets[ip]

		if !exists {
			bucket = &tokenBucket{
				tokens:     l.maxTokens,
				maxTokens:  l.maxTokens,
				refillRate: l.refillRate,
				lastRefill: time.Now(),
			}
			l.buckets[ip] = bucket
		}
		l.mu.Unlock()
	}

	// Consume a token (needs write lock)
	l.mu.Lock()
	defer l.mu.Unlock()

	// Refill tokens based on elapsed time.
	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	bucket.tokens += elapsed * bucket.refillRate

	// Cap at maximum (bucket can't overflow).
	if bucket.tokens > bucket.maxTokens {
		bucket.tokens = bucket.maxTokens
	}
	bucket.lastRefill = now

	// Consume token if available.
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}

	// No tokens, reject
	return false
}

// Cleanup removes stale buckets to avoid unbounded memory growth
func (l *Limiter) Cleanup(maxAge time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	for ip, bucket := range l.buckets {
		if now.Sub(bucket.lastRefill) > maxAge {
			delete(l.buckets, ip)
		}
	}
}
