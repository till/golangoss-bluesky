package main

import (
	"context"
	"errors"
	"os"
	"time"

	"log/slog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	bk "github.com/tailscale/go-bluesky"
	"github.com/till/golangoss-bluesky/internal/bluesky"
	"github.com/till/golangoss-bluesky/internal/content"
)

var (
	blueskyHandle string = "till+bluesky-golang@lagged.biz"
	blueskyAppKey string = ""

	cacheBucket string = "golangoss-cache-bucket"

	ctx context.Context
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	ctx = context.Background()

	if _, status := os.LookupEnv("BLUESKY_APP_KEY"); !status {
		slog.ErrorContext(ctx, "no app key")
		os.Exit(1)
	}

	blueskyAppKey = os.Getenv("BLUESKY_APP_KEY")
}

func main() {
	client, err := bk.Dial(ctx, bk.ServerBskySocial)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	if err := client.Login(ctx, blueskyHandle, blueskyAppKey); err != nil {
		switch {
		case errors.Is(err, bk.ErrMasterCredentials):
			panic("You're not allowed to use your full-access credentials, please create an appkey")
		case errors.Is(err, bk.ErrLoginUnauthorized):
			panic("Username of application password seems incorrect, please double check")
		case err != nil:
			panic("Something else went wrong, please look at the returned error")
		}
	}

	// init s3 client
	mc, err := minio.New(os.Getenv("AWS_ENDPOINT"), &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_KEY"), ""),
		Secure: true,
	})
	if err != nil {
		slog.ErrorContext(ctx, "Failed to initialize MinIO client", slog.Any("err", err))
		os.Exit(1)
	}

	// ensure the bucket exists
	if err := mc.MakeBucket(ctx, cacheBucket, minio.MakeBucketOptions{}); err != nil {
		slog.ErrorContext(ctx, "failed to create bucket", slog.Any("err", err))
		os.Exit(1)
	}

	c := bluesky.Client{
		Client: client,
	}

	cacheClient := &content.CacheClientS3{
		MC:     mc,
		Bucket: cacheBucket,
		CTX:    ctx,
	}

	if err := content.Start(cacheClient); err != nil {
		slog.ErrorContext(ctx, "failed to start service", slog.Any("err", err))
		os.Exit(1)
	}

	for {
		slog.DebugContext(ctx, "checking...")
		if err := content.Do(ctx, c); err != nil {
			slog.ErrorContext(ctx, err.Error())
			os.Exit(1)
		}

		time.Sleep(5 * time.Minute)
	}
}
