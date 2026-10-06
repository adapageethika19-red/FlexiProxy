package logging

import (
	"net/http/httptest"
	"testing"
)

func TestGenerateRequestID(t *testing.T) {
	id1 := GenerateRequestID()
	id2 := GenerateRequestID()

	if id1 == "" || id2 == "" {
		t.Fatalf("Expected non-empty request IDs, got id1=%q, id2=%q", id1, id2)
	}

	if id1 == id2 {
		t.Fatalf("Expected unique request IDs, got duplicate: %q", id1)
	}
}

func TestEnsureRequestID(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/test", nil)

	id1 := EnsureRequestID(req)

	if id1 == "" {
		t.Fatalf("Expected generated request ID, got empty string")
	}

	if req.Header.Get("X-Request-ID") != id1 {
		t.Fatalf(
			"Expected header X-Request-ID to be %q, got %q",
			id1,
			req.Header.Get("X-Request-ID"),
		)
	}

	req2 := httptest.NewRequest("GET", "http://example.com/test", nil)
	req2.Header.Set("X-Request-ID", "custom-id-123")

	id2 := EnsureRequestID(req2)

	if id2 != "custom-id-123" {
		t.Fatalf(
			"Expected preserved request ID 'custom-id-123', got %q",
			id2,
		)
	}
}

func TestLogJSONAndRecentLogs(t *testing.T) {
	event := LogEvent{
		Event:     "test_event",
		RequestID: "req-abc",
		Method:    "POST",
		Path:      "/api/test",
		Status:    200,
	}

	LogJSON(event)

	recent := GetRecentLogs()

	if len(recent) == 0 {
		t.Fatalf("Expected recent logs to contain at least 1 event")
	}

	last := recent[len(recent)-1]

	if last.RequestID != "req-abc" || last.Path != "/api/test" {
		t.Fatalf("Unexpected log entry: %+v", last)
	}
}