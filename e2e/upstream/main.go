package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync/atomic"
)

// Stand-in HTTP dependency (payment API, auth, etc.). The app reaches this only via
// the proxy egress listener (see e2e/README.md topology).
func main() {
	var fragmentHits int64
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/count", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int64{"count": atomic.LoadInt64(&fragmentHits)})
	})
	mux.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt64(&fragmentHits, 0)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/fragment", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&fragmentHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"dep": "ok"})
	})

	log.Fatal(http.ListenAndServe(":8083", mux))
}
