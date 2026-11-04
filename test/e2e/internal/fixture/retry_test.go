// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// retry_test.go pins the retry classification: which failures a builder
// tries again and which it reports at once.

package fixture

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestIsTransientNetwork_KnownMessages_AreTransient checks the four
// connection failures a loaded GitLab produces are recognized, and that an
// ordinary error is not.
func TestIsTransientNetwork_KnownMessages_AreTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "EOF", err: errors.New("read tcp 127.0.0.1:8929: EOF"), want: true},
		{name: "reset", err: errors.New("read: connection reset by peer"), want: true},
		{name: "broken pipe", err: errors.New("write: broken pipe"), want: true},
		{name: "refused", err: errors.New("dial tcp: connection refused"), want: true},
		{name: "ordinary", err: errors.New("name has already been taken"), want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := IsTransientNetwork(testCase.err); got != testCase.want {
				t.Errorf("IsTransientNetwork(%v) = %t, want %t", testCase.err, got, testCase.want)
			}
		})
	}
}

// TestIsRetryable_ByClass_MatchesTheDocumentedSet checks each class the
// classification names, through the structured error client-go returns
// rather than through a number in a message.
func TestIsRetryable_ByClass_MatchesTheDocumentedSet(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "network", err: errors.New("unexpected EOF"), want: true},
		{name: "rate limited", err: statusError(http.StatusTooManyRequests, "Retry later"), want: true},
		{name: "server error", err: statusError(http.StatusInternalServerError, "boom"), want: true},
		{name: "bad gateway", err: statusError(http.StatusBadGateway, "nginx"), want: true},
		{name: "unavailable", err: statusError(http.StatusServiceUnavailable, "maintenance"), want: true},
		{name: "not visible yet", err: statusError(http.StatusNotFound, "404 Branch Not Found"), want: true},
		{name: "branch not ready", err: statusError(http.StatusBadRequest, "You can only create or edit files when you are on a branch"), want: true},
		{name: "branch not ready, read off a tool error", err: errors.New(branchRefusedThroughTheServer), want: true},
		{name: "notification row race", err: statusError(http.StatusBadRequest, "Key (user_id) already exists in source"), want: true},
		{name: "forbidden", err: statusError(http.StatusForbidden, "403 Forbidden"), want: false},
		{name: "validation", err: statusError(http.StatusBadRequest, "name is invalid"), want: false},
		{name: "a 404 spelled inside an ID", err: errors.New("project 404123 refused"), want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := IsRetryable(testCase.err); got != testCase.want {
				t.Errorf("IsRetryable(%v) = %t, want %t", testCase.err, got, testCase.want)
			}
		})
	}
}

// TestCreateRetryable_ByEdition_KeepsTheEnterpriseClassesToEnterprise checks
// that the name collision is retried everywhere and the two repository-side
// failures only where they have been seen.
func TestCreateRetryable_ByEdition_KeepsTheEnterpriseClassesToEnterprise(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		enterprise bool
		want       bool
	}{
		{name: "nil", err: nil, enterprise: true, want: false},
		{name: "taken on free", err: statusError(http.StatusBadRequest, "name has already been taken"), enterprise: false, want: true},
		{name: "taken on enterprise", err: statusError(http.StatusBadRequest, "path has already been taken"), enterprise: true, want: true},
		{name: "network on free", err: errors.New("connection reset by peer"), enterprise: false, want: true},
		{name: "repository on free", err: statusError(http.StatusBadRequest, "Failed to create repository"), enterprise: false, want: false},
		{name: "repository on enterprise", err: statusError(http.StatusBadRequest, "Failed to create repository"), enterprise: true, want: true},
		{name: "gitaly on enterprise", err: statusError(http.StatusBadRequest, "Internal API error (502)"), enterprise: true, want: true},
		{name: "forbidden on enterprise", err: statusError(http.StatusForbidden, "403 Forbidden"), enterprise: true, want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := CreateRetryable(testCase.err, testCase.enterprise); got != testCase.want {
				t.Errorf("CreateRetryable(%v, enterprise=%t) = %t, want %t", testCase.err, testCase.enterprise, got, testCase.want)
			}
		})
	}
}

// TestStatusOf_Errors_ReadsTheStructuredStatus checks the status reader
// behind the classification and the log descriptions.
func TestStatusOf_Errors_ReadsTheStructuredStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", err: nil, want: 0},
		{name: "plain", err: errors.New("EOF"), want: 0},
		{name: "structured", err: statusError(http.StatusConflict, "conflict"), want: http.StatusConflict},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := statusOf(testCase.err); got != testCase.want {
				t.Errorf("statusOf(%v) = %d, want %d", testCase.err, got, testCase.want)
			}
			if testCase.want != 0 && !IsStatus(testCase.err, testCase.want) {
				t.Errorf("IsStatus(%v, %d) = false, want true", testCase.err, testCase.want)
			}
		})
	}
}

// TestDescribeRetry_ByClass_NamesTheReason checks the log line each retried
// class produces, since a retried run is read afterwards through those.
func TestDescribeRetry_ByClass_NamesTheReason(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "network", err: errors.New("broken pipe"), want: "transient network error"},
		{name: "rate limited", err: statusError(http.StatusTooManyRequests, ""), want: "rate limited"},
		{name: "server error", err: statusError(http.StatusBadGateway, ""), want: "server error"},
		{name: "the first server error", err: statusError(http.StatusInternalServerError, ""), want: "server error"},
		{name: "the last client error", err: statusError(499, ""), want: "retryable error"},
		{name: "not visible yet", err: statusError(http.StatusNotFound, ""), want: "not visible yet"},
		{name: "branch not ready", err: errors.New(branchRefusedThroughTheServer), want: "branch not ready"},
		{name: "taken", err: statusError(http.StatusBadRequest, "name has already been taken"), want: "name collision inside GitLab's own validation"},
		{name: "other", err: statusError(http.StatusBadRequest, "already exists in source"), want: "retryable error"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := describeRetry(testCase.err); got != testCase.want {
				t.Errorf("describeRetry(%v) = %q, want %q", testCase.err, got, testCase.want)
			}
		})
	}
}

// rateLimited is a 429 whose Retry-After is the value given, written as
// GitLab writes it, or no header at all for an empty one.
func rateLimited(retryAfter string) error {
	return answerWithRetryAfter(http.StatusTooManyRequests, retryAfter)
}

// answerWithRetryAfter is a GitLab answer of the status given, carrying the
// Retry-After given, or none for an empty one.
func answerWithRetryAfter(status int, retryAfter string) error {
	header := http.Header{}
	if retryAfter != "" {
		header.Set("Retry-After", retryAfter)
	}
	return &gl.ErrorResponse{
		Response: &http.Response{StatusCode: status, Header: header, Request: &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Scheme: "https", Host: "gitlab.example", Path: "/api/v4/projects/1/service_accounts"},
		}},
		Message: "This endpoint has been requested too many times. Try again later.",
	}
}

// TestRateLimitWait_ByAnswer_WaitsOnlyWhatA429Names checks what each failure
// asks the next attempt to wait: a 429's Retry-After in seconds, bounded by
// maxRateLimitWait, and nothing for a 429 that names no positive number of
// seconds or for any other failure.
func TestRateLimitWait_ByAnswer_WaitsOnlyWhatA429Names(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want time.Duration
	}{
		{name: "a minute", err: rateLimited("60"), want: time.Minute},
		{name: "past the bound", err: rateLimited("600"), want: 2 * time.Minute},
		{name: "the bound itself", err: rateLimited("120"), want: 2 * time.Minute},
		{name: "a second short of the bound", err: rateLimited("119"), want: 119 * time.Second},
		{name: "one second", err: rateLimited("1"), want: time.Second},
		{name: "no header", err: rateLimited(""), want: 0},
		{name: "zero", err: rateLimited("0"), want: 0},
		{name: "a date", err: rateLimited("Wed, 21 Oct 2026 07:28:00 GMT"), want: 0},
		{name: "a server error with the header", err: answerWithRetryAfter(http.StatusServiceUnavailable, "60"), want: 0},
		{name: "no response", err: &gl.ErrorResponse{Message: "429"}, want: 0},
		{name: "not a GitLab answer", err: errors.New("429 Too Many Requests"), want: 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := rateLimitWait(testCase.err); got != testCase.want {
				t.Errorf("rateLimitWait(%v) = %s, want %s", testCase.err, got, testCase.want)
			}
		})
	}
}

// recordWaits replaces waitOut for one test with one that returns at once
// and records what it was asked to wait.
func recordWaits(t *testing.T) *[]time.Duration {
	t.Helper()
	var waited []time.Duration
	saved := waitOut
	waitOut = func(_ context.Context, d time.Duration) { waited = append(waited, d) }
	t.Cleanup(func() { waitOut = saved })
	return &waited
}

// TestRetryWhenIn_RateLimited_WaitsOutTheWindowBeforeTheNextAttempt drives
// the retry the builders share through a 429 that names a minute and then a
// success: the minute is waited out before the second attempt, which then
// succeeds. A refusal not worth a retry, and a 429 on the last attempt,
// wait for nothing.
func TestRetryWhenIn_RateLimited_WaitsOutTheWindowBeforeTheNextAttempt(t *testing.T) {
	t.Run("waited out", func(t *testing.T) {
		waited := recordWaits(t)
		answers := []error{rateLimited("60"), nil}
		calls := 0
		got, err := retryWhenIn(t.Context(), t, "create service account", 3, IsRetryable, func() (int, error) {
			calls++
			return calls, answers[calls-1]
		})
		if err != nil || got != 2 || !slices.Equal(*waited, []time.Duration{time.Minute}) {
			t.Errorf("retryWhenIn() = %d, %v after waiting %v; want the second attempt's 2 after one minute", got, err, *waited)
		}
	})
	t.Run("last attempt", func(t *testing.T) {
		waited := recordWaits(t)
		_, err := retryWhenIn(t.Context(), t, "create service account", 1, IsRetryable, func() (int, error) {
			return 0, rateLimited("60")
		})
		if err == nil || len(*waited) != 0 {
			t.Errorf("retryWhenIn() error = %v after waiting %v; want the 429 and no wait", err, *waited)
		}
	})
	t.Run("not retried", func(t *testing.T) {
		waited := recordWaits(t)
		_, err := retryWhenIn(t.Context(), t, "create service account", 3, func(error) bool { return false }, func() (int, error) {
			return 0, rateLimited("60")
		})
		if err == nil || len(*waited) != 0 {
			t.Errorf("retryWhenIn() error = %v after waiting %v; want the 429 and no wait", err, *waited)
		}
	})
}

// TestWaitOut_EndsAtTheEarlierOfTheWaitAndTheContext checks the production
// wait both ways: a short wait runs to its end, and a cancelled context ends
// a long one at once.
func TestWaitOut_EndsAtTheEarlierOfTheWaitAndTheContext(t *testing.T) {
	start := time.Now()
	waitOut(t.Context(), 10*time.Millisecond)
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Errorf("waitOut(10ms) returned after %s", elapsed)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	start = time.Now()
	waitOut(ctx, time.Hour)
	if elapsed := time.Since(start); elapsed > time.Minute {
		t.Errorf("waitOut(1h) on a cancelled context returned after %s", elapsed)
	}
}

// TestUserCreateRetryable_ByClass_RetriesTheWeakPasswordRefusal checks the
// user creation's own classification: nothing for no error, every class
// IsRetryable takes, and GitLab's weak-password refusal, which looks
// permanent and is not, since the builder mints a fresh password per attempt.
func TestUserCreateRetryable_ByClass_RetriesTheWeakPasswordRefusal(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "rate limited", err: statusError(http.StatusTooManyRequests, "Retry later"), want: true},
		{name: "weak password", err: statusError(http.StatusBadRequest, "Password must not contain Commonly Used Combinations of words"), want: true},
		{name: "a real refusal", err: statusError(http.StatusBadRequest, "Email has already been taken"), want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := UserCreateRetryable(testCase.err); got != testCase.want {
				t.Errorf("UserCreateRetryable(%v) = %t, want %t", testCase.err, got, testCase.want)
			}
		})
	}
}

// TestBudgetFor_ByInstance_GivesALicensedOneTheLongerWait checks the wait a
// step is allowed: the enterprise wait on a licensed instance when it is the
// longer one, and the base wait on a Free one or when the enterprise wait is
// not longer. A detached Env is Free, which is the half budget itself is
// driven through.
func TestBudgetFor_ByInstance_GivesALicensedOneTheLongerWait(t *testing.T) {
	cases := []struct {
		name       string
		licensed   bool
		base       time.Duration
		enterprise time.Duration
		want       time.Duration
	}{
		{name: "licensed, longer", licensed: true, base: time.Second, enterprise: 2 * time.Second, want: 2 * time.Second},
		{name: "licensed, equal", licensed: true, base: time.Second, enterprise: time.Second, want: time.Second},
		{name: "licensed, shorter", licensed: true, base: 2 * time.Second, enterprise: time.Second, want: 2 * time.Second},
		{name: "free", licensed: false, base: time.Second, enterprise: 2 * time.Second, want: time.Second},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := budgetFor(testCase.licensed, testCase.base, testCase.enterprise); got != testCase.want {
				t.Errorf("budgetFor() = %s, want %s", got, testCase.want)
			}
		})
	}
	if _, e := detachedStub(t); budget(e, time.Second, time.Minute) != time.Second {
		t.Errorf("budget() on a detached Env is not the base wait")
	}
}

// TestWithCleanupTimeout_Deadline_IsAddedOnlyWhenMissing checks that a
// cleanup context the ledger bounded keeps its bound and one from an exit
// hook gets the package's own.
func TestWithCleanupTimeout_Deadline_IsAddedOnlyWhenMissing(t *testing.T) {
	t.Run("unbounded context is bounded", func(t *testing.T) {
		ctx, cancel := withCleanupTimeout(context.Background())
		defer cancel()
		deadline, has := ctx.Deadline()
		if !has {
			t.Fatal("withCleanupTimeout(Background) carries no deadline")
		}
		if remaining := time.Until(deadline); remaining > cleanupContextTimeout || remaining < cleanupContextTimeout/2 {
			t.Errorf("deadline in %s, want about %s", remaining, cleanupContextTimeout)
		}
	})
	t.Run("bounded context keeps its bound", func(t *testing.T) {
		parent, parentCancel := context.WithTimeout(context.Background(), time.Second)
		defer parentCancel()
		ctx, cancel := withCleanupTimeout(parent)
		defer cancel()
		want, _ := parent.Deadline()
		if got, _ := ctx.Deadline(); !got.Equal(want) {
			t.Errorf("deadline = %s, want the parent's %s", got, want)
		}
	})
}
