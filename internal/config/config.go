package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Duration wraps time.Duration to support values like: "10s", "500ms", "1m"
type Duration struct {
	time.Duration
}

// UnmarshalJSON parses duration strings from config.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration '%s': %w", s, err)
	}
	d.Duration = dur
	return nil
}

// Config represents the application configuration.
type Config struct {
	Server      ServerConfig      `json:"server"`
	Algorithm   string            `json:"algorithm"`
	Backends    []BackendConfig   `json:"backends"`
	HealthCheck HealthCheckConfig `json:"health_check"`
	RateLimit   RateLimitConfig   `json:"rate_limit"`
	Logging     LoggingConfig     `json:"logging"`
	Retry       RetryConfig       `json:"retry"`
}

type ServerConfig struct {
	Port         int      `json:"port"`
	ReadTimeout  Duration `json:"read_timeout"`
	WriteTimeout Duration `json:"write_timeout"`
}

type BackendConfig struct {
	URL    string `json:"url"`
	Weight int    `json:"weight"`
}

type HealthCheckConfig struct {
	Enabled  bool     `json:"enabled"`
	Interval Duration `json:"interval"`
	Timeout  Duration `json:"timeout"`
	Path     string   `json:"path"`
}

// RateLimitConfig holds settings for the per-IP rate limiting system.
type RateLimitConfig struct {
	Enabled           bool    `json:"enabled"`
	RequestsPerSecond float64 `json:"requests_per_second"`
	Burst             int     `json:"burst"`
}

type LoggingConfig struct {
	Format string `json:"format"`
	Level  string `json:"level"`
}

// RetryConfig holds settings for automatic request retry on failure.
type RetryConfig struct {
	Enabled     bool `json:"enabled"`
	MaxAttempts int  `json:"max_attempts"`
}

// Load reads a JSON config file and parses it into a Config struct. Returns a pointer to the Config and an error
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", path, err)
	}

	// Default configuration values.
	cfg := &Config{
		Algorithm: "round-robin",
		Server: ServerConfig{
			Port:         8080,
			ReadTimeout:  Duration{10 * time.Second},
			WriteTimeout: Duration{30 * time.Second},
		},
		HealthCheck: HealthCheckConfig{
			Enabled:  true,
			Interval: Duration{10 * time.Second},
			Timeout:  Duration{3 * time.Second},
			Path:     "/health",
		},
		RateLimit: RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 100,
			Burst:             200,
		},
		Logging: LoggingConfig{Format: "json", Level: "info"},
		Retry:   RetryConfig{Enabled: true, MaxAttempts: 2},
	}

	// Override defaults with config file values.
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

func validate(cfg *Config) error {
	if len(cfg.Backends) == 0 {
		return fmt.Errorf("at least one backend server must be configured")
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535, got %d", cfg.Server.Port)
	}

	switch cfg.Algorithm {
	case "round-robin", "weighted-round-robin":
		// Valid
	default:
		return fmt.Errorf("unknown algorithm '%s'", cfg.Algorithm)
	}

	for i, b := range cfg.Backends {
		if b.URL == "" {
			return fmt.Errorf("backend %d has an empty URL", i)
		}
		if b.Weight <= 0 {
			cfg.Backends[i].Weight = 1
		}
	}
	return nil
}
