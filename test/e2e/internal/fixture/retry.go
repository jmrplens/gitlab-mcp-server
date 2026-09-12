//go:build e2e

// retry.go decides which failures are worth a second attempt.
//
// A GitLab under the load of a parallel suite drops connections, answers 429
// and 5xx, and reports state it has not finished writing yet. Every class
// here was met by the suite this replaces, and each is named so that a retry
// in the log says what it retried and a failure that is not retried says why
// not. The classification is a plain function of the error so it can be
// tested without a GitLab.

package fixture

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The retry budgets. Each is the number of attempts, not of retries.
const (
	// createRetries is how often a project or group creation is attempted.
	// GitLab CE races itself when many projects are created concurrently:
	// spurious "has already been taken" on a name nothing else holds, and
	// transient connection resets while nginx and puma settle.
	createRetries = 5
	// commitRetries is how often a commit is attempted while a fresh branch
	// is still becoming visible to the commits API.
	commitRetries = 8
	// retryBaseDelay is the delay after the first failed attempt; each later
	// wait grows by one more of it.
	retryBaseDelay = time.Second
)

// IsTransientNetwork reports whether err is the kind of connection failure a
// GitLab under heavy parallel load produces when nginx or puma drops a
// connection: EOF, a reset, a broken pipe, a refused connection. These are
// retried everywhere, because nothing about the request was wrong.
func IsTransientNetwork(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "EOF") ||
		strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "connection refused")
}

// IsRetryable reports whether err is likely transient and worth another
// attempt: a network failure, a rate limit, a server error, or one of GitLab's
// eventual-consistency answers about an object it has only just created.
//
// The status is read from the structured error client-go returns, never from
// a bare number in the message: "404" appears inside project IDs, commit SHAs
// and resource names, and matching it as text retried failures that were
// never going to succeed.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if IsTransientNetwork(err) {
		return true
	}
	switch {
	case toolutil.IsHTTPStatus(err, http.StatusTooManyRequests),
		toolutil.IsHTTPStatus(err, http.StatusInternalServerError),
		toolutil.IsHTTPStatus(err, http.StatusBadGateway),
		toolutil.IsHTTPStatus(err, http.StatusServiceUnavailable):
		return true
	// A newly created ref is not yet visible to the API that was told about
	// it, which GitLab reports as a 404 for a few hundred milliseconds.
	case toolutil.IsHTTPStatus(err, http.StatusNotFound):
		return true
	}
	message := strings.ToLower(err.Error())
	// The commits API refuses to write to a branch it cannot see yet.
	if strings.Contains(message, "only create or edit files when you are on a branch") {
		return true
	}
	// GitLab's NotificationSetting row is created inside a read with
	// find_or_initialize_by, so two concurrent readers both insert and the
	// loser is refused with a uniqueness error on a request that only read.
	return strings.Contains(message, "already exists in source")
}

// CreateRetryable reports whether a project or group creation failed for a
// reason a second attempt can fix.
//
// "has already been taken" is the one that looks least like it: it is what
// GitLab CE answers when two concurrent creations race inside its own
// namespace validation, and the name it names is free. On a licensed instance
// two more appear, both from the repository side: "Failed to create
// repository" and an "Internal API error (502)" from Gitaly while the instance
// is under load. They are retried only on an enterprise instance because that
// is the only place they have been seen, and a Free instance answering them
// would be saying something new.
func CreateRetryable(err error, enterprise bool) bool {
	if err == nil {
		return false
	}
	if IsTransientNetwork(err) || strings.Contains(err.Error(), "already been taken") {
		return true
	}
	if !enterprise {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "Failed to create repository") ||
		strings.Contains(message, "Internal API error (502)")
}

// IsStatus reports whether err is GitLab answering with the given HTTP status.
// It reads the structured error client-go returns, which every call in this
// package makes directly, so there is no text to parse.
func IsStatus(err error, code int) bool {
	return toolutil.IsHTTPStatus(err, code)
}

// statusOf returns the HTTP status an error carries, and zero when it carries
// none: a network failure, a cancelled context, an error of this package's
// own.
func statusOf(err error) int {
	var response *gl.ErrorResponse
	if errors.As(err, &response) && response.Response != nil {
		return response.Response.StatusCode
	}
	return 0
}

// retryTransient runs op until it succeeds or fails for a reason IsRetryable
// does not cover, within attempts. It is the shape every builder's create
// takes; the classification is what differs per builder, and a builder that
// needs another passes its own.
func retryTransient[O any](e *harness.Env, label string, attempts int, op func() (O, error)) (O, error) {
	e.T.Helper()
	return retryWhen(e, label, attempts, IsRetryable, op)
}

// retryWhen runs op until it succeeds, fails for a reason retryable does not
// accept, or runs out of attempts, waiting a little longer after each failure.
func retryWhen[O any](e *harness.Env, label string, attempts int, retryable func(error) bool, op func() (O, error)) (O, error) {
	e.T.Helper()
	return harness.Retry(e.Ctx, e.T, label, attempts, retryBaseDelay, func(int) (O, bool, string, error) {
		out, err := op()
		if err == nil {
			return out, false, "", nil
		}
		return out, retryable(err), describeRetry(err), err
	})
}

// describeRetry names the class of a retried failure for the log line, so a
// retried run reads as a story rather than as the same error three times.
func describeRetry(err error) string {
	switch {
	case IsTransientNetwork(err):
		return "transient network error"
	case statusOf(err) == http.StatusTooManyRequests:
		return "rate limited"
	case statusOf(err) >= http.StatusInternalServerError:
		return "server error"
	case statusOf(err) == http.StatusNotFound:
		return "not visible yet"
	case strings.Contains(err.Error(), "already been taken"):
		return "name collision inside GitLab's own validation"
	default:
		return "retryable error"
	}
}

// budget returns the wait a step is allowed, larger on a licensed instance,
// where every write does more work and takes measurably longer.
func budget(e *harness.Env, base, enterprise time.Duration) time.Duration {
	if e.Runtime().Tier.IsEnterprise() && enterprise > base {
		return enterprise
	}
	return base
}

// cleanupContextTimeout bounds one cleanup's own context when the ledger's
// context is already gone, which a hook that runs after the tests uses.
const cleanupContextTimeout = 60 * time.Second

// withCleanupTimeout returns ctx bounded by the cleanup budget when ctx
// carries no deadline of its own.
func withCleanupTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, has := ctx.Deadline(); has {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, cleanupContextTimeout)
}
