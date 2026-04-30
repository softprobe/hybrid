package store_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"softprobe-runtime/internal/store"
)

// Compile-time assertion: RedisStore implements Store.
var _ store.Store = (*store.RedisStore)(nil)

func newTestRedisStore(t *testing.T, tenantID string) *store.RedisStore {
	t.Helper()
	mr := miniredis.RunT(t)
	st, err := store.NewRedisStore(mr.Addr(), "", tenantID, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	return st
}

func TestRedisStore_CreateAndGet(t *testing.T) {
	st := newTestRedisStore(t, "tenant1")
	sess := st.Create("replay")
	if sess.ID == "" {
		t.Fatal("Create returned empty ID")
	}
	if sess.Mode != "replay" {
		t.Errorf("Mode = %q, want replay", sess.Mode)
	}
	got, ok := st.Get(sess.ID)
	if !ok {
		t.Fatal("Get returned false after Create")
	}
	if got.ID != sess.ID || got.Mode != sess.Mode {
		t.Errorf("Get mismatch: %+v", got)
	}
}

func TestRedisStore_GetUnknown(t *testing.T) {
	st := newTestRedisStore(t, "tenant1")
	_, ok := st.Get("sess_doesnotexist")
	if ok {
		t.Fatal("Get should return false for unknown session")
	}
}

func TestRedisStore_LoadCase_BumpsRevision(t *testing.T) {
	st := newTestRedisStore(t, "tenant1")
	sess := st.Create("replay")

	updated, ok := st.LoadCase(sess.ID, []byte(`{"caseId":"demo"}`))
	if !ok {
		t.Fatal("LoadCase returned false")
	}
	if updated.Revision != 1 {
		t.Errorf("Revision = %d, want 1", updated.Revision)
	}

	got, _ := st.Get(sess.ID)
	if string(got.LoadedCase) != `{"caseId":"demo"}` {
		t.Errorf("LoadedCase = %q", got.LoadedCase)
	}
}

func TestRedisStore_ApplyPolicy(t *testing.T) {
	st := newTestRedisStore(t, "tenant1")
	sess := st.Create("replay")
	updated, ok := st.ApplyPolicy(sess.ID, []byte(`{"externalHttp":"strict"}`))
	if !ok || updated.Revision != 1 {
		t.Fatalf("ApplyPolicy: ok=%v revision=%d", ok, updated.Revision)
	}
}

func TestRedisStore_ApplyRules(t *testing.T) {
	st := newTestRedisStore(t, "tenant1")
	sess := st.Create("replay")
	updated, ok := st.ApplyRules(sess.ID, []byte(`{"version":1,"rules":[]}`))
	if !ok || updated.Revision != 1 {
		t.Fatalf("ApplyRules: ok=%v revision=%d", ok, updated.Revision)
	}
}

func TestRedisStore_ApplyFixturesAuth(t *testing.T) {
	st := newTestRedisStore(t, "tenant1")
	sess := st.Create("replay")
	updated, ok := st.ApplyFixturesAuth(sess.ID, []byte(`{"token":"abc"}`))
	if !ok || updated.Revision != 1 {
		t.Fatalf("ApplyFixturesAuth: ok=%v revision=%d", ok, updated.Revision)
	}
}

func TestRedisStore_MultipleRevisions(t *testing.T) {
	st := newTestRedisStore(t, "tenant1")
	sess := st.Create("replay")
	st.LoadCase(sess.ID, []byte(`{}`))
	st.ApplyPolicy(sess.ID, []byte(`{}`))
	updated, _ := st.ApplyRules(sess.ID, []byte(`{}`))
	if updated.Revision != 3 {
		t.Errorf("Revision = %d, want 3", updated.Revision)
	}
}

func TestRedisStore_BufferExtract(t *testing.T) {
	st := newTestRedisStore(t, "tenant1")
	sess := st.Create("capture")
	if !st.BufferExtract(sess.ID, []byte(`otlp1`)) {
		t.Fatal("BufferExtract returned false")
	}
	if !st.BufferExtract(sess.ID, []byte(`otlp2`)) {
		t.Fatal("BufferExtract returned false")
	}
	paths := st.GetExtractRefs(sess.ID)
	if len(paths) != 2 {
		t.Errorf("GetExtractRefs len = %d, want 2", len(paths))
	}
}

func TestRedisStore_Close(t *testing.T) {
	st := newTestRedisStore(t, "tenant1")
	sess := st.Create("replay")
	if !st.Close(sess.ID) {
		t.Fatal("Close returned false")
	}
	if _, ok := st.Get(sess.ID); ok {
		t.Fatal("session still present after Close")
	}
}

func TestRedisStore_CloseUnknown(t *testing.T) {
	st := newTestRedisStore(t, "tenant1")
	if st.Close("sess_unknown") {
		t.Fatal("Close should return false for unknown session")
	}
}

func TestRedisStore_Stats(t *testing.T) {
	st := newTestRedisStore(t, "tenant1")
	sess := st.Create("replay")
	st.RecordInjectedSpans(sess.ID, 3)
	st.RecordExtractedSpans(sess.ID, 5)
	st.RecordStrictMiss(sess.ID, 1)
	got, ok := st.Get(sess.ID)
	if !ok {
		t.Fatal("Get after stats failed")
	}
	if got.Stats.InjectedSpans != 3 {
		t.Errorf("InjectedSpans = %d, want 3", got.Stats.InjectedSpans)
	}
	if got.Stats.ExtractedSpans != 5 {
		t.Errorf("ExtractedSpans = %d, want 5", got.Stats.ExtractedSpans)
	}
	if got.Stats.StrictMisses != 1 {
		t.Errorf("StrictMisses = %d, want 1", got.Stats.StrictMisses)
	}
	// Stats must NOT bump revision
	if got.Revision != 0 {
		t.Errorf("Revision after stats = %d, want 0", got.Revision)
	}
}

func TestRedisStore_TenantIsolation(t *testing.T) {
	mr := miniredis.RunT(t)
	st1, _ := store.NewRedisStore(mr.Addr(), "", "tenantA", 24*time.Hour)
	st2, _ := store.NewRedisStore(mr.Addr(), "", "tenantB", 24*time.Hour)

	sess := st1.Create("replay")
	// tenantB cannot see tenantA's session
	_, ok := st2.Get(sess.ID)
	if ok {
		t.Fatal("tenantB should not see tenantA session")
	}
}
