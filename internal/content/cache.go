package content

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// CacheClientProcess is an in process cache that adheres to larry's interface
type CacheClientProcess struct {
	store sync.Map
}

func (c *CacheClientProcess) Set(key string, value interface{}, exp time.Duration) error {
	slog.Debug("set", slog.String("key", key), slog.Any("value", value))
	c.store.Store(key, value)
	return nil
}

func (c *CacheClientProcess) Get(key string) (string, error) {
	slog.Debug("get", slog.String("key", key))
	val, status := c.store.Load(key)
	if !status {
		return "", redis.Nil
	}
	return val.(string), nil
}

func (c *CacheClientProcess) Del(key string) error {
	c.store.Delete(key)
	return nil
}

func (c *CacheClientProcess) Scan(key string, action func(context.Context, string) error) error {
	return nil
}
