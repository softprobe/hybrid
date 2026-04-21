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

type sessionStatsResponse struct {
	SessionID       string `json:"sessionId"`
	SessionRevision int    `json:"sessionRevision"`
	Mode            string `json:"mode"`
	Stats           struct {
		InjectedSpans  int `json:"injectedSpans"`
		ExtractedSpans int `json:"extractedSpans"`
		StrictMisses   int `json:"strictMisses"`
	} `json:"stats"`
}

func handleCreateSession(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedError(w)
			return
		}

		var req createSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Mode == "" {
			writeInvalidRequestError(w, "invalid create session request")
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
		path := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		switch {
		case strings.HasSuffix(path, "/stats"):
			if r.Method != http.MethodGet {
				writeMethodNotAllowedError(w)
				return
			}
			handleSessionStats(st, strings.TrimSuffix(path, "/stats"))(w, r)
		case strings.HasSuffix(path, "/close"):
			if r.Method != http.MethodPost {
				writeMethodNotAllowedError(w)
				return
			}
			handleCloseSession(st, strings.TrimSuffix(path, "/close"))(w, r)
		case strings.HasSuffix(path, "/load-case"):
			if r.Method != http.MethodPost {
				writeMethodNotAllowedError(w)
				return
			}
			handleLoadCase(st, strings.TrimSuffix(path, "/load-case"))(w, r)
		case strings.HasSuffix(path, "/policy"):
			if r.Method != http.MethodPost {
				writeMethodNotAllowedError(w)
				return
			}
			handlePolicy(st, strings.TrimSuffix(path, "/policy"))(w, r)
		case strings.HasSuffix(path, "/rules"):
			if r.Method != http.MethodPost {
				writeMethodNotAllowedError(w)
				return
			}
			handleRules(st, strings.TrimSuffix(path, "/rules"))(w, r)
		case strings.HasSuffix(path, "/fixtures/auth"):
			if r.Method != http.MethodPost {
				writeMethodNotAllowedError(w)
				return
			}
			handleFixturesAuth(st, strings.TrimSuffix(path, "/fixtures/auth"))(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func handleSessionStats(st *store.Store, sessionID string) http.HandlerFunc {
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

		var resp sessionStatsResponse
		resp.SessionID = session.ID
		resp.SessionRevision = session.Revision
		resp.Mode = session.Mode
		resp.Stats.InjectedSpans = session.Stats.InjectedSpans
		resp.Stats.ExtractedSpans = session.Stats.ExtractedSpans
		resp.Stats.StrictMisses = session.Stats.StrictMisses

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
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
				writeAPIError(w, http.StatusInternalServerError, "internal_error", "write captured case failed")
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
			writeInvalidRequestError(w, "invalid load-case request")
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
			writeInvalidRequestError(w, "invalid control payload")
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
