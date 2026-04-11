package runtimeapp

import (
	"encoding/json"
	"net/http"
)

const (
	SpecVersion   = "http-control-api@v1"
	SchemaVersion = "1"
)

// NewMux returns the HTTP routes for the control runtime.
func NewMux(stores ...*Store) *http.ServeMux {
	store := NewStore()
	if len(stores) > 0 && stores[0] != nil {
		store = stores[0]
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
	mux.HandleFunc("/v1/sessions", handleCreateSession(store))
	mux.HandleFunc("/v1/sessions/", handleSessionCommand(store))
	return mux
}
