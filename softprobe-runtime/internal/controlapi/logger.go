package controlapi

import (
	"log/slog"
	"net/http"
	"strings"

	"softprobe-runtime/internal/store"
)

// ParseLogLevel maps the SOFTPROBE_LOG_LEVEL string to a slog.Level.
// Unrecognized values default to slog.LevelInfo.
func ParseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewMuxWithLogger builds the control mux and wraps it with a structured
// request logger. Every request is logged at debug level.
func NewMuxWithLogger(st store.Store, logger *slog.Logger) http.Handler {
	mux := NewMux(st)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("session", r.URL.Path),
		)
		mux.ServeHTTP(w, r)
	})
}
