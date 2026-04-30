package controlapi

import (
	"context"
	"net/http"
	"strings"

	"softprobe-runtime/internal/authn"
	"softprobe-runtime/internal/store"
)

type contextKey int

const tenantKey contextKey = iota

// WithTenant returns a context carrying the resolved tenant info.
func WithTenant(ctx context.Context, info authn.TenantInfo) context.Context {
	return context.WithValue(ctx, tenantKey, info)
}

// TenantFromContext extracts TenantInfo injected by the hosted auth middleware.
func TenantFromContext(ctx context.Context) (authn.TenantInfo, bool) {
	info, ok := ctx.Value(tenantKey).(authn.TenantInfo)
	return info, ok
}

// NewHostedMux returns an HTTP mux identical to NewMux but with multi-tenant bearer
// auth on all /v1/* routes. /health remains unauthenticated.
func NewHostedMux(resolver *authn.Resolver, stores ...store.Store) *http.ServeMux {
	mux := NewMux(stores...)
	return withHostedAuth(resolver, mux)
}

// NewHostedAuthMux wraps an arbitrary handler with the hosted bearer-auth middleware.
// /health is passed through without auth; all /v1/* paths require a valid bearer token.
func NewHostedAuthMux(resolver *authn.Resolver, inner http.Handler) *http.ServeMux {
	return withHostedAuth(resolver, inner)
}

// withHostedAuth wraps every request to /v1/* with tenant resolution.
// /health is passed through without auth.
func withHostedAuth(resolver *authn.Resolver, next http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
	mux.Handle("/", hostedAuthMiddleware(resolver, next))
	return mux
}

func hostedAuthMiddleware(resolver *authn.Resolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only protect /v1/* paths.
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		token, ok := bearerTokenFromHeader(r.Header.Get("Authorization"))
		if !ok {
			// Compatibility path for proxy WASM until it switches to bearer auth.
			if k := strings.TrimSpace(r.Header.Get("x-public-key")); k != "" {
				token, ok = k, true
			}
		}
		if !ok {
			writeAPIError(w, http.StatusUnauthorized, "missing_bearer_token", "missing bearer token")
			return
		}

		info, err := resolver.Resolve(r.Context(), token)
		if err != nil {
			writeAPIError(w, http.StatusForbidden, "invalid_api_key", "invalid API key")
			return
		}

		next.ServeHTTP(w, r.WithContext(WithTenant(r.Context(), info)))
	})
}
