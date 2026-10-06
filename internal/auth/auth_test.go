package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware(t *testing.T) {
	requiredKey := "secret-admin-key"
	handler := Middleware(requiredKey, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// 1. Missing Key -> 401
	req1 := httptest.NewRequest("POST", "/api/v1/reload", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 Unauthorized for missing key, got %d", rec1.Code)
	}

	// 2. Valid X-API-Key -> 200
	req2 := httptest.NewRequest("POST", "/api/v1/reload", nil)
	req2.Header.Set("X-API-Key", "secret-admin-key")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("expected status 200 OK for valid X-API-Key, got %d", rec2.Code)
	}

	// 3. Valid Bearer Token -> 200
	req3 := httptest.NewRequest("POST", "/api/v1/reload", nil)
	req3.Header.Set("Authorization", "Bearer secret-admin-key")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusOK {
		t.Errorf("expected status 200 OK for valid Bearer token, got %d", rec3.Code)
	}
}
