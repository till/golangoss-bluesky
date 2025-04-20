package content

import (
	"context"
	"fmt"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/till/golangoss-bluesky/internal/utils"
)

// S3Cleanup handles background cleanup of expired objects in S3
type S3Cleanup struct {
	mc     *minio.Client
	bucket string
	// cleanupInterval is how often to run the cleanup routine
	cleanupInterval time.Duration
	// stopCleanup is used to signal the cleanup routine to stop
	stopCleanup chan struct{}
}

// NewS3Cleanup creates a new S3 cleanup handler
func NewS3Cleanup(mc *minio.Client, bucket string) *S3Cleanup {
	return &S3Cleanup{
		mc:              mc,
		bucket:          bucket,
		cleanupInterval: 24 * time.Hour,
		stopCleanup:     make(chan struct{}),
	}
}

// Start begins the background cleanup routine
func (c *S3Cleanup) Start(ctx context.Context) {
	go c.cleanupRoutine(ctx)
}

// Stop stops the background cleanup routine
func (c *S3Cleanup) Stop() {
	close(c.stopCleanup)
}

// cleanupRoutine periodically checks for and deletes expired objects
func (c *S3Cleanup) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.cleanupExpired(ctx); err != nil {
				utils.LogErrorWithContext(ctx, fmt.Errorf("failed to cleanup expired objects: %w", err))
			}
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanupExpired scans the bucket for expired objects and deletes them
func (c *S3Cleanup) cleanupExpired(ctx context.Context) error {
	filter := minio.ListObjectsOptions{
		Recursive:    true,
		WithMetadata: true,
	}

	objectsCh := c.mc.ListObjects(ctx, c.bucket, filter)
	for obj := range objectsCh {
		if obj.Err != nil {
			utils.LogErrorWithContext(ctx, fmt.Errorf("failed to list objects: %w", obj.Err))
			continue
		}

		// Check if object has expiration metadata
		expiresAt, ok := obj.UserMetadata["expires-at"]
		if !ok {
			utils.LogErrorWithContext(
				ctx,
				fmt.Errorf("no expiration metadata found for object: %s", obj.Key),
			)
			if err := c.delete(ctx, obj); err != nil {
				return err
			}
			continue
		}

		expTime, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			utils.LogError(fmt.Errorf("failed to parse expiration time (%s): %w", obj.Key, err))
			continue
		}
		if !time.Now().After(expTime) {
			continue
		}

		// Object has expired, delete it
		if err := c.delete(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

func (c *S3Cleanup) delete(ctx context.Context, obj minio.ObjectInfo) error {
	if err := c.mc.RemoveObject(ctx, c.bucket, obj.Key, minio.RemoveObjectOptions{
		ForceDelete: true,
	}); err != nil {
		return fmt.Errorf("failed to delete expired object (%s): %w", obj.Key, err)
	}
	return nil
}
