package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore persists sessions in Redis under tenant-scoped keys.
// Session document: session:{tenantID}:{sessionID} → JSON
// Extract list:     session:{tenantID}:{sessionID}:extracts → Redis list of strings
type RedisStore struct {
	rdb      *redis.Client
	tenantID string
	ttl      time.Duration
}

// sessionDoc is the JSON shape stored in Redis. Extracts are stored separately
// in a Redis list to avoid loading large blobs on every read.
type sessionDoc struct {
	ID           string       `json:"id"`
	TenantID     string       `json:"tenantId"`
	Mode         string       `json:"mode"`
	Revision     int          `json:"revision"`
	LoadedCase   []byte       `json:"loadedCase,omitempty"`
	Policy       []byte       `json:"policy,omitempty"`
	Rules        []byte       `json:"rules,omitempty"`
	FixturesAuth []byte       `json:"fixturesAuth,omitempty"`
	Stats        SessionStats `json:"stats"`
}

// NewRedisStore returns a RedisStore for the given tenant.
// addr is "host:port", password may be empty.
func NewRedisStore(addr, password, tenantID string, ttl time.Duration) (*RedisStore, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis: connect to %s: %w", addr, err)
	}
	return &RedisStore{rdb: rdb, tenantID: tenantID, ttl: ttl}, nil
}

func (s *RedisStore) sessionKey(id string) string {
	return fmt.Sprintf("session:%s:%s", s.tenantID, id)
}

func (s *RedisStore) extractsKey(id string) string {
	return fmt.Sprintf("session:%s:%s:extracts", s.tenantID, id)
}

func (s *RedisStore) Create(mode string) Session {
	doc := sessionDoc{
		ID:       newSessionID(),
		TenantID: s.tenantID,
		Mode:     mode,
	}
	s.save(context.Background(), &doc)
	return docToSession(doc)
}

func (s *RedisStore) Get(id string) (Session, bool) {
	doc, ok := s.load(context.Background(), id)
	if !ok {
		return Session{}, false
	}
	return docToSession(*doc), true
}

func (s *RedisStore) Close(id string) bool {
	ctx := context.Background()
	key := s.sessionKey(id)
	n, err := s.rdb.Del(ctx, key, s.extractsKey(id)).Result()
	return err == nil && n > 0
}

func (s *RedisStore) LoadCase(id string, loadedCase []byte) (Session, bool) {
	return s.mutate(id, func(doc *sessionDoc) {
		doc.LoadedCase = append([]byte(nil), loadedCase...)
	})
}

func (s *RedisStore) ApplyPolicy(id string, policy []byte) (Session, bool) {
	return s.mutate(id, func(doc *sessionDoc) {
		doc.Policy = append([]byte(nil), policy...)
	})
}

func (s *RedisStore) ApplyRules(id string, rules []byte) (Session, bool) {
	return s.mutate(id, func(doc *sessionDoc) {
		doc.Rules = append([]byte(nil), rules...)
	})
}

func (s *RedisStore) ApplyFixturesAuth(id string, fixtures []byte) (Session, bool) {
	return s.mutate(id, func(doc *sessionDoc) {
		doc.FixturesAuth = append([]byte(nil), fixtures...)
	})
}

// BufferExtract appends a GCS object path (or raw payload key) to the extracts list.
// In the OSS path this stores raw bytes; in the hosted path the caller stores a
// GCS URI string and passes it here as the payload.
func (s *RedisStore) BufferExtract(id string, payload []byte) bool {
	ctx := context.Background()
	if _, ok := s.load(ctx, id); !ok {
		return false
	}
	key := s.extractsKey(id)
	if err := s.rdb.RPush(ctx, key, string(payload)).Err(); err != nil {
		return false
	}
	s.rdb.Expire(ctx, key, s.ttl)
	return true
}

// GetExtractRefs returns the list of extract paths/payloads for a session.
// This is a hosted-only helper not on the Store interface.
func (s *RedisStore) GetExtractRefs(id string) []string {
	ctx := context.Background()
	refs, _ := s.rdb.LRange(ctx, s.extractsKey(id), 0, -1).Result()
	return refs
}

// List satisfies the Store interface; delegates to ListSessions.
func (s *RedisStore) List() []Session { return s.ListSessions() }

// ListSessions returns all sessions for this tenant by scanning Redis keys.
func (s *RedisStore) ListSessions() []Session {
	ctx := context.Background()
	pattern := fmt.Sprintf("session:%s:*", s.tenantID)
	var sessions []Session
	iter := s.rdb.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		// Skip the extracts list keys
		if len(key) > len(":extracts") && key[len(key)-len(":extracts"):] == ":extracts" {
			continue
		}
		data, err := s.rdb.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var doc sessionDoc
		if err := json.Unmarshal(data, &doc); err != nil {
			continue
		}
		if doc.TenantID != s.tenantID {
			continue
		}
		sessions = append(sessions, docToSession(doc))
	}
	return sessions
}

func (s *RedisStore) RecordInjectedSpans(id string, count int) (Session, bool) {
	return s.updateStats(id, func(doc *sessionDoc) { doc.Stats.InjectedSpans += count })
}

func (s *RedisStore) RecordExtractedSpans(id string, count int) (Session, bool) {
	return s.updateStats(id, func(doc *sessionDoc) { doc.Stats.ExtractedSpans += count })
}

func (s *RedisStore) RecordStrictMiss(id string, count int) (Session, bool) {
	return s.updateStats(id, func(doc *sessionDoc) { doc.Stats.StrictMisses += count })
}

// mutate reads, applies fn (which increments revision), and saves atomically.
func (s *RedisStore) mutate(id string, fn func(*sessionDoc)) (Session, bool) {
	ctx := context.Background()
	key := s.sessionKey(id)

	for {
		if err := s.rdb.Watch(ctx, func(tx *redis.Tx) error {
			doc, ok := s.loadTx(ctx, tx, id)
			if !ok {
				return redis.TxFailedErr
			}
			doc.Revision++
			fn(doc)
			data, err := json.Marshal(doc)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, key, data, s.ttl)
				return nil
			})
			return err
		}, key); err == redis.TxFailedErr {
			continue // retry on optimistic lock failure
		} else if err != nil {
			return Session{}, false
		}
		break
	}

	doc, ok := s.load(ctx, id)
	if !ok {
		return Session{}, false
	}
	return docToSession(*doc), true
}

// updateStats writes stats without bumping revision.
func (s *RedisStore) updateStats(id string, fn func(*sessionDoc)) (Session, bool) {
	ctx := context.Background()
	key := s.sessionKey(id)

	for {
		if err := s.rdb.Watch(ctx, func(tx *redis.Tx) error {
			doc, ok := s.loadTx(ctx, tx, id)
			if !ok {
				return redis.TxFailedErr
			}
			fn(doc)
			data, err := json.Marshal(doc)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, key, data, s.ttl)
				return nil
			})
			return err
		}, key); err == redis.TxFailedErr {
			continue
		} else if err != nil {
			return Session{}, false
		}
		break
	}

	doc, ok := s.load(ctx, id)
	if !ok {
		return Session{}, false
	}
	return docToSession(*doc), true
}

func (s *RedisStore) load(ctx context.Context, id string) (*sessionDoc, bool) {
	data, err := s.rdb.Get(ctx, s.sessionKey(id)).Bytes()
	if err != nil {
		return nil, false
	}
	var doc sessionDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, false
	}
	// Enforce tenant isolation: reject sessions belonging to another tenant.
	if doc.TenantID != s.tenantID {
		return nil, false
	}
	return &doc, true
}

func (s *RedisStore) loadTx(ctx context.Context, tx *redis.Tx, id string) (*sessionDoc, bool) {
	data, err := tx.Get(ctx, s.sessionKey(id)).Bytes()
	if err != nil {
		return nil, false
	}
	var doc sessionDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, false
	}
	if doc.TenantID != s.tenantID {
		return nil, false
	}
	return &doc, true
}

func (s *RedisStore) save(ctx context.Context, doc *sessionDoc) {
	data, _ := json.Marshal(doc)
	s.rdb.Set(ctx, s.sessionKey(doc.ID), data, s.ttl)
}

func docToSession(doc sessionDoc) Session {
	return Session{
		ID:           doc.ID,
		Mode:         doc.Mode,
		Revision:     doc.Revision,
		LoadedCase:   doc.LoadedCase,
		Policy:       doc.Policy,
		Rules:        doc.Rules,
		FixturesAuth: doc.FixturesAuth,
		Stats:        doc.Stats,
	}
}
