package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"flexiproxy/internal/auth"
	"flexiproxy/internal/config"
	"flexiproxy/internal/metrics"
	"flexiproxy/internal/models"
	"flexiproxy/internal/monitor"
	"flexiproxy/internal/websocket"
)

type AdminServer struct {
	configManager *config.Manager
	backends      []*models.Backend
	startTime     time.Time
	mux           *http.ServeMux
}

func NewAdminServer(cfgMgr *config.Manager, backends []*models.Backend) *AdminServer {
	s := &AdminServer{
		configManager: cfgMgr,
		backends:      backends,
		startTime:     time.Now(),
		mux:           http.NewServeMux(),
	}

	s.routes()
	return s
}

func (s *AdminServer) routes() {
	cfg := s.configManager.Get()
	adminKey := cfg.Server.AdminAPIKey

	cors := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			h(w, r)
		}
	}

	protected := func(h http.HandlerFunc) http.HandlerFunc {
		return cors(auth.Middleware(adminKey, h))
	}

	// Public Status & Metrics
	s.mux.HandleFunc("/api/v1/status", cors(s.handleStatus))
	s.mux.HandleFunc("/api/v1/backends", cors(s.handleBackends))
	s.mux.HandleFunc("/api/v1/backends/", cors(s.handleBackendByID))
	s.mux.HandleFunc("/api/v1/metrics", cors(metrics.Handler(s.backends)))
	s.mux.HandleFunc("/metrics", cors(metrics.Handler(s.backends)))
	s.mux.HandleFunc("/api/v1/config", cors(s.handleConfig))
	s.mux.HandleFunc("/api/v1/circuit-breakers", cors(s.handleCircuitBreakers))
	s.mux.HandleFunc("/api/v1/health", cors(s.handleHealth))
	s.mux.HandleFunc("/api/v1/events", cors(websocket.ServeSSE(s.backends)))
	s.mux.HandleFunc("/api/v1/ws", cors(websocket.ServeSSE(s.backends)))

	// Protected Mutating Operations (e.g. Hot Reload)
	s.mux.HandleFunc("/api/v1/reload", protected(s.handleReload))

	// Serve Static Observability Dashboard UI
	fileServer := http.FileServer(http.Dir("dashboard"))
	s.mux.Handle("/dashboard/", http.StripPrefix("/dashboard/", fileServer))
	s.mux.Handle("/", http.RedirectHandler("/dashboard/", http.StatusFound))
}

func (s *AdminServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *AdminServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	sysStats := monitor.GetSystemStats()
	cfg := s.configManager.Get()

	healthyCount := 0
	for _, b := range s.backends {
		if b.IsHealthy() {
			healthyCount++
		}
	}

	status := map[string]interface{}{
		"system":                  "FlexiProxy",
		"version":                 "0.1.0",
		"status":                  "OPERATIONAL",
		"uptime_seconds":          sysStats.UptimeSeconds,
		"active_goroutines":       sysStats.NumGoroutine,
		"memory_alloc_mb":         sysStats.AllocMB,
		"heap_alloc_mb":           sysStats.HeapAllocMB,
		"num_cpu":                 sysStats.NumCPU,
		"backends_total":          len(s.backends),
		"backends_healthy":        healthyCount,
		"load_balancer_algorithm": cfg.LoadBalancer.Algorithm,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (s *AdminServer) handleBackends(w http.ResponseWriter, r *http.Request) {
	type BackendResponse struct {
		ID           string  `json:"id"`
		Address      string  `json:"address"`
		Healthy      bool    `json:"healthy"`
		CircuitState string  `json:"circuit_state"`
		ActiveConns  int64   `json:"active_conns"`
		TotalReqs    uint64  `json:"total_reqs"`
		FailedReqs   uint64  `json:"failed_reqs"`
		SuccessReqs  uint64  `json:"success_reqs"`
		LatencyMs    float64 `json:"latency_ms"`
		CPUUsage     float64 `json:"cpu_usage"`
		MemUsageMB   float64 `json:"mem_usage_mb"`
	}

	result := make([]BackendResponse, 0, len(s.backends))

	for _, b := range s.backends {
		state := "CLOSED"

		switch b.CircuitState.Load() {
		case 1:
			state = "OPEN"
		case 2:
			state = "HALF_OPEN"
		}

		result = append(result, BackendResponse{
			ID:           b.ID,
			Address:      b.Address,
			Healthy:      b.IsHealthy(),
			CircuitState: state,
			ActiveConns:  b.ActiveConns,
			TotalReqs:    b.TotalReqs,
			FailedReqs:   b.FailedReqs,
			SuccessReqs:  b.SuccessReqs,
			LatencyMs:    float64(b.LatencyNano) / 1_000_000,
			CPUUsage:     b.CPUUsage,
			MemUsageMB:   b.MemUsageMB,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *AdminServer) handleBackendByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/backends/")
	for _, b := range s.backends {
		if b.ID == id {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(b)
			return
		}
	}
	http.Error(w, `{"error":"Not Found","message":"Backend ID not found"}`, http.StatusNotFound)
}

func (s *AdminServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.configManager.Get())
}

func (s *AdminServer) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method Not Allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		// Reload from default file path if body is empty
		newCfg, err := s.configManager.ReloadFromFile("")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Reload failed", "message": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "reloaded_from_file", "config": newCfg})
		return
	}

	// Reload from uploaded YAML payload
	newCfg, err := s.configManager.ReloadFromBytes(body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid configuration payload", "message": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "reloaded_from_bytes", "config": newCfg})
}

func (s *AdminServer) handleCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	states := make(map[string]string)
	for _, b := range s.backends {
		st := b.CircuitState.Load()
		switch st {
		case 0:
			states[b.ID] = "CLOSED"
		case 1:
			states[b.ID] = "OPEN"
		case 2:
			states[b.ID] = "HALF_OPEN"
		default:
			states[b.ID] = "UNKNOWN"
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(states)
}

func (s *AdminServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":    "UP",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
