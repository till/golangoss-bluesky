package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	bk "github.com/tailscale/go-bluesky"

	"github.com/minio/minio-go/v7"
	"github.com/till/golangoss-bluesky/internal/bluesky"
	"github.com/till/golangoss-bluesky/internal/content"
)

const (
	checkInterval time.Duration = 15 * time.Minute
	// How long to wait before retrying after a connection failure
	reconnectDelay time.Duration = 2 * time.Minute
)

// connectBluesky establishes a connection to Bluesky and logs in
func connectBluesky(ctx context.Context, handle, appKey string) (*bk.Client, error) {
	client, err := bk.Dial(ctx, bk.ServerBskySocial)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection: %v", err)
	}

	if err := client.Login(ctx, handle, appKey); err != nil {
		client.Close()
		switch {
		case errors.Is(err, bk.ErrMasterCredentials):
			return nil, fmt.Errorf("you're not allowed to use your full-access credentials, please create an appkey")
		case errors.Is(err, bk.ErrLoginUnauthorized):
			return nil, fmt.Errorf("username of application password seems incorrect, please double check")
		default:
			return nil, fmt.Errorf("login failed: %v", err)
		}
	}

	return client, nil
}

// RunWithReconnect attempts to run the bot with automatic reconnection on failure
func RunWithReconnect(ctx context.Context, mc *minio.Client, cfg Config) error {
	cacheClient := content.NewCacheClientS3(ctx, mc, cfg.CacheBucket)

	cleanup := content.NewS3Cleanup(mc, cfg.CacheBucket)
	cleanup.Start(ctx)
	defer cleanup.Stop()

	if err := content.Start(cfg.GitHubToken, cacheClient); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		client, err := connectBluesky(ctx, cfg.Handle, cfg.AppKey)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Error("failed to connect to Bluesky", "error", err)
			slog.Info("retrying connection", "delay", reconnectDelay)
			if err := sleepCtx(ctx, reconnectDelay); err != nil {
				return err
			}
			continue
		}

		c := bluesky.Client{Client: client}
		runSession(ctx, c)
		client.Close()

		if err := sleepCtx(ctx, reconnectDelay); err != nil {
			return err
		}
	}
}

// runSession runs the inner check loop until ctx is cancelled or content.Do
// returns a non-recoverable error. The caller is responsible for closing the
// bluesky client and reconnecting.
func runSession(ctx context.Context, c bluesky.Client) {
	for {
		slog.DebugContext(ctx, "checking...")
		if err := content.Do(ctx, c); err != nil {
			if !errors.Is(err, content.ErrCouldNotContent) {
				slog.Error("error during content check", "error", err)
				return
			}
			slog.DebugContext(ctx, "backing off...")
		}
		if err := sleepCtx(ctx, checkInterval); err != nil {
			return
		}
	}
}

// sleepCtx sleeps for d, returning ctx.Err() if ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
