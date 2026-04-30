package controlapi

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// withOptionalBearerAuth protects a route only when a runtime API token is configured.
func withOptionalBearerAuth(token string, next http.Handler) http.Handler {
	if strings.TrimSpace(token) == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearerToken, ok := bearerTokenFromHeader(r.Header.Get("Authorization"))
		if !ok {
			writeAPIError(w, http.StatusUnauthorized, "missing_bearer_token", "missing bearer token")
			return
		}
		if subtle.ConstantTimeCompare([]byte(bearerToken), []byte(token)) != 1 {
			writeAPIError(w, http.StatusForbidden, "invalid_bearer_token", "invalid bearer token")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func bearerTokenFromHeader(header string) (string, bool) {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "Bearer ") {
		return "", false
	}

	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" {
		return "", false
	}

	return token, true
}

func writeInvalidRequestError(w http.ResponseWriter, message string) {
	writeAPIError(w, http.StatusBadRequest, "invalid_request", message)
}

func writeMethodNotAllowedError(w http.ResponseWriter) {
	writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}

func writeUnknownSessionError(w http.ResponseWriter) {
	writeAPIError(w, http.StatusNotFound, "unknown_session", "unknown session")
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
