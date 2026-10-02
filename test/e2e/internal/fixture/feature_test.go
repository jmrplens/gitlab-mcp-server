//go:build e2e

// feature_test.go drives the wait a flag change costs its next reader, which
// is the half of the feature fixture that needs no GitLab.

package fixture

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestAwaitFlagSettled_WaitsOutTheProcessCacheAfterAChange checks the three
// answers: a flag this process never changed is settled at once, a change is
// waited out for the lifetime given, measured from the change and not from the
// call, and a context that ends first ends the wait with its error, naming the
// flag.
func TestAwaitFlagSettled_WaitsOutTheProcessCacheAfterAChange(t *testing.T) {
	forget := func(name string) {
		flagChanges.Lock()
		defer flagChanges.Unlock()
		delete(flagChanges.at, name)
	}

	t.Run("never changed", func(t *testing.T) {
		started := time.Now()
		if err := awaitFlagSettled(t.Context(), "never_changed_flag", time.Hour); err != nil {
			t.Fatalf("awaitFlagSettled() = %v, want nil", err)
		}
		if waited := time.Since(started); waited > time.Second {
			t.Errorf("a flag nothing changed was waited on for %s", waited)
		}
	})

	t.Run("changed a moment ago", func(t *testing.T) {
		const name = "changed_flag"
		t.Cleanup(func() { forget(name) })
		noteFlagChange(name)
		const lifetime = 150 * time.Millisecond
		if err := awaitFlagSettled(t.Context(), name, lifetime); err != nil {
			t.Fatalf("awaitFlagSettled() = %v, want nil", err)
		}
		flagChanges.Lock()
		changed := flagChanges.at[name]
		flagChanges.Unlock()
		if since := time.Since(changed); since < lifetime {
			t.Errorf("the wait ended %s after the change, before the %s lifetime", since, lifetime)
		}
	})

	t.Run("changed long enough ago", func(t *testing.T) {
		const name = "settled_flag"
		t.Cleanup(func() { forget(name) })
		flagChanges.Lock()
		flagChanges.at[name] = time.Now().Add(-time.Hour)
		flagChanges.Unlock()
		started := time.Now()
		if err := awaitFlagSettled(t.Context(), name, time.Minute); err != nil {
			t.Fatalf("awaitFlagSettled() = %v, want nil", err)
		}
		if waited := time.Since(started); waited > time.Second {
			t.Errorf("a flag changed an hour ago was waited on for %s", waited)
		}
	})

	t.Run("the context ends first", func(t *testing.T) {
		const name = "unsettled_flag"
		t.Cleanup(func() { forget(name) })
		noteFlagChange(name)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := awaitFlagSettled(ctx, name, time.Hour)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("awaitFlagSettled() = %v, want the context's cancellation", err)
		}
		if got := err.Error(); !strings.Contains(got, name) {
			t.Errorf("the error %q does not name the flag", got)
		}
	})
}
