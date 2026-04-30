// Package gcs wraps cloud.google.com/go/storage for case file and extract blob operations.
package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// Client wraps GCS operations needed by the hosted runtime.
type Client struct {
	gcs *storage.Client
}

// NewClient creates a Client using Application Default Credentials.
func NewClient(ctx context.Context) (*Client, error) {
	c, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("gcs: new client: %w", err)
	}
	return &Client{gcs: c}, nil
}

// NewClientFromStorage wraps an existing *storage.Client (used in tests via fake-gcs-server).
func NewClientFromStorage(c *storage.Client) *Client {
	return &Client{gcs: c}
}

// NewClientWithEndpoint creates a Client pointing at a custom endpoint.
// Pass option.WithHTTPClient(httpClient) via extraOpts to use a fake server's transport.
func NewClientWithEndpoint(ctx context.Context, endpoint string, extraOpts ...option.ClientOption) (*Client, error) {
	opts := append([]option.ClientOption{
		option.WithEndpoint(endpoint + "/storage/v1/"),
		option.WithoutAuthentication(),
	}, extraOpts...)
	c, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("gcs: new client: %w", err)
	}
	return &Client{gcs: c}, nil
}

// Put writes data to gs://{bucket}/{object}, overwriting any existing object.
func (c *Client) Put(ctx context.Context, bucket, object string, data []byte) error {
	w := c.gcs.Bucket(bucket).Object(object).NewWriter(ctx)
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return fmt.Errorf("gcs: write %s/%s: %w", bucket, object, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("gcs: close %s/%s: %w", bucket, object, err)
	}
	return nil
}

// Get reads and returns the full contents of gs://{bucket}/{object}.
func (c *Client) Get(ctx context.Context, bucket, object string) ([]byte, error) {
	r, err := c.gcs.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("gcs: open %s/%s: %w", bucket, object, err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gcs: read %s/%s: %w", bucket, object, err)
	}
	return data, nil
}

// IsNotFound reports whether err represents a missing bucket/object in GCS.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, storage.ErrObjectNotExist) {
		return true
	}
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		return gerr.Code == http.StatusNotFound
	}
	return false
}
