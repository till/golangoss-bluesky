package bluesky

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/xrpc"
	bk "github.com/tailscale/go-bluesky"
)

type Client struct {
	Client *bk.Client
}

// PostRecord constructs a post record with a facet. To get there, it will find the
// position of the URL inside the text and attaches it to the post.
func PostRecord(title, description, url, author, stargazers, hashtags string) *bsky.FeedPost {
	text := title

	var startAuthor int64 = -1

	if len(author) > 0 {
		text += " by " + author
		startAuthor = int64(strings.Index(text, " by @")) + 4
	}

	if len(stargazers) > 0 {
		text += fmt.Sprintf(" (%s)", stargazers)
	}

	// 300 = limit + '#go'
	if len(text) < (300-3) && len(description) > 0 {
		// poor version of normalize
		description = strings.Join(strings.Fields(description), " ")
		if len(description) > 150 {
			text += "\n\n" + description[0:147] + "..."
		} else {
			text += "\n\n" + description
		}
	}

	if len(hashtags) > 0 {
		text += "\n\n" + hashtags
	}

	var startRepoURL int64 = 0

	facets := []*bsky.RichtextFacet{}
	facets = append(facets, addFacet(
		startRepoURL,
		startRepoURL+int64(len(title)),
		addLinkFeature(url)))

	if startAuthor > 0 {
		facets = append(facets, addFacet(
			startAuthor,
			startAuthor+int64(len(author)),
			addLinkFeature("https://github.com/"+author[1:]),
		))
	}

	if len(hashtags) > 0 {
		allTags := strings.Fields(hashtags)

		startHashTag := int64(strings.Index(text, hashtags))

		for _, t := range allTags {
			slog.Debug(t)
			facets = append(facets, addFacet(
				startHashTag,
				startHashTag+int64(len(t)),
				addTagFeature(t[1:]),
			))

			// set cursor for next hashtag
			startHashTag += int64(len(t)) + 1
		}
	}

	// fmt.Printf("%v", facets)

	return &bsky.FeedPost{
		Text:      text,
		CreatedAt: time.Now().Format(time.RFC3339),
		Langs:     []string{"en-UK"},
		Facets:    facets,
	}
}

// build structure for the facet (enables linking)
func addFacet(start, end int64, feature any) *bsky.RichtextFacet {
	facet := &bsky.RichtextFacet{
		Index: &bsky.RichtextFacet_ByteSlice{
			ByteStart: start,
			ByteEnd:   end,
		},
		Features: []*bsky.RichtextFacet_Features_Elem{},
	}

	switch f := feature.(type) {
	case *bsky.RichtextFacet_Link:
		facet.Features = append(facet.Features, &bsky.RichtextFacet_Features_Elem{
			RichtextFacet_Link: f,
		})
	case *bsky.RichtextFacet_Tag:
		facet.Features = append(facet.Features, &bsky.RichtextFacet_Features_Elem{
			RichtextFacet_Tag: f,
		})
	case *bsky.RichtextFacet_Mention:
		panic("mention not supported")
	default:
		panic("unknown type")
	}

	return facet
}

// build structure for the feature (link target)
func addLinkFeature(uri string) *bsky.RichtextFacet_Link {
	return &bsky.RichtextFacet_Link{
		Uri: uri,
	}
}

func addTagFeature(tag string) *bsky.RichtextFacet_Tag {
	return &bsky.RichtextFacet_Tag{
		Tag: tag,
	}
}

// Post creates a post on BlueSky
func (c *Client) Post(ctx context.Context, post *bsky.FeedPost) error {
	return c.Client.CustomCall(func(api *xrpc.Client) error {
		sessionResult, err := atproto.ServerGetSession(ctx, api)
		if err != nil {
			return handleError(ctx, err)
		}

		_, err = atproto.RepoCreateRecord(ctx, api, &atproto.RepoCreateRecord_Input{
			Collection: "app.bsky.feed.post",
			Repo:       sessionResult.Did,
			Record: &util.LexiconTypeDecoder{
				Val: post,
			},
		})

		return handleError(ctx, err)
	})
}

func handleError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	// handle atproto specific errors
	if rpcErr, ok := err.(*xrpc.Error); ok {
		switch rpcErr.StatusCode {
		case 400, 413:
			slog.WarnContext(ctx, "the record data is invalid")
			return err
		case 401, 403:
			slog.ErrorContext(ctx, "authentication failed; check the credentials")
			return err
		case 429:
			slog.InfoContext(ctx, "we got rate limited, let's back off: "+rpcErr.Error())
			return nil
		default:
			slog.DebugContext(ctx, "unhandled error: "+rpcErr.Error())
			return err
		}
	}

	// probably a low-level http/dns error
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if netErr.Temporary() {
			slog.InfoContext(ctx, "temporary error: "+netErr.Error())
			return nil
		}

		if netErr.Timeout() {
			slog.InfoContext(ctx, "timeout error: "+netErr.Error())
			return nil
		}

		// unhandled net error
		slog.ErrorContext(ctx, netErr.Error())
		return err
	}

	// unhandled
	slog.ErrorContext(ctx, err.Error())
	return err
}
