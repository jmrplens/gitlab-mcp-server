// freshness_test.go verifies the harness switch in both directions.
//
// Both halves are load-bearing and neither substitutes for the other. If the
// deferral stopped working, the tests that compare a committed artifact would
// fail on every layer of a stack below its top, which is the cost this package
// exists to remove. If it started applying when nothing asked for it, those
// same comparisons would be skipped where the artifact lands, and a stale
// artifact would reach main unnoticed. So every value the variable can carry
// is asserted, not just the one that defers.
package freshness

import (
	"os"
	"strings"
	"testing"
)

// TestDeferred_OnlyTheDeferredValueDefers verifies exactly one spelling of the
// variable defers and every other reading compares: the value CI passes when
// the artifacts are checked, an unset variable (a developer's machine, and the
// jobs that deliberately set nothing), and the near misses a hand-typed value
// produces.
func TestDeferred_OnlyTheDeferredValueDefers(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "deferred", value: "deferred", want: true},
		{name: "checked", value: "checked", want: false},
		{name: "empty", value: "", want: false},
		{name: "capitalized", value: "Deferred", want: false},
		{name: "padded", value: " deferred ", want: false},
		{name: "truthy", value: "true", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvVar, tt.value)
			if got := Deferred(); got != tt.want {
				t.Errorf("Deferred() with %s=%q = %t, want %t", EnvVar, tt.value, got, tt.want)
			}
		})
	}

	t.Run("unset", func(t *testing.T) {
		// t.Setenv first so its cleanup restores whatever the process had,
		// however the variable is left below.
		t.Setenv(EnvVar, "deferred")
		if err := os.Unsetenv(EnvVar); err != nil {
			t.Fatalf("unset %s: %v", EnvVar, err)
		}
		if Deferred() {
			t.Errorf("Deferred() with %s unset = true, want false: an unset variable must compare", EnvVar)
		}
	})
}

// TestSkipIfDeferred_SkipsOnlyWhenDeferred verifies the one call the tests
// make behaves both ways: it skips when the harness deferred, and it returns
// so the comparison runs when it did not. The skip is observed through
// [testing.T.Skipped] from a deferred function, because [testing.T.Skip] ends
// the subtest's goroutine and nothing after the call would run.
func TestSkipIfDeferred_SkipsOnlyWhenDeferred(t *testing.T) {
	var skipped bool
	t.Run("deferred", func(t *testing.T) {
		t.Setenv(EnvVar, "deferred")
		defer func() { skipped = t.Skipped() }()
		SkipIfDeferred(t)
	})
	if !skipped {
		t.Errorf("SkipIfDeferred() ran the comparison the harness deferred through %s", EnvVar)
	}

	skipped = true
	t.Run("checked", func(t *testing.T) {
		t.Setenv(EnvVar, "checked")
		defer func() { skipped = t.Skipped() }()
		SkipIfDeferred(t)
	})
	if skipped {
		t.Errorf("SkipIfDeferred() skipped a comparison %s=checked asked for", EnvVar)
	}
}

// TestSkipReason_NamesTheSettingAndTheValue verifies the message a skipped run
// prints identifies the decision rather than only announcing one: the variable
// that made it, the value it carried, and where the comparison does happen.
func TestSkipReason_NamesTheSettingAndTheValue(t *testing.T) {
	for _, want := range []string{EnvVar, deferredValue, "top of a stack", "push to main"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(SkipReason, want) {
				t.Errorf("SkipReason = %q, want it to mention %q", SkipReason, want)
			}
		})
	}
}
