package autoupdate

import "github.com/creativeprojects/go-selfupdate"

// newGitHubSource creates a GitHub-backed selfupdate.Source.
// Public GitHub API rate limits apply.
func newGitHubSource() (*selfupdate.GitHubSource, error) {
	return selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
}
