package main

import (
	"net/http"
	"os"

	"softprobe-runtime/internal/controlapi"
)

const defaultListenAddr = "127.0.0.1:8080"

// newMux returns the HTTP routes for the control runtime.
func newMux() *http.ServeMux { return controlapi.NewMux() }

// listenAddr returns the runtime bind address, defaulting to localhost.
func listenAddr() string {
	if addr := os.Getenv("SOFTPROBE_LISTEN_ADDR"); addr != "" {
		return addr
	}
	return defaultListenAddr
}
