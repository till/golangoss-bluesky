// package main is the entry point for this application
package main

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"os/signal"
	"syscall"

	"log/slog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/till/golangoss-bluesky/internal/cmd"
	"github.com/till/golangoss-bluesky/internal/utils"
	"github.com/urfave/cli/v3"
)

var (
	blueskyHandle = "till+bluesky-golang@lagged.biz"

	// for cache
	cacheBucket = "golangoss-cache-bucket"
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
}

func main() {
	bot := cli.Command{
		Name:        "golangoss-bluesky",
		Description: "A little bot to post interesting Github projects to Bluesky",
		HideVersion: true,
		Authors: []any{
			mail.Address{Name: "Till Klampeckel"},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "bluesky-app-key",
				Sources:  cli.EnvVars("BLUESKY_APP_KEY"),
				Required: true,
			},
			&cli.StringFlag{
				Name:     "aws-endpoint",
				Sources:  cli.EnvVars("AWS_ENDPOINT"),
				Required: true,
			},
			&cli.StringFlag{
				Name:     "aws-access-key-id",
				Sources:  cli.EnvVars("AWS_ACCESS_KEY_ID"),
				Required: true,
			},
			&cli.StringFlag{
				Name:     "aws-secret-key",
				Sources:  cli.EnvVars("AWS_SECRET_KEY"),
				Required: true,
			},
			&cli.StringFlag{
				Name:     "github-token",
				Sources:  cli.EnvVars("GITHUB_TOKEN"),
				Required: true,
			},
		},

		Action: func(ctx context.Context, c *cli.Command) error {
			// Initialize S3 client
			mc, err := minio.New(c.String("aws-endpoint"), &minio.Options{
				Creds:  credentials.NewStaticV4(c.String("aws-access-key-id"), c.String("aws-secret-key"), ""),
				Secure: true,
			})
			if err != nil {
				return fmt.Errorf("failed to initialize minio client: %v", err)
			}

			exists, err := mc.BucketExists(ctx, cacheBucket)
			if err != nil {
				return fmt.Errorf("failed to check bucket: %w", err)
			}
			if !exists {
				if err := mc.MakeBucket(ctx, cacheBucket, minio.MakeBucketOptions{}); err != nil {
					return fmt.Errorf("failed to create bucket: %w", err)
				}
			}

			config := cmd.Config{
				Handle:      blueskyHandle,
				AppKey:      c.String("bluesky-app-key"),
				CacheBucket: cacheBucket,
				GitHubToken: c.String("github-token"),
			}

			return cmd.RunWithReconnect(ctx, mc, config)
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := bot.Run(ctx, os.Args); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		utils.LogError(err)
		os.Exit(1)
	}
}
