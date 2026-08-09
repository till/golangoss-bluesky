// Package cache stores which repos we've already posted, backed by an
// S3-compatible object store. Entries carry an expires-at metadata header;
// stale entries are cleaned up lazily on Get and swept periodically by the
// cleanup routine in the content package.
package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/minio/minio-go/v7"
)

// ClientS3 is a small cache that is backed by an S3-compatible store
type ClientS3 struct {
	mc                *minio.Client
	bucket            string
	defaultExpiration time.Duration
}

// NewCacheClientS3 creates a new S3 cache client with default settings
func NewClientS3(mc *minio.Client, bucket string) ClientS3 {
	return ClientS3{
		mc:                mc,
		bucket:            bucket,
		defaultExpiration: 60 * 24 * time.Hour,
	}
}

// Set sets a value in the cache
func (c *ClientS3) Set(ctx context.Context, key string, value any, exp time.Duration) error {
	var data bytes.Buffer
	if err := json.NewEncoder(&data).Encode(value); err != nil {
		return err
	}

	r := bytes.NewReader(data.Bytes())

	// Use the provided expiration time or fall back to default
	expiration := exp
	if expiration == 0 {
		expiration = c.defaultExpiration
	}

	// Calculate the expiration time
	expiresAt := time.Now().Add(expiration)

	// Set metadata to track expiration
	metadata := map[string]string{
		"expires-at": expiresAt.Format(time.RFC3339),
	}

	_, err := c.mc.PutObject(ctx, c.bucket, key, r, int64(r.Len()), minio.PutObjectOptions{
		UserMetadata: metadata,
	})
	return err
}

// Get returns an object, returns nil when it does not exist.
// Other S3 errors (network, 5xx, auth) are propagated so callers can distinguish a genuine cache miss from a transient failure.
func (c *ClientS3) Get(ctx context.Context, key string) (string, error) {
	objInfo, err := c.mc.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return "", redis.Nil
		}
		return "", err
	}

	if expiresAt, ok := objInfo.UserMetadata["expires-at"]; ok {
		expTime, err := time.Parse(time.RFC3339, expiresAt)
		if err == nil && time.Now().After(expTime) {
			// Object has expired, delete it and return not found
			_ = c.Del(ctx, key) // Ignore delete error
			return "", redis.Nil
		}
	}

	object, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return "", err
	}

	var val any
	if err := json.NewDecoder(object).Decode(&val); err != nil {
		return "", err
	}

	// not even sure why this method returns a string, when it's only used for bools
	switch v := val.(type) {
	case bool:
		return strconv.FormatBool(v), nil
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("unexpected cached value type %T for key %q", v, key)
	}
}

// Del deletes a value from the cache
func (c *ClientS3) Del(ctx context.Context, key string) error {
	return c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{
		ForceDelete: true,
	})
}
