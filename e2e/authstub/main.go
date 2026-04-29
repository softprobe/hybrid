package main

import (
	"encoding/json"
	"net/http"
)

type validateReq struct {
	APIKey string `json:"apiKey"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req validateReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.APIKey == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"tenantId": "tenant-local-e2e",
				"resources": []map[string]any{
					{
						"resourceType": "BIGQUERY_DATASET",
						"configJson":   `{"dataset_id":"local","bucket_name":"local"}`,
					},
				},
			},
		})
	})

	_ = http.ListenAndServe(":8091", mux)
}
