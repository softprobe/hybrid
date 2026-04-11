package main

import (
	"net/http"

	"softprobe-runtime/internal/runtimeapp"
)

const defaultListenAddr = "127.0.0.1:8080"

// newMux returns the HTTP routes for the control runtime.
func newMux() *http.ServeMux { return runtimeapp.NewMux() }
