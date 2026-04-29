package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"softprobe-runtime/internal/authn"
	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/datalake"
	"softprobe-runtime/internal/hostedbackend"
	"softprobe-runtime/internal/store"
)

const defaultListenAddr = "127.0.0.1:8080"

// newMux returns the HTTP mux. Hosted mode activates automatically when
// SOFTPROBE_AUTH_URL, REDIS_HOST, and DATALAKE_URL are all set. Otherwise
// the OSS in-memory mux is used (local / self-hosted).
func newMux() http.Handler {
	level := controlapi.ParseLogLevel(os.Getenv("SOFTPROBE_LOG_LEVEL"))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	if os.Getenv("SOFTPROBE_AUTH_URL") == "" || os.Getenv("REDIS_HOST") == "" || os.Getenv("DATALAKE_URL") == "" {
		return controlapi.NewMuxWithLogger(store.NewStore(), logger)
	}
	return newHostedMux()
}

func newHostedMux() http.Handler {
	authURL := requireEnv("SOFTPROBE_AUTH_URL")
	redisAddr := fmt.Sprintf("%s:%s", requireEnv("REDIS_HOST"), envOrDefault("REDIS_PORT", "6379"))
	redisPassword := os.Getenv("REDIS_PASSWORD")
	datalakeURL := requireEnv("DATALAKE_URL")

	resolver := authn.NewResolver(authURL, 60*time.Second)

	// RedisStore is tenant-scoped per request; at startup we create a sentinel
	// store only to satisfy NewHostedMux. The real per-tenant store is created
	// per-request inside the middleware. For now use a single-tenant store
	// keyed by a placeholder — the hosted middleware injects the resolved tenant.
	// TODO: make Store construction lazy/per-tenant in a follow-up.
	st, err := store.NewRedisStore(redisAddr, redisPassword, "global", 24*time.Hour)
	if err != nil {
		fmt.Fprintf(os.Stderr, "softprobe-runtime: connect to Redis: %v\n", err)
		os.Exit(1)
	}

	datalakeClient := datalake.NewClient(datalakeURL)

	overrides := &controlapi.SessionCommandOverrides{
		Close:    hostedbackend.HandleClose(st),
		LoadCase: hostedbackend.HandleLoadCase(st),
		Traces:   hostedbackend.HandleTraces(st, datalakeClient),
	}
	inner := http.NewServeMux()
	inner.Handle("/v1/captures/", hostedbackend.NewHostedEndpoints(st, datalakeClient))
	inner.Handle("/", controlapi.NewMuxWithOverrides(overrides, st))
	return controlapi.NewHostedAuthMux(resolver, inner)
}

func listenAddr() string {
	if addr := os.Getenv("SOFTPROBE_LISTEN_ADDR"); addr != "" {
		return addr
	}
	return defaultListenAddr
}

func requireEnv(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	fmt.Fprintf(os.Stderr, "softprobe-runtime: required env var %s is not set\n", key)
	os.Exit(1)
	return ""
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
