package health

import (
	"sync/atomic"
	"time"

	"flexiproxy/internal/models"
)

// PassiveMonitor tracks real user request outcomes to detect backend degradation in real time.
type PassiveMonitor struct{}

func NewPassiveMonitor() *PassiveMonitor {
	return &PassiveMonitor{}
}

// ObserveRequest evaluates live response status and duration for passive backend health tracking.
func (p *PassiveMonitor) ObserveRequest(backend *models.Backend, statusCode int, duration time.Duration, err error) {
	if backend == nil {
		return
	}

	if err != nil || statusCode >= 500 {
		atomic.AddUint64(&backend.FailedReqs, 1)
	} else {
		atomic.AddUint64(&backend.SuccessReqs, 1)
	}
}
