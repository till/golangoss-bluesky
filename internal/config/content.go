// Package config holds the tunables the content pipeline reads when it
// asks the provider for the next repo to publish: which language to search,
// whether to include archived repos, and how recent the last push must be.
package config

import "time"

type Config struct {
	Language    string    // programming language
	Archived    bool      // if "false", it will filter out archived repos
	PushedSince time.Time // filter to only get active repos
}
