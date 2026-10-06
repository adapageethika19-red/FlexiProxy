package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

type ServerInfo struct {
	ID           string    `json:"id"`
	Port         int       `json:"port"`
	StartTime    time.Time `json:"start_time"`
	UptimeSec    int64     `json:"uptime_seconds"`
	RequestCount uint64    `json:"request_count"`
	NumGoroutine int       `json:"num_goroutine"`
	OS           string    `json:"os"`
	Arch         string    `json:"arch"`
}

type ResponsePayload struct {
	BackendID    string              `json:"backend_id"`
	Path         string              `json:"path"`
	Method       string              `json:"method"`
	Headers      http.Header         `json:"headers"`
	Query        map[string][]string `json:"query"`
	Message      string              `json:"message"`
	RequestCount uint64              `json:"request_count"`
	Timestamp    time.Time           `json:"timestamp"`
}

func main() {
	port := flag.Int("port", 8001, "Port for backend demo service to listen on")
	id := flag.String("id", "backend-1", "Identifier for backend demo service")
	flag.Parse()

	startTime := time.Now()
	var requestCounter uint64
	var healthy uint32 = 1 // 1 = healthy, 0 = unhealthy

	mux := http.NewServeMux()

	// GET / - Standard response endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		reqNum := atomic.AddUint64(&requestCounter, 1)
		payload := ResponsePayload{
			BackendID:    *id,
			Path:         r.URL.Path,
			Method:       r.Method,
			Headers:      r.Header,
			Query:        r.URL.Query(),
			Message:      fmt.Sprintf("Hello from backend server %s!", *id),
			RequestCount: reqNum,
			Timestamp:    time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-ID", *id)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(payload)
	})

	// GET /health - Active health check target
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddUint64(&requestCounter, 1)
		isHealthy := atomic.LoadUint32(&healthy) == 1

		if r.Method == http.MethodPost {
			statusParam := r.URL.Query().Get("status")
			if statusParam == "down" || statusParam == "unhealthy" {
				atomic.StoreUint32(&healthy, 0)
				isHealthy = false
			} else if statusParam == "up" || statusParam == "healthy" {
				atomic.StoreUint32(&healthy, 1)
				isHealthy = true
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-ID", *id)

		if !isHealthy {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "DOWN",
				"id":     *id,
				"error":  "backend marked unhealthy via fault injection",
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":         "UP",
			"id":             *id,
			"uptime_seconds": int64(time.Since(startTime).Seconds()),
		})
	})

	// GET /slow - Intentionally delayed response
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddUint64(&requestCounter, 1)
		delayStr := r.URL.Query().Get("delay")
		delay := 2 * time.Second
		if delayStr != "" {
			if d, err := time.ParseDuration(delayStr); err == nil {
				delay = d
			} else if ms, err := strconv.Atoi(delayStr); err == nil {
				delay = time.Duration(ms) * time.Millisecond
			}
		}

		select {
		case <-time.After(delay):
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Backend-ID", *id)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"backend_id": *id,
				"message":    fmt.Sprintf("Delayed response after %v", delay),
				"delay":      delay.String(),
			})
		case <-r.Context().Done():
			// Client or proxy canceled the request
			log.Printf("[%s] /slow request canceled by client/proxy context", *id)
		}
	})

	// GET /error - Configurable HTTP error generator
	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddUint64(&requestCounter, 1)
		codeStr := r.URL.Query().Get("code")
		code := http.StatusInternalServerError
		if codeStr != "" {
			if c, err := strconv.Atoi(codeStr); err == nil && c >= 400 && c <= 599 {
				code = c
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-ID", *id)
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"backend_id":  *id,
			"error":       http.StatusText(code),
			"status_code": code,
		})
	})

	// POST /admin/fault - Controlled fault injection endpoint for testing
	mux.HandleFunc("/admin/fault", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-ID", *id)

		var req struct {
			Mode  string `json:"mode"`
			Delay string `json:"delay"`
			Code  int    `json:"code"`
		}

		if r.Header.Get("Content-Type") == "application/json" {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		if req.Mode == "" {
			req.Mode = r.URL.Query().Get("mode")
		}

		switch req.Mode {
		case "unhealthy":
			atomic.StoreUint32(&healthy, 0)
		case "healthy", "up":
			atomic.StoreUint32(&healthy, 1)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"backend_id": *id,
			"mode":       req.Mode,
			"healthy":    atomic.LoadUint32(&healthy) == 1,
			"message":    "Fault injection configuration updated",
		})
	})

	// GET /info - Real system resource information
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		reqNum := atomic.AddUint64(&requestCounter, 1)
		info := ServerInfo{
			ID:           *id,
			Port:         *port,
			StartTime:    startTime,
			UptimeSec:    int64(time.Since(startTime).Seconds()),
			RequestCount: reqNum,
			NumGoroutine: runtime.NumGoroutine(),
			OS:           runtime.GOOS,
			Arch:         runtime.GOARCH,
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-ID", *id)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(info)
	})

	addr := fmt.Sprintf(":%d", *port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	log.Printf("Starting Backend Demo [%s] on http://localhost%s", *id, addr)

	// Graceful shutdown setup
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[%s] HTTP server error: %v", *id, err)
		}
	}()

	<-stop
	log.Printf("[%s] Shutting down backend demo server...", *id)
}
