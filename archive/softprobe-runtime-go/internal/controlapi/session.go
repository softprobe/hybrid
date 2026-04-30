package controlapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"softprobe-runtime/internal/metrics"
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

// handleSessions dispatches POST (create) and GET (list) for /v1/sessions.
func handleSessions(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handleCreateSession(st).ServeHTTP(w, r)
		case http.MethodGet:
			handleListSessions(st).ServeHTTP(w, r)
		default:
			writeMethodNotAllowedError(w)
		}
	}
}

func handleCreateSession(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Mode == "" {
			writeInvalidRequestError(w, "invalid create session request")
			return
		}

		session := st.Create(req.Mode)
		metrics.Global.SessionsTotal.Inc(session.Mode)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(createSessionResponse{
			SessionID:       session.ID,
			SessionRevision: 0,
		})
	}
}

func handleListSessions(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type sessionSummary struct {
			ID       string `json:"sessionId"`
			Mode     string `json:"mode"`
			Revision int    `json:"sessionRevision"`
		}
		sessions := st.List()
		result := make([]sessionSummary, 0, len(sessions))
		for _, s := range sessions {
			result = append(result, sessionSummary{ID: s.ID, Mode: s.Mode, Revision: s.Revision})
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"sessions": result})
	}
}

// handleSessionCommandWithOverrides is like handleSessionCommand but replaces close/load-case
// with hosted-backend handlers when provided.
func handleSessionCommandWithOverrides(st store.Store, ov *SessionCommandOverrides) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		switch {
		case strings.HasSuffix(path, "/close"):
			if r.Method != http.MethodPost {
				writeMethodNotAllowedError(w)
				return
			}
			id := strings.TrimSuffix(path, "/close")
			if ov.Close != nil {
				ov.Close(w, r, id)
			} else {
				handleCloseSession(st, id)(w, r)
			}
		case strings.HasSuffix(path, "/load-case"):
			if r.Method != http.MethodPost {
				writeMethodNotAllowedError(w)
				return
			}
			id := strings.TrimSuffix(path, "/load-case")
			if ov.LoadCase != nil {
				ov.LoadCase(w, r, id)
			} else {
				handleLoadCase(st, id)(w, r)
			}
		default:
			handleSessionCommand(st)(w, r)
		}
	}
}

func handleSessionCommand(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		switch {
		case strings.HasSuffix(path, "/stats"):
			if r.Method != http.MethodGet {
				writeMethodNotAllowedError(w)
				return
			}
			handleSessionStats(st, strings.TrimSuffix(path, "/stats"))(w, r)
		case strings.HasSuffix(path, "/state"):
			if r.Method != http.MethodGet {
				writeMethodNotAllowedError(w)
				return
			}
			handleSessionState(st, strings.TrimSuffix(path, "/state"))(w, r)
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

// handleSessionState returns the composite session snapshot consumed by
// `softprobe inspect session`. The payload intentionally mirrors what a CI
// operator needs at a glance: mode, revision, policy, rules, case summary,
// and live counters. No persistent state is stored to a disk — the store is
// the source of truth.
func handleSessionState(st store.Store, sessionID string) http.HandlerFunc {
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

		out := map[string]any{
			"sessionId":       session.ID,
			"sessionRevision": session.Revision,
			"mode":            session.Mode,
			"caseSummary":     summarizeLoadedCase(session.LoadedCase),
			"stats": map[string]int{
				"injectedSpans":  session.Stats.InjectedSpans,
				"extractedSpans": session.Stats.ExtractedSpans,
				"strictMisses":   session.Stats.StrictMisses,
			},
		}
		if len(session.Policy) > 0 {
			out["policy"] = json.RawMessage(session.Policy)
		}
		if len(session.Rules) > 0 {
			out["rules"] = json.RawMessage(session.Rules)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	}
}

// summarizeLoadedCase returns a cheap summary of a stored case payload so
// that `inspect session` can render a caseId + trace count without pulling
// the whole document across the wire.
func summarizeLoadedCase(loadedCase []byte) map[string]any {
	summary := map[string]any{"traceCount": 0}
	if len(loadedCase) == 0 {
		return summary
	}
	var doc struct {
		CaseID string        `json:"caseId"`
		Traces []interface{} `json:"traces"`
	}
	if err := json.Unmarshal(loadedCase, &doc); err != nil {
		return summary
	}
	if doc.CaseID != "" {
		summary["caseId"] = doc.CaseID
	}
	summary["traceCount"] = len(doc.Traces)
	return summary
}

func handleSessionStats(st store.Store, sessionID string) http.HandlerFunc {
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

func handleCloseSession(st store.Store, sessionID string) http.HandlerFunc {
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

		var capturePath string
		if session.Mode == "capture" {
			override := strings.TrimSpace(r.URL.Query().Get("out"))
			path, err := proxybackend.WriteCapturedCaseTo(session.ID, session.Extracts, override)
			if err != nil {
				writeAPIError(w, http.StatusInternalServerError, "internal_error", "write captured case failed")
				return
			}
			capturePath = path
		}

		if !st.Close(sessionID) {
			writeUnknownSessionError(w)
			return
		}

		resp := map[string]any{
			"sessionId": sessionID,
			"closed":    true,
		}
		if capturePath != "" {
			resp["capturePath"] = capturePath
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func handleLoadCase(st store.Store, sessionID string) http.HandlerFunc {
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

func handlePolicy(st store.Store, sessionID string) http.HandlerFunc {
	return handleMutatingPayload(st, sessionID, func(id string, payload []byte) (store.Session, bool) {
		return st.ApplyPolicy(id, payload)
	})
}

func handleRules(st store.Store, sessionID string) http.HandlerFunc {
	return handleMutatingPayload(st, sessionID, func(id string, payload []byte) (store.Session, bool) {
		return st.ApplyRules(id, payload)
	})
}

func handleFixturesAuth(st store.Store, sessionID string) http.HandlerFunc {
	return handleMutatingPayload(st, sessionID, func(id string, payload []byte) (store.Session, bool) {
		return st.ApplyFixturesAuth(id, payload)
	})
}

func handleMutatingPayload(st store.Store, sessionID string, update func(string, []byte) (store.Session, bool)) http.HandlerFunc {
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
