package controlapi

import (
	"encoding/json"
	"net/http"

	"softprobe-runtime/internal/proxybackend"
	"softprobe-runtime/internal/store"
)

const (
	SpecVersion   = "http-control-api@v1"
	SchemaVersion = "1"
)

// NewMux returns the HTTP routes for the control runtime.
func NewMux(stores ...*store.Store) *http.ServeMux {
	st := store.NewStore()
	if len(stores) > 0 && stores[0] != nil {
		st = stores[0]
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
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
	mux.HandleFunc("/v1/sessions", handleCreateSession(st))
	mux.HandleFunc("/v1/sessions/", handleSessionCommand(st))
	mux.HandleFunc("/v1/inject", proxybackend.HandleInject(st))
	mux.HandleFunc("/v1/traces", proxybackend.HandleTraces(st))
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
