package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Local backend servers used for testing load balancing, retries, and health checks.
func main() {
	backends := []struct {
		port int
		name string
	}{
		{8081, "Backend 1"},
		{8082, "Backend 2"},
		{8083, "Backend 3"},
	}

	singleMode := len(os.Args) > 1

	// Start specific backend(s):
	// go run ./testserver 8082
	// go run ./testserver 8081 8082
	if singleMode {
		started := false
		for _, arg := range os.Args[1:] {
			found := false
			for _, b := range backends {
				if fmt.Sprintf("%d", b.port) == arg {
					log.Printf("[testserver] starting %s on :%d", b.name, b.port)
					go startBackend(b.port, b.name)
					started = true
					found = true
					break
				}
			}
			if !found {
				log.Printf("[testserver] unknown backend port skipped: %s", arg)
			}
		}
		if !started {
			log.Fatal("[testserver] no valid backend ports provided")
		}
	} else {
		// Start all configured backends concurrently
		// go run ./testserver
		for _, b := range backends {
			go startBackend(b.port, b.name)
		}

		log.Println("--------------------------------------------------")
		log.Println("[testserver] backend servers running")
		log.Println("[testserver] press Ctrl+C to stop")
		log.Println("--------------------------------------------------")
	}

	// Keep process alive until shutdown signal arrives.
	/* select {} --- IGNORE --- */
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("\n[testserver] shutting down")
}

// startBackend launches a lightweight HTTP backend instance.
func startBackend(port int, name string) {
	// Create a new request router
	mux := http.NewServeMux()

	// Health endpoint used by active health checks.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "OK")
	})

	// Route: "/",  Catch-all application handler (handles ALL other requests)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Simulate abrupt upstream connection drop during active request processing.
		if r.URL.Query().Get("crash") == "true" {
			/* Simulate catastrophic backend crash during active request processing.
			log.Printf("[testserver] simulating backend crash on %s (%d)", name, port)
			os.Exit(1)
			*/

			log.Printf("[testserver] simulating connection drop on %s (%d)", name, port)

			hijacker, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
				return
			}

			conn, _, err := hijacker.Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}

		// Simulate retryable upstream HTTP failure.
		if r.URL.Query().Get("badgateway") == "true" {
			log.Printf("[testserver] simulating 502 response on %s (%d)", name, port)
			http.Error(w, "simulated upstream failure", http.StatusBadGateway)
			return
		}

		// Simulate lightweight backend processing latency.
		time.Sleep(5 * time.Millisecond)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Hello from %s (port %d)\nPath: %s\nMethod: %s\n", name, port, r.URL.Path, r.Method)
	})

	// Create and start the HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("[testserver] %s listening on :%d", name, port)

	if err := server.ListenAndServe(); err != nil {
		log.Printf("[testserver] %s stopped: %v", name, err)
	}
}
