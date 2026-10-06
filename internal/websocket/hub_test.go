
package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"flexiproxy/internal/models"
)

func TestEventBroadcaster(t *testing.T) {
	b := NewEventBroadcaster()
	ch := b.Register()
	defer b.Unregister(ch)

	msg := map[string]string{"type": "test", "data": "hello"}
	b.Broadcast(msg)

	select {
	case data := <-ch:
		if !strings.Contains(string(data), "hello") {
			t.Fatalf("Expected broadcast data to contain 'hello', got %s", string(data))
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("Timeout waiting for broadcast event")
	}
}

func TestServeSSE(t *testing.T) {
	handler := ServeSSE([]*models.Backend{})

	req := httptest.NewRequest("GET", "/api/v1/events", nil)
	w := httptest.NewRecorder()

	// Create a request context that automatically ends after 500ms.
	ctx, cancel := context.WithTimeout(req.Context(), 500*time.Millisecond)
	defer cancel()

	req = req.WithContext(ctx)

	// Send an SSE event while the stream is active.
	go func() {
		time.Sleep(100 * time.Millisecond)
		GlobalBroadcaster.Broadcast(map[string]string{"event": "ping"})
	}()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for SSE, got %d", w.Code)
	}
}

