package autoupdate

import (
	"context"
	"errors"
	"io"

	selfupdate "github.com/creativeprojects/go-selfupdate"
)

// EmptySource is a selfupdate.Source that returns no releases.
// Used for testing when a valid Updater is needed but no real update server.
type EmptySource struct{}

// ListReleases returns no releases.
func (EmptySource) ListReleases(_ context.Context, _ selfupdate.Repository) ([]selfupdate.SourceRelease, error) {
	return []selfupdate.SourceRelease{}, nil
}

// DownloadReleaseAsset returns nil.
func (EmptySource) DownloadReleaseAsset(_ context.Context, _ *selfupdate.Release, _ int64) (io.ReadCloser, error) {
	return io.NopCloser(nil), nil
}

// ErrorSource is a selfupdate.Source that returns an error on every operation.
// Used for testing error handling paths.
type ErrorSource struct{ Err error }

// ListReleases returns the configured error.
func (e ErrorSource) ListReleases(_ context.Context, _ selfupdate.Repository) ([]selfupdate.SourceRelease, error) {
	if e.Err != nil {
		return nil, e.Err
	}
	return nil, errors.New("error source")
}

// DownloadReleaseAsset returns the configured error.
func (e ErrorSource) DownloadReleaseAsset(_ context.Context, _ *selfupdate.Release, _ int64) (io.ReadCloser, error) {
	if e.Err != nil {
		return nil, e.Err
	}
	return nil, errors.New("error source")
}
