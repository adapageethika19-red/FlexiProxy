package websocket

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"flexiproxy/internal/models"
)

// EventBroadcaster manages real-time event distribution to connected dashboard sessions.
type EventBroadcaster struct {
	mu        sync.RWMutex
	listeners map[chan []byte]bool
}

// GlobalBroadcaster singleton instance.
var GlobalBroadcaster = NewEventBroadcaster()

func NewEventBroadcaster() *EventBroadcaster {
	return &EventBroadcaster{
		listeners: make(map[chan []byte]bool),
	}
}

func (b *EventBroadcaster) Register() chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan []byte, 64)
	b.listeners[ch] = true
	return ch
}

func (b *EventBroadcaster) Unregister(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.listeners[ch] {
		delete(b.listeners, ch)
		close(ch)
	}
}

func (b *EventBroadcaster) Broadcast(event interface{}) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.listeners {
		select {
		case ch <- data:
		default:
			// Non-blocking drop if channel buffer full
		}
	}
}

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func writeWSFrame(conn net.Conn, payload []byte) error {
	length := len(payload)
	var header []byte

	if length <= 125 {
		header = []byte{0x81, byte(length)}
	} else if length <= 65535 {
		header = make([]byte, 4)
		header[0] = 0x81
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:], uint16(length))
	} else {
		header = make([]byte, 10)
		header[0] = 0x81
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:], uint64(length))
	}

	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

// ServeWS exposes a true RFC 6455 WebSocket endpoint for live dashboard updates.
func ServeWS(backends []*models.Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "Expected Upgrade: websocket", http.StatusBadRequest)
			return
		}

		key := r.Header.Get("Sec-WebSocket-Key")
		if key == "" {
			http.Error(w, "Missing Sec-WebSocket-Key", http.StatusBadRequest)
			return
		}

		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "Webserver does not support hijacking", http.StatusInternalServerError)
			return
		}

		conn, bufrw, err := hj.Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()

		acceptKey := computeAcceptKey(key)
		resHeader := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"

		if _, err := bufrw.WriteString(resHeader); err != nil {
			return
		}
		if err := bufrw.Flush(); err != nil {
			return
		}

		listener := GlobalBroadcaster.Register()
		defer GlobalBroadcaster.Unregister(listener)

		// Drains incoming WebSocket frames (detects close/ping/pong/disconnects)
		done := make(chan struct{})
		go func() {
			defer close(done)
			buf := make([]byte, 1024)
			for {
				_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
				_, err := bufrw.Read(buf)
				if err != nil {
					return
				}
			}
		}()

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case data := <-listener:
				if err := writeWSFrame(conn, data); err != nil {
					return
				}
			case <-ticker.C:
				snapshot := map[string]interface{}{
					"event":     "snapshot",
					"timestamp": time.Now().Format(time.RFC3339),
					"backends":  backends,
				}
				data, _ := json.Marshal(snapshot)
				if err := writeWSFrame(conn, data); err != nil {
					return
				}
			case <-done:
				return
			case <-r.Context().Done():
				return
			}
		}
	}
}

// ServeSSE exposes real-time Server-Sent Events (SSE) stream as a fallback.
func ServeSSE(backends []*models.Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		listener := GlobalBroadcaster.Register()
		defer GlobalBroadcaster.Unregister(listener)

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case data := <-listener:
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			case <-ticker.C:
				snapshot := map[string]interface{}{
					"event":     "snapshot",
					"timestamp": time.Now().Format(time.RFC3339),
					"backends":  backends,
				}
				data, _ := json.Marshal(snapshot)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	}
}
