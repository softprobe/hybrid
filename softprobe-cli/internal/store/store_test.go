package store

import (
	"bytes"
	"testing"
)

func TestStoreCreateLoadAndMutateSession(t *testing.T) {
	st := NewStore()

	session := st.Create("replay")
	if session.ID == "" {
		t.Fatal("session ID is empty")
	}
	if session.Mode != "replay" {
		t.Fatalf("mode = %q, want replay", session.Mode)
	}
	if session.Revision != 0 {
		t.Fatalf("revision = %d, want 0", session.Revision)
	}

	loadedCase := []byte(`{"caseId":"demo"}`)
	updated, ok := st.LoadCase(session.ID, loadedCase)
	if !ok {
		t.Fatal("LoadCase returned false")
	}
	if updated.Revision != 1 {
		t.Fatalf("revision after load-case = %d, want 1", updated.Revision)
	}
	loadedCase[0] = 'X'
	if !bytes.Equal(updated.LoadedCase, []byte(`{"caseId":"demo"}`)) {
		t.Fatalf("loaded case was not copied: %s", string(updated.LoadedCase))
	}

	policy := []byte(`{"externalHttp":"strict"}`)
	updated, ok = st.ApplyPolicy(session.ID, policy)
	if !ok {
		t.Fatal("ApplyPolicy returned false")
	}
	if updated.Revision != 2 {
		t.Fatalf("revision after policy = %d, want 2", updated.Revision)
	}

	if got, ok := st.Get(session.ID); !ok || got.Revision != 2 {
		t.Fatalf("Get returned %+v, ok=%v, want revision 2", got, ok)
	}

	if !st.Close(session.ID) {
		t.Fatal("Close returned false")
	}
	if _, ok := st.Get(session.ID); ok {
		t.Fatal("session still present after Close")
	}
}
