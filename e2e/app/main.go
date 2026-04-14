package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"sync/atomic"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Minimal application (SUT): serves /hello and calls a dependency through the proxy
// egress port (client → proxy → app → proxy → upstream).
//
// Outbound calls MUST propagate context with OpenTelemetry (W3C traceparent + tracestate),
// not by copying x-softprobe-session-id. The mesh proxy places the session id in tracestate
// (see softprobe-proxy inject_trace_context_headers / build_new_tracestate); the Go
// TraceContext propagator carries it on the next hop.
func main() {
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(prop)
	otel.SetTracerProvider(sdktrace.NewTracerProvider())

	egressBase := os.Getenv("EGRESS_PROXY_URL")
	if egressBase == "" {
		egressBase = "http://127.0.0.1:8084"
	}

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	var helloCount int64
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/count", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int64{"count": atomic.LoadInt64(&helloCount)})
	})
	mux.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt64(&helloCount, 0)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&helloCount, 1)

		ctx := r.Context()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, egressBase+"/fragment", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		depBody, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		var dep map[string]string
		depField := ""
		if json.Unmarshal(depBody, &dep) == nil {
			depField = dep["dep"]
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "hello",
			"dep":     depField,
		})
	})

	handler := otelhttp.NewHandler(mux, "e2e-app",
		otelhttp.WithPropagators(prop),
	)

	log.Fatal(http.ListenAndServe(":8081", handler))
}
