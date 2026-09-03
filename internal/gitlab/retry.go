package gitlab

import (
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	// maxRetries is how many times a failed GitLab request is re-sent.
	//
	// client-go's own default is 5, which turns one tool call into six
	// upstream requests and, with its 700-900 ms service-interruption wait,
	// puts a floor of roughly twelve seconds under every call that meets a
	// 500. Two retries keep the point of retrying — a single blipping
	// request recovers — without letting one caller multiply itself sixfold
	// against an instance that is already struggling.
	maxRetries = 2

	// maxRetryBackoff caps a single wait between attempts.
	//
	// It is the whole reason this package supplies a backoff at all.
	// client-go sets the wait to time.Until(RateLimit-Reset) whenever that
	// is longer than the minimum and clamps it with nothing, so a 429 naming
	// a reset an hour ahead parks the calling goroutine for an hour — and
	// because the wait is recomputed per attempt, an upstream that keeps
	// re-sending a fresh reset parks it for longer still. Nothing else
	// bounds a tool call's upstream time.
	maxRetryBackoff = 5 * time.Second

	// retryBackoffStep is the per-attempt wait for a non-429 failure. It
	// matches the 700 ms floor client-go uses for service interruptions, so
	// a struggling instance gets the same breathing room it did before.
	retryBackoffStep = 700 * time.Millisecond

	// rateLimitResetHeader carries the Unix timestamp at which GitLab says
	// the caller's rate-limit window reopens.
	rateLimitResetHeader = "RateLimit-Reset"
)

// retryOptions is the retry policy every GitLab client in this package is
// built with.
//
// [gl.WithOnlyIdempotentRetries] is part of it because client-go retries a 5xx
// for any method by default, so a POST that GitLab already acted on before
// failing to answer is replayed — a duplicate note, a duplicate branch. It
// restricts 5xx retries to idempotent methods, which upstream says will become
// the default in the next major version.
func retryOptions() []gl.ClientOptionFunc {
	return []gl.ClientOptionFunc{
		gl.WithCustomRetryMax(maxRetries),
		gl.WithCustomBackoff(clampedBackoff),
		gl.WithOnlyIdempotentRetries(),
	}
}

// clampedBackoff decides how long to wait before re-sending a failed request.
//
// It keeps client-go's intent — honor RateLimit-Reset on a 429 so the retry
// lands after the window reopens, back off linearly on anything else — and
// adds the ceiling client-go leaves out. Waiting past [maxRetryBackoff] is not
// politeness: the request has a caller waiting on it, and a wait longer than
// the caller's patience only converts a fast failure into a pinned goroutine.
// A reset further out than the ceiling is answered by giving up sooner and
// letting the caller see the 429, which is information the caller can act on.
func clampedBackoff(minWait, maxWait time.Duration, attemptNum int, resp *http.Response) time.Duration {
	wait := time.Duration(attemptNum+1) * retryBackoffStep
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		if reset := rateLimitResetWait(resp); reset > wait {
			wait = reset
		}
	}
	if wait > maxRetryBackoff {
		wait = maxRetryBackoff
	}
	if wait < minWait {
		wait = minWait
	}
	// A little jitter so a fleet of pooled clients released by the same
	// reset does not re-send in lockstep.
	if maxWait > minWait {
		wait += time.Duration(rand.Int64N(int64(maxWait - minWait))) //nolint:gosec // retry jitter, not a security decision
	}
	if wait > maxRetryBackoff {
		wait = maxRetryBackoff
	}
	return wait
}

// rateLimitResetWait reports how long GitLab's RateLimit-Reset header says the
// caller must wait, or zero when the header is absent, unparseable or already
// in the past.
func rateLimitResetWait(resp *http.Response) time.Duration {
	raw := resp.Header.Get(rateLimitResetHeader)
	if raw == "" {
		return 0
	}
	reset, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || reset <= 0 {
		return 0
	}
	wait := time.Until(time.Unix(reset, 0))
	if wait < 0 {
		return 0
	}
	return wait
}
