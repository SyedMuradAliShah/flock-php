// Package github_client creates authenticated GitHub API clients for workflow services.
package github_client

import (
	"net/url"
	"strings"

	"github.com/google/go-github/v93/github"
)

func New(baseURL, token string) (*github.Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	return github.NewClient(
		github.WithAuthToken(token),
		github.WithURLs(new(parsed.String()), nil),
	)
}
