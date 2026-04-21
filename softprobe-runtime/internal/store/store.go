package store

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
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
	Extracts     [][]byte
	Stats        SessionStats
}

// SessionStats tracks per-session runtime counters without affecting revision.
type SessionStats struct {
	InjectedSpans  int
	ExtractedSpans int
	StrictMisses   int
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

// BufferExtract appends a captured extract payload without bumping the revision.
func (s *Store) BufferExtract(id string, payload []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return false
	}

	session.Extracts = append(session.Extracts, append([]byte(nil), payload...))
	s.sessions[id] = session
	return true
}

// RecordInjectedSpans increments the inject-hit counter without bumping revision.
func (s *Store) RecordInjectedSpans(id string, count int) (Session, bool) {
	return s.recordStats(id, func(stats *SessionStats) {
		stats.InjectedSpans += count
	})
}

// RecordExtractedSpans increments the accepted-extract counter without bumping revision.
func (s *Store) RecordExtractedSpans(id string, count int) (Session, bool) {
	return s.recordStats(id, func(stats *SessionStats) {
		stats.ExtractedSpans += count
	})
}

// RecordStrictMiss increments the strict-policy miss counter without bumping revision.
func (s *Store) RecordStrictMiss(id string, count int) (Session, bool) {
	return s.recordStats(id, func(stats *SessionStats) {
		stats.StrictMisses += count
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

func (s *Store) recordStats(id string, fn func(*SessionStats)) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return Session{}, false
	}

	fn(&session.Stats)
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
