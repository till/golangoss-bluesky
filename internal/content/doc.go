// Package content orchestrates the posting cycle: it asks the provider for a
// repo to publish and hands the result to the bluesky client. It also owns the
// background S3 cleanup that expires stale cache entries.
package content
