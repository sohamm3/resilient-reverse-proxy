package stats

import (
	"encoding/json"
	"net/http"
	"time"

	"resilient-reverse-proxy/internal/backend"
)

type StatsProvider interface {
	TotalRequests() uint64
	RetryCount() uint64
	RateLimitedRequests() uint64
}

type Dashboard struct {
	pool      *backend.Pool
	stats     StatsProvider
	algorithm string
	startTime time.Time
}

type BackendStats struct {
	URL    string `json:"url"`
	Alive  bool   `json:"alive"`
	Weight int    `json:"weight"`
}

type StatsResponse struct {
	Algorithm           string         `json:"algorithm"`
	TotalRequests       uint64         `json:"total_requests"`
	RetryCount          uint64         `json:"retry_count"`
	RateLimitedRequests uint64         `json:"rate_limited_requests"`
	BackendStats        []BackendStats `json:"backend_stats"`
	HealthyBackends     int            `json:"healthy_backends"`
	UnhealthyBackends   int            `json:"unhealthy_backends"`
	Uptime              string         `json:"uptime"`
	Timestamp           string         `json:"timestamp"`
}

func NewDashboard(pool *backend.Pool, stats StatsProvider, algorithm string) *Dashboard {
	return &Dashboard{
		pool:      pool,
		stats:     stats,
		algorithm: algorithm,
		startTime: time.Now(),
	}
}

func (d *Dashboard) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		backends := d.pool.GetAllBackends()

		response := StatsResponse{
			Algorithm:           d.algorithm,
			TotalRequests:       d.stats.TotalRequests(),
			RetryCount:          d.stats.RetryCount(),
			RateLimitedRequests: d.stats.RateLimitedRequests(),
			Uptime:              time.Since(d.startTime).Round(time.Second).String(),
			Timestamp:           time.Now().UTC().Format(time.RFC3339),
		}

		for _, b := range backends {
			if b.IsAlive() {
				response.HealthyBackends++
			} else {
				response.UnhealthyBackends++
			}
			response.BackendStats = append(response.BackendStats, BackendStats{
				URL:    b.RawURL,
				Alive:  b.IsAlive(),
				Weight: b.Weight,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")

		if err := encoder.Encode(response); err != nil {
			http.Error(w, "failed to encode stats response", http.StatusInternalServerError)
		}
	}
}
