package proxy

import (
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"
)

// ShouldRetryStatus determines whether an upstream HTTP response should trigger retry logic.
func ShouldRetryStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusBadGateway, // 502
		http.StatusServiceUnavailable, // 503
		http.StatusGatewayTimeout:     // 504
		return true
	default:
		return false
	}
}

// ShouldRetryError determines whether a transport/network error is considered transient and retryable.
func ShouldRetryError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return true
	}
	return false
}
