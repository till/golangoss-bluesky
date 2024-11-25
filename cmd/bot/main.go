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
	"github.com/urfave/cli/v2"
)

var (
	blueskyHandle string = "till+bluesky-golang@lagged.biz"
	blueskyAppKey string = ""

	cacheBucket string = "golangoss-cache-bucket"

	ctx context.Context

	// for cache
	awsEndpoint    string = ""
	awsAccessKeyId string = ""
	awsSecretKey   string = ""

	// for github crawling
	githubToken string = ""

	checkInterval time.Duration = 15 * time.Minute
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	ctx = context.Background()
}

func main() {
	bot := cli.App{
		Name: "golangoss-bluesky",
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
				Destination: &awsAccessKeyId,
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
			client, err := bk.Dial(ctx, bk.ServerBskySocial)
			if err != nil {
				return fmt.Errorf("failed to open connection: %v", err)
			}
			defer client.Close()

			if err := client.Login(ctx, blueskyHandle, blueskyAppKey); err != nil {
				switch {
				case errors.Is(err, bk.ErrMasterCredentials):
					return fmt.Errorf("you're not allowed to use your full-access credentials, please create an appkey")
				case errors.Is(err, bk.ErrLoginUnauthorized):
					return fmt.Errorf("username of application password seems incorrect, please double check")
				case err != nil:
					return fmt.Errorf("something else went wrong, please look at the returned error")
				}
			}

			// init s3 client
			mc, err := minio.New(awsEndpoint, &minio.Options{
				Creds:  credentials.NewStaticV4(awsAccessKeyId, awsSecretKey, ""),
				Secure: true,
			})
			if err != nil {
				return fmt.Errorf("failed to initialize minio client: %v", err)
			}

			// ensure the bucket exists
			if err := mc.MakeBucket(ctx, cacheBucket, minio.MakeBucketOptions{}); err != nil {
				return fmt.Errorf("failed to create bucket: %v", err)
			}

			c := bluesky.Client{
				Client: client,
			}

			cacheClient := &content.CacheClientS3{
				MC:     mc,
				Bucket: cacheBucket,
				CTX:    ctx,
			}

			if err := content.Start(githubToken, cacheClient); err != nil {
				return fmt.Errorf("failed to start service: %v", err)
			}

			var runErr error

			for {
				slog.DebugContext(ctx, "checking...")
				if err := content.Do(ctx, c); err != nil {
					if !errors.Is(err, content.ErrCouldNotContent) {
						runErr = err
						break
					}
					slog.DebugContext(ctx, "backing off...")
				}

				time.Sleep(checkInterval)
			}
			return runErr
		},
	}

	if err := bot.Run(os.Args); err != nil {
		slog.ErrorContext(ctx, err.Error())
		os.Exit(1)
	}

}
