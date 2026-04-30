package store_test

import (
	"testing"

	"softprobe-runtime/internal/store"
)

// Compile-time assertion: MemoryStore implements Store.
var _ store.Store = (*store.MemoryStore)(nil)

func TestMemoryStoreImplementsInterface(t *testing.T) {
	var st store.Store = store.NewMemoryStore()
	sess := st.Create("replay")
	if sess.ID == "" {
		t.Fatal("Create returned empty ID")
	}
	got, ok := st.Get(sess.ID)
	if !ok || got.ID != sess.ID {
		t.Fatal("Get failed after Create")
	}
	updated, ok := st.LoadCase(sess.ID, []byte(`{"caseId":"x"}`))
	if !ok || updated.Revision != 1 {
		t.Fatalf("LoadCase revision = %d, want 1", updated.Revision)
	}
	if !st.Close(sess.ID) {
		t.Fatal("Close returned false")
	}
}
