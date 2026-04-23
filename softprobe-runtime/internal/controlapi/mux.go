package controlapi

import (
	"encoding/json"
	"net/http"
	"os"

	"softprobe-runtime/internal/metrics"
	"softprobe-runtime/internal/proxybackend"
	"softprobe-runtime/internal/store"
)

const (
	RuntimeVersion = "0.0.0-dev"
	SpecVersion    = "http-control-api@v1"
	SchemaVersion  = "1"
)

// SessionCommandOverrides replaces specific session-command handlers for the hosted path.
// A nil field means use the OSS default.
type SessionCommandOverrides struct {
	// Close replaces the close sub-command handler. Signature: func(w, r, sessionID).
	Close func(http.ResponseWriter, *http.Request, string)
	// LoadCase replaces the load-case sub-command handler.
	LoadCase func(http.ResponseWriter, *http.Request, string)
	// Traces replaces the /v1/traces handler entirely.
	Traces http.Handler
}

// NewMux returns the HTTP routes for the control runtime.
func NewMux(stores ...store.Store) *http.ServeMux {
	return newMuxWithOverrides(nil, stores...)
}

// NewMuxWithOverrides builds the control mux and replaces specific handlers for the hosted path.
func NewMuxWithOverrides(overrides *SessionCommandOverrides, stores ...store.Store) *http.ServeMux {
	return newMuxWithOverrides(overrides, stores...)
}

func newMuxWithOverrides(overrides *SessionCommandOverrides, stores ...store.Store) *http.ServeMux {
	var st store.Store = store.NewStore()
	if len(stores) > 0 && stores[0] != nil {
		st = stores[0]
	}
	authToken := os.Getenv("SOFTPROBE_API_TOKEN")

	tracesHandler := http.Handler(proxybackend.HandleTraces(st))
	if overrides != nil && overrides.Traces != nil {
		tracesHandler = overrides.Traces
	}

	var sessionCmdHandler http.Handler = handleSessionCommand(st)
	if overrides != nil && (overrides.Close != nil || overrides.LoadCase != nil) {
		sessionCmdHandler = handleSessionCommandWithOverrides(st, overrides)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedError(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":        "ok",
			"specVersion":   SpecVersion,
			"schemaVersion": SchemaVersion,
		})
	})
	mux.Handle("/v1/meta", withOptionalBearerAuth(authToken, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedError(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"runtimeVersion": RuntimeVersion,
			"specVersion":    SpecVersion,
			"schemaVersion":  SchemaVersion,
		})
	})))
	mux.Handle("/v1/sessions", withOptionalBearerAuth(authToken, handleSessions(st)))
	mux.Handle("/v1/sessions/", withOptionalBearerAuth(authToken, sessionCmdHandler))
	mux.Handle("/v1/inject", withOptionalBearerAuth(authToken, proxybackend.HandleInject(st)))
	mux.Handle("/v1/traces", withOptionalBearerAuth(authToken, tracesHandler))
	mux.HandleFunc("/metrics", handleMetrics)
	registerAPIDocs(mux)
	return mux
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowedError(w)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	metrics.Global.WriteTo(w)
}

func handleNotImplementedOTLP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": "not implemented",
	})
}
