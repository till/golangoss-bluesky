package bluesky_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/till/golangoss-bluesky/internal/bluesky"
)

type testCase struct {
	Title          string
	Description    string
	URL            string
	Author         string
	Stargazers     string
	Tag            string
	ExpectedFacets int
}

func TestPostRecord(t *testing.T) {
	testCases := []testCase{
		{
			Title:          "simple",
			Description:    "description",
			URL:            "https://github.com/user/repo",
			Author:         "@user",
			Stargazers:     "1 ⭐️",
			Tag:            "#go",
			ExpectedFacets: 3,
		},
		{
			Title:          "extra-long-description",
			Description:    "description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description, description",
			URL:            "https://github.com/org/repo",
			Author:         "",
			Stargazers:     "1 ⭐️",
			Tag:            "#go",
			ExpectedFacets: 2,
		},
		{
			Title:          "Short",
			URL:            "https://github.com/s/s",
			Author:         "",
			Stargazers:     "0 ⭐️",
			Tag:            "",
			ExpectedFacets: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Title, func(t *testing.T) {
			record := bluesky.PostRecord(tc.Title, tc.Description, tc.URL, tc.Author, tc.Stargazers, tc.Tag)
			assert.NotNil(t, record)

			assert.NotEmpty(t, record.CreatedAt)
			assert.Len(t, record.Facets, tc.ExpectedFacets)

			assert.True(t, (len(record.Text) <= 300))
		})
	}
}
