package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func createTestHandler(id string) http.Handler {
	startTime := time.Now()
	var healthy uint32 = 1

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-ID", id)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"backend_id": id,
			"path":       r.URL.Path,
		})
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-ID", id)
		if healthy == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "DOWN"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":         "UP",
			"id":             id,
			"uptime_seconds": int64(time.Since(startTime).Seconds()),
		})
	})

	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "slow response"})
	})

	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Internal Server Error"})
	})

	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	})

	return mux
}

func TestBackendEndpoints(t *testing.T) {
	handler := createTestHandler("test-backend-1")
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Test GET /
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Backend-ID") != "test-backend-1" {
		t.Errorf("expected header X-Backend-ID test-backend-1, got %s", resp.Header.Get("X-Backend-ID"))
	}
	_ = resp.Body.Close()

	// Test GET /health
	resp, err = http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected health status 200, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Test GET /error
	resp, err = http.Get(ts.URL + "/error")
	if err != nil {
		t.Fatalf("GET /error failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected error status 500, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Test GET /info
	resp, err = http.Get(ts.URL + "/info")
	if err != nil {
		t.Fatalf("GET /info failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected info status 200, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}
