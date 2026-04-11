package runtimeapp

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Session is the in-memory control-runtime session record.
type Session struct {
	ID           string
	Mode         string
	Revision     int
	LoadedCase   []byte
	Policy       []byte
	Rules        []byte
	FixturesAuth []byte
}

// Store keeps control-runtime sessions in memory.
type Store struct {
	mu       sync.Mutex
	sessions map[string]Session
}

// NewStore creates an empty in-memory session store.
func NewStore() *Store {
	return &Store{sessions: make(map[string]Session)}
}

// Create stores a new session and returns it.
func (s *Store) Create(mode string) Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := Session{
		ID:       newSessionID(),
		Mode:     mode,
		Revision: 0,
	}
	s.sessions[session.ID] = session
	return session
}

// Get returns a session by ID.
func (s *Store) Get(id string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	return session, ok
}

// Close removes a session from the store.
func (s *Store) Close(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return false
	}
	delete(s.sessions, id)
	return true
}

// LoadCase replaces the stored case payload and bumps the revision.
func (s *Store) LoadCase(id string, loadedCase []byte) (Session, bool) {
	return s.mutate(id, func(session *Session) {
		session.LoadedCase = append([]byte(nil), loadedCase...)
	})
}

// ApplyPolicy replaces the stored policy payload and bumps the revision.
func (s *Store) ApplyPolicy(id string, policy []byte) (Session, bool) {
	return s.mutate(id, func(session *Session) {
		session.Policy = append([]byte(nil), policy...)
	})
}

// ApplyRules replaces the stored rules payload and bumps the revision.
func (s *Store) ApplyRules(id string, rules []byte) (Session, bool) {
	return s.mutate(id, func(session *Session) {
		session.Rules = append([]byte(nil), rules...)
	})
}

// ApplyFixturesAuth replaces the stored auth fixtures payload and bumps the revision.
func (s *Store) ApplyFixturesAuth(id string, fixtures []byte) (Session, bool) {
	return s.mutate(id, func(session *Session) {
		session.FixturesAuth = append([]byte(nil), fixtures...)
	})
}

func (s *Store) mutate(id string, fn func(*Session)) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return Session{}, false
	}

	session.Revision++
	fn(&session)
	s.sessions[id] = session
	return session, true
}

func newSessionID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Errorf("generate session id: %w", err))
	}
	return "sess_" + base64.RawURLEncoding.EncodeToString(raw[:])
}

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

func handleCreateSession(store *Store) http.HandlerFunc {
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

		session := store.Create(req.Mode)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(createSessionResponse{
			SessionID:       session.ID,
			SessionRevision: 0,
		})
	}
}

func handleSessionCommand(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		switch {
		case strings.HasSuffix(path, "/close"):
			handleCloseSession(store, strings.TrimSuffix(path, "/close"))(w, r)
		case strings.HasSuffix(path, "/load-case"):
			handleLoadCase(store, strings.TrimSuffix(path, "/load-case"))(w, r)
		case strings.HasSuffix(path, "/policy"):
			handlePolicy(store, strings.TrimSuffix(path, "/policy"))(w, r)
		case strings.HasSuffix(path, "/rules"):
			handleRules(store, strings.TrimSuffix(path, "/rules"))(w, r)
		case strings.HasSuffix(path, "/fixtures/auth"):
			handleFixturesAuth(store, strings.TrimSuffix(path, "/fixtures/auth"))(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func handleCloseSession(store *Store, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID = strings.TrimSuffix(sessionID, "/")
		if sessionID == "" {
			writeUnknownSessionError(w)
			return
		}

		if !store.Close(sessionID) {
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

func handleLoadCase(store *Store, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID = strings.TrimSuffix(sessionID, "/")
		if sessionID == "" {
			writeUnknownSessionError(w)
			return
		}

		if _, ok := store.Get(sessionID); !ok {
			writeUnknownSessionError(w)
			return
		}

		loadedCase, err := io.ReadAll(r.Body)
		if err != nil || len(loadedCase) == 0 {
			http.Error(w, "invalid load-case request", http.StatusBadRequest)
			return
		}

		session, ok := store.LoadCase(sessionID, loadedCase)
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

func handlePolicy(store *Store, sessionID string) http.HandlerFunc {
	return handleMutatingPayload(store, sessionID, func(id string, payload []byte) (Session, bool) {
		return store.ApplyPolicy(id, payload)
	})
}

func handleRules(store *Store, sessionID string) http.HandlerFunc {
	return handleMutatingPayload(store, sessionID, func(id string, payload []byte) (Session, bool) {
		return store.ApplyRules(id, payload)
	})
}

func handleFixturesAuth(store *Store, sessionID string) http.HandlerFunc {
	return handleMutatingPayload(store, sessionID, func(id string, payload []byte) (Session, bool) {
		return store.ApplyFixturesAuth(id, payload)
	})
}

func handleMutatingPayload(store *Store, sessionID string, update func(string, []byte) (Session, bool)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID = strings.TrimSuffix(sessionID, "/")
		if sessionID == "" {
			writeUnknownSessionError(w)
			return
		}

		if _, ok := store.Get(sessionID); !ok {
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
