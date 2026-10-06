package auth

import (
	"net/http"
	"strings"
)

// Middleware validates X-API-Key or Bearer Token headers for Admin API endpoints.
func Middleware(requiredKey string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requiredKey == "" {
			// No authentication required if key is empty
			next(w, r)
			return
		}

		// Check X-API-Key header
		key := r.Header.Get("X-API-Key")
		if key == "" {
			// Check Authorization Bearer header
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				key = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if key != requiredKey {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Unauthorized","message":"Invalid or missing admin API key"}`))
			return
		}

		next(w, r)
	}
}
