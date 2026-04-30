package gcs_test

import (
	"context"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"

	"softprobe-runtime/internal/gcs"
)

func newTestClient(t *testing.T, bucket string) *gcs.Client {
	t.Helper()
	srv := fakestorage.NewServer([]fakestorage.Object{})
	t.Cleanup(srv.Stop)
	srv.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: bucket})
	return gcs.NewClientFromStorage(srv.Client())
}

func TestPutAndGet(t *testing.T) {
	client := newTestClient(t, "test-bucket")

	want := []byte(`{"hello":"world"}`)
	if err := client.Put(context.Background(), "test-bucket", "cases/sess1.case.json", want); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := client.Get(context.Background(), "test-bucket", "cases/sess1.case.json")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("Get = %q, want %q", got, want)
	}
}

func TestGet_MissingObject(t *testing.T) {
	client := newTestClient(t, "test-bucket")
	_, err := client.Get(context.Background(), "test-bucket", "notexist.json")
	if err == nil {
		t.Fatal("expected error for missing object, got nil")
	}
}

func TestPut_OverwritesExisting(t *testing.T) {
	client := newTestClient(t, "test-bucket")
	_ = client.Put(context.Background(), "test-bucket", "obj.json", []byte("v1"))
	_ = client.Put(context.Background(), "test-bucket", "obj.json", []byte("v2"))
	got, err := client.Get(context.Background(), "test-bucket", "obj.json")
	if err != nil {
		t.Fatalf("Get after overwrite: %v", err)
	}
	if string(got) != "v2" {
		t.Errorf("got %q, want v2", got)
	}
}
