package bluesky

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/xrpc"
	bk "github.com/tailscale/go-bluesky"
)

type Client struct {
	Client *bk.Client
}

// PostRecord constructs a post record with a facet. To get there, it will find the
// position of the URL inside the text and attaches it to the post.
func PostRecord(title, url, author, stargazers, hashtags string) map[string]interface{} {
	text := title

	startAuthor := -1

	if len(author) > 0 {
		text += " by " + author
		startAuthor = strings.Index(text, " by @") + 4
	}

	if len(stargazers) > 0 {
		text += fmt.Sprintf(" (%s)", stargazers)
	}

	text += "\n\n" + url

	if len(hashtags) > 0 {
		text += "\n\n" + hashtags
	}

	startRepoURL := strings.Index(text, url)

	facets := []map[string]interface{}{
		{
			"index": map[string]int{
				"byteStart": startRepoURL,
				"byteEnd":   startRepoURL + len(url),
			},
			"features": []map[string]string{
				addFeature("app.bsky.richtext.facet#link", "uri", url),
			},
		},
	}

	if startAuthor > 0 {
		facets = append(facets, map[string]interface{}{
			"index": map[string]int{
				"byteStart": startAuthor,
				"byteEnd":   startAuthor + len(author),
			},
			"features": []map[string]string{
				addFeature("app.bsky.richtext.facet#link", "uri", "https://github.com/"+author[1:]),
			},
		})
	}

	// if len(hashtags) > 0 {
	// 	facets = append(facets, map[string]interface{}{
	// 		"index": map[string]int{
	// 			"byteStart": 0,
	// 			"byteEnd":   0,
	// 		},
	// 		"features": []map[string]string{
	// 			addFeature("app.bsky.richtext.facet#tag", "tag", "tag-here-fixme"),
	// 		},
	// 	})
	// }

	return map[string]interface{}{
		"$type":     "app.bsky.feed.post",
		"text":      text,
		"createdAt": time.Now().Format(time.RFC3339),
		"langs":     []string{"en-UK"},
		"facets":    facets,
	}

}

func addFeature(fType, fAttr, value string) map[string]string {
	return map[string]string{
		"$type": fType,
		fAttr:   value,
	}
}

// Post creates a post on BlueSky
func (c *Client) Post(ctx context.Context, post map[string]interface{}) error {
	return c.Client.CustomCall(func(api *xrpc.Client) error {
		sessionResult, err := atproto.ServerGetSession(ctx, api)
		if err != nil {
			return err
		}

		jRecord, err := json.Marshal(post)
		if err != nil {
			return fmt.Errorf("failed to marshal record: %v", err)
		}

		slog.Debug(string(jRecord))

		record := &util.LexiconTypeDecoder{}
		if err := record.UnmarshalJSON(jRecord); err != nil {
			return fmt.Errorf("failed to pack record: %v", err)
		}

		_, err = atproto.RepoCreateRecord(ctx, api, &atproto.RepoCreateRecord_Input{
			Collection: "app.bsky.feed.post",
			Repo:       sessionResult.Did,
			Record:     record,
		})

		return err
	})
}
