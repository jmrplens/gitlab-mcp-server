package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// fetchFunc fetches one page and returns the status and the headers of the
// response it ends on, redirects followed.
type fetchFunc func(ctx context.Context, url string) (int, http.Header, error)

// requestTimeoutSeconds bounds one fetch, the redirects and the body
// included, so a site that never answers cannot hold the job until the
// runner's own limit. It is a count of seconds, multiplied where the client is
// built, so the product sits in a statement the coverage profile records.
const requestTimeoutSeconds = 30

// bodyLimit is as much of a body as a fetch reads before closing it. Nothing
// is judged in the body; it is read so the connection ends cleanly.
const bodyLimit = 1 << 20

// newHTTPClient returns the client the command fetches with.
func newHTTPClient() *http.Client {
	return &http.Client{Timeout: requestTimeoutSeconds * time.Second}
}

// httpFetch returns a fetchFunc that sends a GET with client, the method a
// browser loads a page with, so the headers judged are the ones a reader's
// browser receives.
func httpFetch(client *http.Client) fetchFunc {
	return func(ctx context.Context, url string) (int, http.Header, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if err != nil {
			return 0, nil, err
		}
		response, err := client.Do(request) //#nosec G704 -- the URL is the documentation page the operator named on the command line
		if err != nil {
			return 0, nil, err
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, bodyLimit))
		_ = response.Body.Close()
		return response.StatusCode, response.Header, nil
	}
}

// fetchWithRetry fetches url up to attempts times, waiting delay between two
// attempts, while the fetch fails or answers 5xx: right after a deploy the
// site can still be propagating. Any other answer is returned at once, and
// when the attempts run out the last outcome is.
func fetchWithRetry(ctx context.Context, fetch fetchFunc, url string, attempts int, delay time.Duration, wait func(time.Duration)) (int, http.Header, error) {
	var (
		status int
		header http.Header
		err    error
	)
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			wait(delay)
		}
		status, header, err = fetch(ctx, url)
		if err == nil && status < http.StatusInternalServerError {
			return status, header, nil
		}
	}
	if err != nil {
		return 0, nil, fmt.Errorf("after %d attempt(s): %w", attempts, err)
	}
	return status, header, nil
}
