package content

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/minio/minio-go/v7"
)

// CacheClientS3 is a small cache that is backed by an S3-compatible store
type CacheClientS3 struct {
	mc                *minio.Client
	bucket            string
	ctx               context.Context
	defaultExpiration time.Duration
}

// NewCacheClientS3 creates a new S3 cache client with default settings
func NewCacheClientS3(ctx context.Context, mc *minio.Client, bucket string) *CacheClientS3 {
	return &CacheClientS3{
		mc:                mc,
		bucket:            bucket,
		ctx:               ctx,
		defaultExpiration: 24 * time.Hour,
	}
}

func (c *CacheClientS3) Set(key string, value any, exp time.Duration) error {
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

	_, err := c.mc.PutObject(c.ctx, c.bucket, key, r, int64(r.Len()), minio.PutObjectOptions{
		UserMetadata: metadata,
	})
	return err
}

// Get returns an object, it follows the original pattern in larry to return redis.Nil when an object
// does not exist, in other case we can use minio.ToErrorResponse(err) to extract more details about the
// potential S3 related error
func (c *CacheClientS3) Get(key string) (string, error) {
	// First check if object exists and get its metadata
	objInfo, err := c.mc.StatObject(c.ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return "", redis.Nil
	}

	if expiresAt, ok := objInfo.UserMetadata["expires-at"]; ok {
		expTime, err := time.Parse(time.RFC3339, expiresAt)
		if err == nil && time.Now().After(expTime) {
			// Object has expired, delete it and return not found
			_ = c.Del(key) // Ignore delete error
			return "", redis.Nil
		}
	}

	object, err := c.mc.GetObject(c.ctx, c.bucket, key, minio.GetObjectOptions{})
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
		panic("unknown type")
	}
}

func (c *CacheClientS3) Del(key string) error {
	return c.mc.RemoveObject(c.ctx, c.bucket, key, minio.RemoveObjectOptions{
		ForceDelete: true,
	})
}

func (c *CacheClientS3) Scan(key string, action func(context.Context, string) error) error {
	return nil
}
