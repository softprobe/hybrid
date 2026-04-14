package controlapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"softprobe-runtime/internal/proxybackend"
	"softprobe-runtime/internal/store"
)

type createSessionRequest struct {
	Mode string `json:"mode"`
}

type createSessionResponse struct {
	SessionID       string `json:"sessionId"`
	SessionRevision int    `json:"sessionRevision"`
}

type loadCaseResponse struct {
	SessionID       string `json:"sessionId"`
	SessionRevision int    `json:"sessionRevision"`
}

func handleCreateSession(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req createSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Mode == "" {
			http.Error(w, "invalid create session request", http.StatusBadRequest)
			return
		}

		session := st.Create(req.Mode)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(createSessionResponse{
			SessionID:       session.ID,
			SessionRevision: 0,
		})
	}
}

func handleSessionCommand(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		switch {
		case strings.HasSuffix(path, "/close"):
			handleCloseSession(st, strings.TrimSuffix(path, "/close"))(w, r)
		case strings.HasSuffix(path, "/load-case"):
			handleLoadCase(st, strings.TrimSuffix(path, "/load-case"))(w, r)
		case strings.HasSuffix(path, "/policy"):
			handlePolicy(st, strings.TrimSuffix(path, "/policy"))(w, r)
		case strings.HasSuffix(path, "/rules"):
			handleRules(st, strings.TrimSuffix(path, "/rules"))(w, r)
		case strings.HasSuffix(path, "/fixtures/auth"):
			handleFixturesAuth(st, strings.TrimSuffix(path, "/fixtures/auth"))(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func handleCloseSession(st *store.Store, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID = strings.TrimSuffix(sessionID, "/")
		if sessionID == "" {
			writeUnknownSessionError(w)
			return
		}

		session, ok := st.Get(sessionID)
		if !ok {
			writeUnknownSessionError(w)
			return
		}

		if session.Mode == "capture" {
			if err := proxybackend.WriteCapturedCase(session.ID, session.Extracts); err != nil {
				http.Error(w, "write captured case failed", http.StatusInternalServerError)
				return
			}
		}

		if !st.Close(sessionID) {
			writeUnknownSessionError(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sessionId": sessionID,
			"closed":    true,
		})
	}
}

func handleLoadCase(st *store.Store, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID = strings.TrimSuffix(sessionID, "/")
		if sessionID == "" {
			writeUnknownSessionError(w)
			return
		}

		if _, ok := st.Get(sessionID); !ok {
			writeUnknownSessionError(w)
			return
		}

		loadedCase, err := io.ReadAll(r.Body)
		if err != nil || len(loadedCase) == 0 {
			http.Error(w, "invalid load-case request", http.StatusBadRequest)
			return
		}

		session, ok := st.LoadCase(sessionID, loadedCase)
		if !ok {
			writeUnknownSessionError(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(loadCaseResponse{
			SessionID:       session.ID,
			SessionRevision: session.Revision,
		})
	}
}

func handlePolicy(st *store.Store, sessionID string) http.HandlerFunc {
	return handleMutatingPayload(st, sessionID, func(id string, payload []byte) (store.Session, bool) {
		return st.ApplyPolicy(id, payload)
	})
}

func handleRules(st *store.Store, sessionID string) http.HandlerFunc {
	return handleMutatingPayload(st, sessionID, func(id string, payload []byte) (store.Session, bool) {
		return st.ApplyRules(id, payload)
	})
}

func handleFixturesAuth(st *store.Store, sessionID string) http.HandlerFunc {
	return handleMutatingPayload(st, sessionID, func(id string, payload []byte) (store.Session, bool) {
		return st.ApplyFixturesAuth(id, payload)
	})
}

func handleMutatingPayload(st *store.Store, sessionID string, update func(string, []byte) (store.Session, bool)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID = strings.TrimSuffix(sessionID, "/")
		if sessionID == "" {
			writeUnknownSessionError(w)
			return
		}

		if _, ok := st.Get(sessionID); !ok {
			writeUnknownSessionError(w)
			return
		}

		payload, err := io.ReadAll(r.Body)
		if err != nil || len(payload) == 0 {
			http.Error(w, "invalid control payload", http.StatusBadRequest)
			return
		}

		session, ok := update(sessionID, payload)
		if !ok {
			writeUnknownSessionError(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(loadCaseResponse{
			SessionID:       session.ID,
			SessionRevision: session.Revision,
		})
	}
}

func writeUnknownSessionError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    "unknown_session",
			"message": "unknown session",
		},
	})
}
