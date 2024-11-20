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
	MC     *minio.Client
	Bucket string
	CTX    context.Context
}

func (c *CacheClientS3) Set(key string, value interface{}, exp time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	r := bytes.NewReader(data)

	_, err = c.MC.PutObject(c.CTX, c.Bucket, key, r, int64(r.Len()), minio.PutObjectOptions{
		Expires: time.Now().Add(exp),
	})
	return err
}

// Get returns an object, it follows the original pattern in larry to return redis.Nil when an object
// does not exist, in other case we can use minio.ToErrorResponse(err) to extract more details about the
// potential S3 related error
func (c *CacheClientS3) Get(key string) (string, error) {
	if _, err := c.MC.StatObject(c.CTX, c.Bucket, key, minio.StatObjectOptions{}); err != nil {
		return "", redis.Nil
	}

	object, err := c.MC.GetObject(c.CTX, c.Bucket, key, minio.GetObjectOptions{})
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
	return c.MC.RemoveObject(c.CTX, c.Bucket, key, minio.RemoveObjectOptions{
		ForceDelete: true,
	})
}

func (c *CacheClientS3) Scan(key string, action func(context.Context, string) error) error {

	return nil
}
