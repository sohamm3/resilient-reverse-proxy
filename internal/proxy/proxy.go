package proxy

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync/atomic"
	"time"

	"resilient-reverse-proxy/internal/backend"
	"resilient-reverse-proxy/internal/balancer"
	"resilient-reverse-proxy/internal/logger"
	"resilient-reverse-proxy/internal/ratelimit"
)

// Proxy coordinates request routing, retries, rate limiting, and request logging.
type Proxy struct {
	pool         *backend.Pool
	algorithm    balancer.Algorithm
	rateLimiter  *ratelimit.Limiter
	log          *logger.Logger
	retryEnabled bool
	maxRetries   int
	retryCount   uint64
	totalReqs    uint64
	rateLimited  uint64
}

// NewProxy initializes the reverse proxy engine.
func NewProxy(pool *backend.Pool, algo balancer.Algorithm, limiter *ratelimit.Limiter, lg *logger.Logger, retryEnabled bool, maxRetries int) *Proxy {
	return &Proxy{
		pool:         pool,
		algorithm:    algo,
		rateLimiter:  limiter,
		log:          lg,
		retryEnabled: retryEnabled,
		maxRetries:   maxRetries,
	}
}

// ServeHTTP handles incoming requests and forwards them to healthy backends.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	atomic.AddUint64(&p.totalReqs, 1)
	clientIP := getClientIP(r)

	// Apply per-IP rate limiting.
	if !p.rateLimiter.Allow(clientIP) {
		atomic.AddUint64(&p.rateLimited, 1)
		duration := time.Since(start)
		p.log.LogRequest(clientIP, r.Method, r.URL.Path, "rate-limited", http.StatusTooManyRequests, duration, p.algorithm.Name(), true)
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	maxAttempts := 1
	if p.retryEnabled {
		maxAttempts += p.maxRetries
	}

	triedBackends := make(map[string]bool)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		alive := p.pool.GetAliveBackends()

		// Avoid retrying already-attempted backends.
		filtered := make([]*backend.Backend, 0, len(alive))

		for _, b := range alive {
			if !triedBackends[b.RawURL] {
				filtered = append(filtered, b)
			}
		}
		alive = filtered

		// If no alive backends are available, return 502 Bad Gateway.
		if len(alive) == 0 {
			duration := time.Since(start)
			p.log.LogRequest(clientIP, r.Method, r.URL.Path, "no-backends", http.StatusBadGateway, duration, p.algorithm.Name(), false)
			http.Error(w, "Bad Gateway — no healthy backends available", http.StatusBadGateway)
			return
		}

		target := p.algorithm.Next(alive)
		if target == nil {
			http.Error(w, "Backend selection failed", http.StatusBadGateway)
			return
		}

		triedBackends[target.RawURL] = true

		recorder := &responseRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusBadGateway,
		}

		reverseProxy := httputil.NewSingleHostReverseProxy(target.URL)

		// Rewrite outbound upstream request headers.
		reverseProxy.Rewrite = func(pr *httputil.ProxyRequest) {
			// Rewrite target URL.
			pr.SetURL(target.URL)

			// Preserve original Host header if desired.
			pr.Out.Host = pr.In.Host

			// Forward client IP chain.
			if prior := pr.In.Header.Get("X-Forwarded-For"); prior != "" {
				pr.Out.Header.Set("X-Forwarded-For", prior+", "+clientIP)
			} else {
				pr.Out.Header.Set("X-Forwarded-For", clientIP)
			}

			// Forward original requested host.
			pr.Out.Header.Set("X-Forwarded-Host", r.Host)

			// Forward original protocol.
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}

			pr.Out.Header.Set("X-Forwarded-Proto", scheme)
		}

		proxyError := false
		retryableResponse := false
		reverseProxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			if ShouldRetryError(err) {
				proxyError = true
				target.SetAlive(false) // passive health checks
				log.Printf("[proxy] upstream transport failure backend=%s error=%v", target.RawURL, err)
				return
			}
			if retryableResponse {
				return
			}
			log.Printf("[proxy] upstream proxy error backend=%s error=%v", target.RawURL, err)
		}

		reverseProxy.ModifyResponse = func(resp *http.Response) error {
			resp.Header.Set("X-Backend-Server", target.RawURL)
			resp.Header.Set(
				"X-Response-Time",
				fmt.Sprintf("%dms", time.Since(start).Milliseconds()),
			)

			// Retry selected upstream status codes.
			if ShouldRetryStatus(resp.StatusCode) {
				retryableResponse = true
				log.Printf("[proxy] retryable upstream status backend=%s status=%d", target.RawURL, resp.StatusCode)
				return fmt.Errorf("retryable upstream status code: %d", resp.StatusCode)
			}
			return nil
		}

		reverseProxy.ServeHTTP(recorder, r)

		if (proxyError || retryableResponse) && attempt < maxAttempts-1 {
			atomic.AddUint64(&p.retryCount, 1)
			log.Printf("[proxy] retry triggered path=%s next_attempt=%d/%d", r.URL.Path, attempt+2, maxAttempts)
			continue
		}

		duration := time.Since(start)
		p.log.LogRequest(clientIP, r.Method, r.URL.Path, target.RawURL, recorder.statusCode, duration, p.algorithm.Name(), false)
		return
	}
	http.Error(w, "Bad Gateway - all retry attempts failed", http.StatusBadGateway)
}

// TotalRequests returns total processed request count.
func (p *Proxy) TotalRequests() uint64 {
	return atomic.LoadUint64(&p.totalReqs)
}

// RateLimitedRequests returns rejected request count.
func (p *Proxy) RateLimitedRequests() uint64 {
	return atomic.LoadUint64(&p.rateLimited)
}

func (p *Proxy) RetryCount() uint64 {
	return atomic.LoadUint64(&p.retryCount)
}

// responseRecorder captures response status codes for logging and retry decisions.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// WriteHeader intercepts the status code before passing it to the real writer.
func (rec *responseRecorder) WriteHeader(code int) {
	if !rec.written {
		rec.statusCode = code
		rec.written = true
	}
	rec.ResponseWriter.WriteHeader(code)
}

// getClientIP resolves the originating client IP using forwarded headers when available.
func getClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
