package monitor

import (
	"runtime"
	"time"
)

// SystemStats holds real-time OS process resource usage stats.
type SystemStats struct {
	NumCPU          int     `json:"num_cpu"`
	NumGoroutine    int     `json:"num_goroutine"`
	AllocMB         float64 `json:"alloc_mb"`
	TotalAllocMB    float64 `json:"total_alloc_mb"`
	SysMB           float64 `json:"sys_mb"`
	HeapAllocMB     float64 `json:"heap_alloc_mb"`
	HeapSysMB       float64 `json:"heap_sys_mb"`
	NumGC           uint32  `json:"num_gc"`
	OS              string  `json:"os"`
	Arch            string  `json:"arch"`
	UptimeSeconds   int64   `json:"uptime_seconds"`
}

var startTime = time.Now()

// GetSystemStats collects real OS memory and runtime stats.
func GetSystemStats() SystemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return SystemStats{
		NumCPU:        runtime.NumCPU(),
		NumGoroutine:  runtime.NumGoroutine(),
		AllocMB:       float64(m.Alloc) / 1024.0 / 1024.0,
		TotalAllocMB:  float64(m.TotalAlloc) / 1024.0 / 1024.0,
		SysMB:         float64(m.Sys) / 1024.0 / 1024.0,
		HeapAllocMB:   float64(m.HeapAlloc) / 1024.0 / 1024.0,
		HeapSysMB:     float64(m.HeapSys) / 1024.0 / 1024.0,
		NumGC:         m.NumGC,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		UptimeSeconds: int64(time.Since(startTime).Seconds()),
	}
}
