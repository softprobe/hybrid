package store

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
)

// Store is the session persistence interface. MemoryStore is the OSS
// in-process implementation; RedisStore is the hosted implementation.
type Store interface {
	Create(mode string) Session
	Get(id string) (Session, bool)
	List() []Session
	Close(id string) bool
	LoadCase(id string, loadedCase []byte) (Session, bool)
	ApplyPolicy(id string, policy []byte) (Session, bool)
	ApplyRules(id string, rules []byte) (Session, bool)
	ApplyFixturesAuth(id string, fixtures []byte) (Session, bool)
	BufferExtract(id string, payload []byte) bool
	RecordInjectedSpans(id string, count int) (Session, bool)
	RecordExtractedSpans(id string, count int) (Session, bool)
	RecordStrictMiss(id string, count int) (Session, bool)
}

// Session is the control-runtime session record.
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

// MemoryStore keeps sessions in memory. It satisfies the Store interface.
type MemoryStore struct {
	mu       sync.Mutex
	sessions map[string]Session
}

// NewMemoryStore creates an empty in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: make(map[string]Session)}
}

// NewStore is an alias for NewMemoryStore kept for backwards compatibility
// within the package and existing tests.
func NewStore() *MemoryStore { return NewMemoryStore() }

func (s *MemoryStore) Create(mode string) Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := Session{ID: newSessionID(), Mode: mode, Revision: 0}
	s.sessions[session.ID] = session
	return session
}

func (s *MemoryStore) Get(id string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	return session, ok
}

func (s *MemoryStore) List() []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		result = append(result, sess)
	}
	return result
}

func (s *MemoryStore) Close(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return false
	}
	delete(s.sessions, id)
	return true
}

func (s *MemoryStore) LoadCase(id string, loadedCase []byte) (Session, bool) {
	return s.mutate(id, func(sess *Session) {
		sess.LoadedCase = append([]byte(nil), loadedCase...)
	})
}

func (s *MemoryStore) ApplyPolicy(id string, policy []byte) (Session, bool) {
	return s.mutate(id, func(sess *Session) {
		sess.Policy = append([]byte(nil), policy...)
	})
}

func (s *MemoryStore) ApplyRules(id string, rules []byte) (Session, bool) {
	return s.mutate(id, func(sess *Session) {
		sess.Rules = append([]byte(nil), rules...)
	})
}

func (s *MemoryStore) ApplyFixturesAuth(id string, fixtures []byte) (Session, bool) {
	return s.mutate(id, func(sess *Session) {
		sess.FixturesAuth = append([]byte(nil), fixtures...)
	})
}

func (s *MemoryStore) BufferExtract(id string, payload []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return false
	}
	sess.Extracts = append(sess.Extracts, append([]byte(nil), payload...))
	s.sessions[id] = sess
	return true
}

func (s *MemoryStore) RecordInjectedSpans(id string, count int) (Session, bool) {
	return s.recordStats(id, func(stats *SessionStats) { stats.InjectedSpans += count })
}

func (s *MemoryStore) RecordExtractedSpans(id string, count int) (Session, bool) {
	return s.recordStats(id, func(stats *SessionStats) { stats.ExtractedSpans += count })
}

func (s *MemoryStore) RecordStrictMiss(id string, count int) (Session, bool) {
	return s.recordStats(id, func(stats *SessionStats) { stats.StrictMisses += count })
}

func (s *MemoryStore) mutate(id string, fn func(*Session)) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, false
	}
	sess.Revision++
	fn(&sess)
	s.sessions[id] = sess
	return sess, true
}

func (s *MemoryStore) recordStats(id string, fn func(*SessionStats)) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, false
	}
	fn(&sess.Stats)
	s.sessions[id] = sess
	return sess, true
}

func newSessionID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Errorf("generate session id: %w", err))
	}
	return "sess_" + base64.RawURLEncoding.EncodeToString(raw[:])
}
