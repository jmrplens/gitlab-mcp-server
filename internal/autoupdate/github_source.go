package autoupdate

import "github.com/creativeprojects/go-selfupdate"

// newGitHubSource creates a selfupdate.Source backed by the public GitHub API.
// It defaults to [defaultGitHubSource] and can be overridden in tests to
// inject mock sources without requiring network access.
var newGitHubSource = defaultGitHubSource

func defaultGitHubSource() (selfupdate.Source, error) {
	return selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
}
