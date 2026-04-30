// package main is the entry point for this application
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"log/slog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	bk "github.com/tailscale/go-bluesky"
	"github.com/till/golangoss-bluesky/internal/bluesky"
	"github.com/till/golangoss-bluesky/internal/content"
	"github.com/till/golangoss-bluesky/internal/utils"
	"github.com/urfave/cli/v2"
)

var (
	blueskyHandle = "till+bluesky-golang@lagged.biz"
	blueskyAppKey = ""

	// for cache
	awsEndpoint    = ""
	awsAccessKeyID = ""
	awsSecretKey   = ""
	cacheBucket    = "golangoss-cache-bucket"

	// for github crawling
	githubToken = ""

	checkInterval time.Duration = 15 * time.Minute
	// How long to wait before retrying after a connection failure
	reconnectDelay time.Duration = 2 * time.Minute
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
}

// connectBluesky establishes a connection to Bluesky and logs in
func connectBluesky(ctx context.Context) (*bk.Client, error) {
	client, err := bk.Dial(ctx, bk.ServerBskySocial)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection: %v", err)
	}

	if err := client.Login(ctx, blueskyHandle, blueskyAppKey); err != nil {
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

// runWithReconnect attempts to run the bot with automatic reconnection on failure
func runWithReconnect(ctx context.Context, mc *minio.Client) error {
	for {
		client, err := connectBluesky(ctx)
		if err != nil {
			slog.Error("failed to connect to Bluesky", "error", err)
			slog.Info("retrying connection", "delay", reconnectDelay)
			time.Sleep(reconnectDelay)
			continue
		}

		c := bluesky.Client{
			Client: client,
		}

		cacheClient := content.NewCacheClientS3(ctx, mc, cacheBucket)

		// Initialize and start the cleanup handler
		cleanup := content.NewS3Cleanup(mc, cacheBucket)
		cleanup.Start(ctx)
		defer cleanup.Stop()

		if err := content.Start(githubToken, cacheClient); err != nil {
			slog.Error("failed to start service", "error", err)
			client.Close()
			time.Sleep(reconnectDelay)
			continue
		}

		// Run the main loop
		for {
			slog.DebugContext(ctx, "checking...")
			if err := content.Do(ctx, c); err != nil {
				if !errors.Is(err, content.ErrCouldNotContent) {
					slog.Error("error during content check", "error", err)
					client.Close()
					time.Sleep(reconnectDelay)
					break
				}
				slog.DebugContext(ctx, "backing off...")
			}

			time.Sleep(checkInterval)
		}
	}
}

func main() {
	bot := cli.App{
		Name:        "golangoss-bluesky",
		Description: "A little bot to post interesting Github projects to Bluesky",
		HideVersion: true,
		Authors: []*cli.Author{
			{
				Name: "Till Klampeckel",
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "bluesky-app-key",
				EnvVars:     []string{"BLUESKY_APP_KEY"},
				Required:    true,
				Destination: &blueskyAppKey,
			},
			&cli.StringFlag{
				Name:        "aws-endpoint",
				EnvVars:     []string{"AWS_ENDPOINT"},
				Required:    true,
				Destination: &awsEndpoint,
			},
			&cli.StringFlag{
				Name:        "aws-access-key-id",
				EnvVars:     []string{"AWS_ACCESS_KEY_ID"},
				Required:    true,
				Destination: &awsAccessKeyID,
			},
			&cli.StringFlag{
				Name:        "aws-secret-key",
				EnvVars:     []string{"AWS_SECRET_KEY"},
				Required:    true,
				Destination: &awsSecretKey,
			},
			&cli.StringFlag{
				Name:        "github-token",
				EnvVars:     []string{"GITHUB_TOKEN"},
				Required:    true,
				Destination: &githubToken,
			},
		},

		Action: func(cCtx *cli.Context) error {
			// Initialize S3 client
			mc, err := minio.New(awsEndpoint, &minio.Options{
				Creds:  credentials.NewStaticV4(awsAccessKeyID, awsSecretKey, ""),
				Secure: true,
			})
			if err != nil {
				return fmt.Errorf("failed to initialize minio client: %v", err)
			}

			// Ensure the bucket exists
			if err := mc.MakeBucket(cCtx.Context, cacheBucket, minio.MakeBucketOptions{}); err != nil {
				return fmt.Errorf("failed to create bucket: %v", err)
			}

			return runWithReconnect(cCtx.Context, mc)
		},
	}

	if err := bot.Run(os.Args); err != nil {
		utils.LogError(err)
		os.Exit(1)
	}
}
