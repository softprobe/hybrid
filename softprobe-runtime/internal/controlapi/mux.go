package controlapi

import (
	"encoding/json"
	"net/http"
	"os"

	"softprobe-runtime/internal/proxybackend"
	"softprobe-runtime/internal/store"
)

const (
	RuntimeVersion = "0.0.0-dev"
	SpecVersion    = "http-control-api@v1"
	SchemaVersion  = "1"
)

// NewMux returns the HTTP routes for the control runtime.
func NewMux(stores ...*store.Store) *http.ServeMux {
	st := store.NewStore()
	if len(stores) > 0 && stores[0] != nil {
		st = stores[0]
	}
	authToken := os.Getenv("SOFTPROBE_API_TOKEN")

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
	mux.Handle("/v1/sessions", withOptionalBearerAuth(authToken, handleCreateSession(st)))
	mux.Handle("/v1/sessions/", withOptionalBearerAuth(authToken, handleSessionCommand(st)))
	mux.Handle("/v1/inject", withOptionalBearerAuth(authToken, proxybackend.HandleInject(st)))
	mux.Handle("/v1/traces", withOptionalBearerAuth(authToken, proxybackend.HandleTraces(st)))
	return mux
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
