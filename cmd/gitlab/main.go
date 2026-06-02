// Command gitlab prints public gitlab.com projects matching a language and query.
//
//	go run ./cmd/gitlab -lang Go -q bot -min-stars 5 -n 20
//
// Set GITLAB_TOKEN for higher rate limits.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	gl "github.com/till/golangoss-bluesky/internal/gitlab"
)

func main() {
	lang := flag.String("lang", "Go", "programming language filter (empty disables)")
	query := flag.String("q", "", "free-text search")
	minStars := flag.Int64("min-stars", 0, "drop projects with fewer stars")
	perPage := flag.Int("n", 20, "page size (1-100)")
	page := flag.Int("page", 1, "page number (1-indexed)")
	random := flag.Bool("random", false, "return one randomly-picked project (ignores -q, -n, -page, -min-stars)")
	flag.Parse()

	c, err := gl.New(os.Getenv("GITLAB_TOKEN"), nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		os.Exit(1)
	}

	ctx := context.Background()

	if *random {
		p, err := c.RandomProject(ctx, *lang)
		if err != nil {
			fmt.Fprintln(os.Stderr, "random:", err)
			os.Exit(1)
		}
		if p == nil {
			fmt.Println("no results")
			return
		}
		fmt.Printf("%-40s  %5d  %s\n", p.PathWithNS, p.Stars, p.WebURL)
		if p.Description != "" {
			fmt.Println(p.Description)
		}
		return
	}

	results, err := c.Search(ctx, gl.SearchOptions{
		Language: *lang,
		Query:    *query,
		MinStars: *minStars,
		PerPage:  *perPage,
		Page:     *page,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "search:", err)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Println("no results")
		return
	}
	for _, p := range results {
		fmt.Printf("%-40s  %5d  %s\n", p.PathWithNS, p.Stars, p.WebURL)
	}
}
