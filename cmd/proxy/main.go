package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"resilient-reverse-proxy/internal/backend"
	"resilient-reverse-proxy/internal/balancer"
	"resilient-reverse-proxy/internal/config"
	"resilient-reverse-proxy/internal/health"
	"resilient-reverse-proxy/internal/logger"
	"resilient-reverse-proxy/internal/proxy"
	"resilient-reverse-proxy/internal/ratelimit"
	"resilient-reverse-proxy/internal/stats"
)

func main() {
	// Load application configuration.
	cfg, err := config.Load("config/config.json")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Println("[startup] configuration loaded")

	// Initialize backend pool.
	pool := backend.NewPool()

	for _, backendCfg := range cfg.Backends {
		b, err := backend.NewBackend(backendCfg.URL, backendCfg.Weight)
		if err != nil {
			log.Printf("[startup] invalid backend skipped (%s): %v", backendCfg.URL, err)
			continue
		}
		pool.AddBackend(b)
		log.Printf("[startup] backend added: %s (weight=%d)", backendCfg.URL, backendCfg.Weight)
	}

	if pool.Len() == 0 {
		log.Fatal("[startup] no valid backends configured")
	}

	log.Printf("[startup] backend pool initialized (%d backends)", pool.Len())

	// Configure balancing strategy.
	algo, err := balancer.NewAlgorithm(cfg.Algorithm, pool)
	if err != nil {
		log.Fatalf("failed to initialize balancer: %v", err)
	}

	log.Printf("[startup] algorithm=%s", algo.Name())

	// Initialize request rate limiter.
	limiter := ratelimit.NewLimiter(
		cfg.RateLimit.Enabled,
		cfg.RateLimit.RequestsPerSecond,
		cfg.RateLimit.Burst,
	)

	if cfg.RateLimit.Enabled {
		log.Printf("[startup] rate limiter enabled (rps=%v burst=%d)", cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst)
	}

	// Structured logging engine.
	lg := logger.NewLogger(cfg.Logging.Format)
	log.Printf("[startup] logging=%s", cfg.Logging.Format)

	// Core reverse proxy engine.
	p := proxy.NewProxy(pool, algo, limiter, lg, cfg.Retry.Enabled, cfg.Retry.MaxAttempts)
	log.Println("[startup] reverse proxy initialized")

	// Shared shutdown context for background workers.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Active backend health monitoring.
	if cfg.HealthCheck.Enabled {
		checker := health.NewChecker(
			pool,
			cfg.HealthCheck.Interval.Duration,
			cfg.HealthCheck.Timeout.Duration,
			cfg.HealthCheck.Path,
		)
		go checker.Start(ctx)
		log.Printf("[startup] health checks enabled (interval=%s)", cfg.HealthCheck.Interval.Duration)
	}

	// Cleanup stale rate limiter entries periodically.
	go func() {
		cleanupTicker := time.NewTicker(5 * time.Minute)
		defer cleanupTicker.Stop()
		for range cleanupTicker.C {
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	// Terminal stats
	dash := stats.NewDashboard(pool, p, algo.Name())
	// http.HandleFunc("/admin/stats", dash.Handler())
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/stats", dash.Handler())
	mux.Handle("/", p)

	// HTTP server configuration.
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout.Duration,
		WriteTimeout: cfg.Server.WriteTimeout.Duration,
	}

	// Start HTTP listener.
	go func() {
		log.Println("--------------------------------------------------")
		log.Printf("[startup] proxy listening on http://localhost:%d", cfg.Server.Port)
		log.Println("[startup] press Ctrl+C for graceful shutdown")
		log.Println("--------------------------------------------------")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Listen for shutdown signals.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("\n[shutdown] received signal: %v - initiating graceful shutdown", sig)

	cancel()

	// Graceful shutdown window.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[shutdown] graceful shutdown error: %v", err)
	}

	log.Println("[shutdown] proxy has been shut down gracefully")
}
