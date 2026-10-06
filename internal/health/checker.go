package health

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"flexiproxy/internal/config"
	"flexiproxy/internal/models"
)

// ActiveChecker handles periodic background probing of downstream backend health endpoints.
type ActiveChecker struct {
	cfg        config.HealthCheckConfig
	backends   []*models.Backend
	client     *http.Client
	stopChan   chan struct{}
	wg         sync.WaitGroup
	statusLock sync.RWMutex

	// Track consecutive health check outcomes per backend
	consecutiveSuccesses map[string]int
	consecutiveFailures  map[string]int
}

// NewActiveChecker initializes the health checker with dedicated HTTP timeout transport.
func NewActiveChecker(cfg config.HealthCheckConfig, backends []*models.Backend) *ActiveChecker {
	client := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			DisableKeepAlives: true, // Clean isolated probes
		},
	}

	return &ActiveChecker{
		cfg:                  cfg,
		backends:             backends,
		client:               client,
		stopChan:             make(chan struct{}),
		consecutiveSuccesses: make(map[string]int),
		consecutiveFailures:  make(map[string]int),
	}
}

// Start launches the background ticker goroutine.
func (c *ActiveChecker) Start(ctx context.Context) {
	if !c.cfg.Enabled {
		return
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.cfg.Interval)
		defer ticker.Stop()

		// Initial immediate probe on startup
		c.CheckAll(ctx)

		for {
			select {
			case <-ticker.C:
				c.CheckAll(ctx)
			case <-c.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop gracefully signals the background probing goroutine to stop.
func (c *ActiveChecker) Stop() {
	if !c.cfg.Enabled {
		return
	}
	close(c.stopChan)
	c.wg.Wait()
}

// CheckAll executes concurrent HTTP health checks against all configured backends.
func (c *ActiveChecker) CheckAll(ctx context.Context) {
	var wg sync.WaitGroup
	for _, b := range c.backends {
		wg.Add(1)
		go func(backend *models.Backend) {
			defer wg.Done()
			c.checkSingle(ctx, backend)
		}(b)
	}
	wg.Wait()
}

func (c *ActiveChecker) checkSingle(ctx context.Context, backend *models.Backend) {
	probeURL := fmt.Sprintf("%s://%s%s", backend.URL.Scheme, backend.URL.Host, c.cfg.Path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		c.recordFailure(backend, fmt.Errorf("failed to create probe request: %w", err))
		return
	}
	req.Header.Set("User-Agent", "FlexiProxy-HealthChecker/1.0")

	startTime := time.Now()
	resp, err := c.client.Do(req)
	latency := time.Since(startTime)

	if err != nil {
		c.recordFailure(backend, err)
		return
	}
	_ = resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		c.recordSuccess(backend, latency)
	} else {
		c.recordFailure(backend, fmt.Errorf("health check status code %d", resp.StatusCode))
	}
}

func (c *ActiveChecker) recordSuccess(backend *models.Backend, latency time.Duration) {
	c.statusLock.Lock()
	defer c.statusLock.Unlock()

	c.consecutiveFailures[backend.ID] = 0
	c.consecutiveSuccesses[backend.ID]++

	if c.consecutiveSuccesses[backend.ID] >= c.cfg.HealthyThreshold {
		backend.SetHealthy(true)
	}
}

func (c *ActiveChecker) recordFailure(backend *models.Backend, err error) {
	c.statusLock.Lock()
	defer c.statusLock.Unlock()

	c.consecutiveSuccesses[backend.ID] = 0
	c.consecutiveFailures[backend.ID]++

	if c.consecutiveFailures[backend.ID] >= c.cfg.UnhealthyThreshold {
		backend.SetHealthy(false)
	}
}
