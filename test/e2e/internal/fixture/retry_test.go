//go:build e2e

// retry_test.go pins the retry classification: which failures a builder
// tries again and which it reports at once.

package fixture

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
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
		{name: "not visible yet", err: statusError(http.StatusNotFound, ""), want: "not visible yet"},
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
