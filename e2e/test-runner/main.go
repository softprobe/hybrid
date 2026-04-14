package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	runtimeURL := mustEnv("RUNTIME_URL", "http://softprobe-runtime:8080")
	proxyURL := mustEnv("PROXY_URL", "http://softprobe-proxy:8082")
	appURL := mustEnv("APP_URL", "http://app:8081")
	upstreamURL := mustEnv("UPSTREAM_URL", "http://upstream:8083")

	client := &http.Client{Timeout: 5 * time.Second}
	check := func(name, url string) {
		for i := 0; i < 30; i++ {
			resp, err := client.Get(url + "/health")
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return
				}
				body, _ := io.ReadAll(resp.Body)
				log.Printf("%s health status = %d: %s", name, resp.StatusCode, string(body))
			} else {
				log.Printf("%s health failed: %v", name, err)
			}
			time.Sleep(1 * time.Second)
		}
		log.Fatalf("%s health never became ready", name)
	}

	check("runtime", runtimeURL)
	check("proxy", proxyURL)
	check("app", appURL)
	check("upstream", upstreamURL)

	resp, err := client.Get(proxyURL + "/hello")
	if err != nil {
		log.Fatalf("proxy hello failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("proxy hello status = %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		log.Fatalf("decode proxy hello: %v", err)
	}
	if body["message"] != "hello" || body["dep"] != "ok" {
		log.Fatalf("proxy hello body = %+v, want message=hello and dep=ok", body)
	}

	fmt.Println("ok")
	_ = os.WriteFile("/tmp/e2e-ok", []byte("ok"), 0o600)
}

func mustEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
