package logger

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

// RequestLog represents a single proxied request entry.
type RequestLog struct {
	Time        string `json:"time"`
	ClientIP    string `json:"client"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Backend     string `json:"backend"`
	StatusCode  int    `json:"status"`
	DurationMs  int64  `json:"duration_ms"`
	Algorithm   string `json:"algorithm"`
	RateLimited bool   `json:"rate_limited,omitempty"`
}

// Logger handles structured request logging and stores recent entries for dashboard access.
type Logger struct {
	stdLogger  *log.Logger
	format     string
	recentLogs []RequestLog
	maxRecent  int
	mu         sync.RWMutex
}

// NewLogger initializes the logging engine.
func NewLogger(format string) *Logger {
	return &Logger{
		stdLogger:  log.New(os.Stdout, "", 0),
		format:     format,
		recentLogs: make([]RequestLog, 0),
		maxRecent:  50,
	}
}

// LogRequest writes request metadata to stdout and updates the recent request buffer.
func (l *Logger) LogRequest(clientIP, method, path, backendURL string, statusCode int, duration time.Duration, algorithm string, rateLimited bool) {
	entry := RequestLog{
		Time:        time.Now().UTC().Format(time.RFC3339), // ISO 8601 timestamp
		ClientIP:    clientIP,
		Method:      method,
		Path:        path,
		Backend:     backendURL,
		StatusCode:  statusCode,
		DurationMs:  duration.Milliseconds(),
		Algorithm:   algorithm,
		RateLimited: rateLimited,
	}

	// Maintain rolling request history for dashboard usage.
	l.mu.Lock()
	l.recentLogs = append(l.recentLogs, entry)

	if len(l.recentLogs) > l.maxRecent {
		l.recentLogs = l.recentLogs[len(l.recentLogs)-l.maxRecent:]
	}
	l.mu.Unlock()

	if l.format == "json" {
		jsonData, err := json.Marshal(entry)
		if err != nil {
			l.stdLogger.Printf("[logger] failed to marshal request log: %v", err)
			return
		}
		l.stdLogger.Println(string(jsonData))
		return
	}

	l.stdLogger.Printf("[%s] %s %s %s -> %s | status=%d | duration=%dms | algo=%s", entry.Time, entry.ClientIP, entry.Method, entry.Path, entry.Backend, entry.StatusCode, entry.DurationMs, entry.Algorithm)
}

// GetRecentLogs returns a snapshot of recent request entries.
func (l *Logger) GetRecentLogs() []RequestLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]RequestLog, len(l.recentLogs))
	copy(result, l.recentLogs)
	return result
}
