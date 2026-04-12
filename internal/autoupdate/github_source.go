package autoupdate

import "github.com/creativeprojects/go-selfupdate"

// newGitHubSource creates a GitHub-backed selfupdate.Source.
// If token is empty, public GitHub API rate limits apply.
func newGitHubSource(token string) (*selfupdate.GitHubSource, error) {
	return selfupdate.NewGitHubSource(selfupdate.GitHubConfig{
		APIToken: token,
	})
}
