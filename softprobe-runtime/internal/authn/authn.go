// Package authn resolves API keys to tenant identity by calling auth.softprobe.ai.
package authn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// TenantInfo is the resolved identity for an API key.
type TenantInfo struct {
	TenantID   string
	BucketName string
	DatasetID  string
}

// Resolver validates API keys against the auth service and caches results.
type Resolver struct {
	url    string
	ttl    time.Duration
	mu     sync.Mutex
	cache  map[string]cacheEntry
	client *http.Client
}

type cacheEntry struct {
	info      TenantInfo
	expiresAt time.Time
}

// NewResolver returns a Resolver that calls authURL for validation and caches results for ttl.
func NewResolver(authURL string, ttl time.Duration) *Resolver {
	return &Resolver{
		url:    authURL,
		ttl:    ttl,
		cache:  make(map[string]cacheEntry),
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Resolve returns the TenantInfo for the given API key.
// Returns an error if the key is empty, invalid, or the auth service is unreachable.
func (r *Resolver) Resolve(ctx context.Context, apiKey string) (TenantInfo, error) {
	if apiKey == "" {
		return TenantInfo{}, fmt.Errorf("authn: empty API key")
	}

	r.mu.Lock()
	if e, ok := r.cache[apiKey]; ok && time.Now().Before(e.expiresAt) {
		info := e.info
		r.mu.Unlock()
		return info, nil
	}
	r.mu.Unlock()

	info, err := r.callAuthService(ctx, apiKey)
	if err != nil {
		return TenantInfo{}, err
	}

	r.mu.Lock()
	r.cache[apiKey] = cacheEntry{info: info, expiresAt: time.Now().Add(r.ttl)}
	r.mu.Unlock()

	return info, nil
}

func (r *Resolver) callAuthService(ctx context.Context, apiKey string) (TenantInfo, error) {
	body, _ := json.Marshal(map[string]string{"apiKey": apiKey})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return TenantInfo{}, fmt.Errorf("authn: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return TenantInfo{}, fmt.Errorf("authn: auth service unreachable: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Success bool `json:"success"`
		Data    *struct {
			TenantID  string `json:"tenantId"`
			Resources []struct {
				ResourceType string `json:"resourceType"`
				ConfigJSON   string `json:"configJson"`
			} `json:"resources"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return TenantInfo{}, fmt.Errorf("authn: decode response: %w", err)
	}
	if !payload.Success || payload.Data == nil {
		return TenantInfo{}, fmt.Errorf("authn: invalid API key")
	}

	info := TenantInfo{TenantID: payload.Data.TenantID}
	for _, res := range payload.Data.Resources {
		if res.ResourceType != "BIGQUERY_DATASET" && res.ResourceType != "BIGQUERY_STORAGE" {
			continue
		}
		var cfg struct {
			DatasetID  string `json:"dataset_id"`
			BucketName string `json:"bucket_name"`
		}
		if err := json.Unmarshal([]byte(res.ConfigJSON), &cfg); err != nil {
			continue
		}
		info.DatasetID = cfg.DatasetID
		info.BucketName = cfg.BucketName
		break
	}

	if info.TenantID == "" {
		return TenantInfo{}, fmt.Errorf("authn: auth response missing tenantId")
	}
	return info, nil
}
