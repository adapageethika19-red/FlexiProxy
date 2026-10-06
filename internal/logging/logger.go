package logging

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// LogEvent represents a structured JSON log record.
type LogEvent struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Event     string `json:"event"`
	RequestID string `json:"request_id,omitempty"`
	Method    string `json:"method,omitempty"`
	Path      string `json:"path,omitempty"`
	Backend   string `json:"backend,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Status    int    `json:"status,omitempty"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
}

var (
	logMu       sync.RWMutex
	recentLogs  = make([]LogEvent, 0, 100)
	maxLogsSize = 100
)

// GenerateRequestID creates a unique cryptographically random 16-character hex request ID.
func GenerateRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// LogJSON outputs a structured JSON log line to stdout and stores in recent logs.
func LogJSON(event LogEvent) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339Nano)
	}
	if event.Level == "" {
		event.Level = "info"
	}

	data, err := json.Marshal(event)
	if err == nil {
		fmt.Fprintln(os.Stdout, string(data))
	}

	logMu.Lock()
	if len(recentLogs) >= maxLogsSize {
		recentLogs = recentLogs[1:]
	}
	recentLogs = append(recentLogs, event)
	logMu.Unlock()
}

// GetRecentLogs returns a copy of the recent log events buffer.
func GetRecentLogs() []LogEvent {
	logMu.RLock()
	defer logMu.RUnlock()
	copied := make([]LogEvent, len(recentLogs))
	copy(copied, recentLogs)
	return copied
}

// EnsureRequestID inspects incoming X-Request-ID header or injects a new one.
func EnsureRequestID(r *http.Request) string {
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = GenerateRequestID()
		r.Header.Set("X-Request-ID", reqID)
	}
	return reqID
}
