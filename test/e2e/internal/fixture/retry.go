// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
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
	// assignRetries is how often an assignment or a review request is
	// attempted while a user it names was made a member too recently for
	// GitLab to keep them. Eight attempts wait up to 28 seconds between them,
	// against the fifteen it took on a GitLab 19.4.1-ee under the suite's
	// load.
	assignRetries = 8
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
	if branchNotReady(err) {
		return true
	}
	// GitLab's NotificationSetting row is created inside a read with
	// find_or_initialize_by, so two concurrent readers both insert and the
	// loser is refused with a uniqueness error on a request that only read.
	return strings.Contains(strings.ToLower(err.Error()), "already exists in source")
}

// branchNotReadyAnswer is what the commits API answers a write to a branch it
// cannot see yet, which it does for a moment after the branch was created,
// whoever created it: the fixture through client-go or a scenario through the
// server under test.
const branchNotReadyAnswer = "only create or edit files when you are on a branch"

// branchNotReady reports whether err carries [branchNotReadyAnswer], whatever
// its case and whatever wraps it: a client-go error, or the text of a tool
// error the server answered with.
func branchNotReady(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), branchNotReadyAnswer)
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

// UserCreateRetryable reports whether a user creation failed for a reason a
// second attempt can fix.
//
// Besides the transient ones, there is one that looks permanent and is not:
// GitLab judges the password against its own weak-password rules, and a random
// password occasionally lands on something it refuses ("must not contain
// commonly used combinations of words and letters"). Nothing about the user
// being created is wrong, and the builder mints a fresh password per attempt,
// so the next one goes through. One run in this suite hit it once in some
// forty user creations, which is often enough to redden a suite and rare
// enough to look like a mystery when it does.
func UserCreateRetryable(err error) bool {
	if err == nil {
		return false
	}
	if IsRetryable(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "commonly used combinations")
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
	return retryWhenIn(e.Ctx, e.T, label, attempts, retryable, op)
}

// retryWhenIn is [retryWhen] for a caller that holds a context and a test to
// report its retries to rather than an Env: the World's extras, which run
// under a budget of their own derived from the first asking test's context
// and must never fail that test.
//
// A 429 that names a Retry-After is waited out before the next attempt
// (rateLimitWait), on top of the backoff every retry gets: GitLab answers
// some creations from a fixed window, ten service accounts a minute per
// user, and backing off a few seconds against a one-minute window spent five
// attempts in about 23 seconds and failed the world. The last attempt waits
// for nothing, since nothing follows it.
func retryWhenIn[O any](ctx context.Context, tb testing.TB, label string, attempts int, retryable func(error) bool, op func() (O, error)) (O, error) {
	tb.Helper()
	return harness.Retry(ctx, tb, label, attempts, retryBaseDelay, func(attempt int) (O, bool, string, error) {
		out, err := op()
		if err == nil {
			return out, false, "", nil
		}
		retry, reason := retryable(err), describeRetry(err)
		if wait := rateLimitWait(err); retry && wait > 0 && attempt < attempts-1 {
			waitOut(ctx, wait)
			reason += fmt.Sprintf(", waited %s as its Retry-After asked", wait)
		}
		return out, retry, reason, err
	})
}

// maxRateLimitWait bounds how long one attempt waits out a rate limit window,
// so a Retry-After far longer than any window GitLab uses cannot hold a world
// past its test's own budget. It is two minutes, written as nanoseconds
// rather than as a product of time.Second: a product at package level is an
// arithmetic mutant no coverage profile reaches, so gremlins reports it as
// never covered although a test holds the value to two minutes.
const maxRateLimitWait time.Duration = 120e9

// rateLimitWait is how long a 429 asks the next attempt to wait: its
// Retry-After, in the whole seconds GitLab writes it in, bounded by
// maxRateLimitWait. Any other failure, and a 429 that names no positive
// number of seconds, asks for nothing.
func rateLimitWait(err error) time.Duration {
	var response *gl.ErrorResponse
	if !errors.As(err, &response) || response.Response == nil || response.Response.StatusCode != http.StatusTooManyRequests {
		return 0
	}
	seconds, parseErr := strconv.Atoi(response.Response.Header.Get("Retry-After"))
	if parseErr != nil || seconds <= 0 {
		return 0
	}
	return min(time.Duration(seconds)*time.Second, maxRateLimitWait)
}

// waitOut waits for d or until ctx ends, whichever comes first. A variable,
// so a test can see the wait asked for without sitting through it.
var waitOut = func(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// describeRetry names the class of a retried failure for the log line, so a
// retried run reads as a story rather than as the same error three times.
//
// It is a chain of ifs rather than a tagless switch because gremlins reports
// the comparison in a case expression as not covered whatever the tests do,
// which would leave the status classes unjudged.
func describeRetry(err error) string {
	if IsTransientNetwork(err) {
		return "transient network error"
	}
	status := statusOf(err)
	if status == http.StatusTooManyRequests {
		return "rate limited"
	}
	if status >= http.StatusInternalServerError {
		return "server error"
	}
	if status == http.StatusNotFound {
		return "not visible yet"
	}
	if branchNotReady(err) {
		return "branch not ready"
	}
	if strings.Contains(err.Error(), "already been taken") {
		return "name collision inside GitLab's own validation"
	}
	return "retryable error"
}

// budget returns the wait a step is allowed, larger on a licensed instance,
// where every write does more work and takes measurably longer.
func budget(e *harness.Env, base, enterprise time.Duration) time.Duration {
	return budgetFor(e.Runtime().Tier.IsEnterprise(), base, enterprise)
}

// budgetFor is [budget] for an instance known to be licensed or not: the
// enterprise wait on a licensed one when it is the longer of the two, and the
// base wait otherwise. It is apart from the Env so a test can ask it about a
// licensed instance, which no detached Env describes.
func budgetFor(licensed bool, base, enterprise time.Duration) time.Duration {
	if licensed && enterprise > base {
		return enterprise
	}
	return base
}

// cleanupContextTimeout bounds one cleanup's own context when the ledger's
// context is already gone, which a hook that runs after the tests uses.
const cleanupContextTimeout = time.Minute

// withCleanupTimeout returns ctx bounded by the cleanup budget when ctx
// carries no deadline of its own.
func withCleanupTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, has := ctx.Deadline(); has {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, cleanupContextTimeout)
}
