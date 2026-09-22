package config

import (
	"strings"
	"testing"
	"time"
)

// TestLoadAuthBudgetEnv_Defaults_WhenNothingIsSet checks the figures a
// deployment gets when it says nothing, which is the configuration almost
// every deployment runs.
func TestLoadAuthBudgetEnv_Defaults_WhenNothingIsSet(t *testing.T) {
	for _, name := range []string{
		"AUTH_FAILURE_LIMIT", "AUTH_FAILURE_WINDOW",
		"AUTH_DISTINCT_TOKEN_LIMIT", "AUTH_DISTINCT_TOKEN_WINDOW",
	} {
		t.Setenv(EnvPrefix+name, "")
	}

	got, err := loadAuthBudgetEnv()
	if err != nil {
		t.Fatalf("loadAuthBudgetEnv() error = %v, want nil", err)
	}
	want := authBudgetEnv{
		failureLimit:   DefaultAuthFailureLimit,
		failureWindow:  DefaultAuthFailureWindow,
		distinctLimit:  DefaultAuthDistinctTokenLimit,
		distinctWindow: DefaultAuthDistinctWindow,
	}
	if got != want {
		t.Errorf("loadAuthBudgetEnv() = %+v, want %+v", got, want)
	}
}

// TestLoadAuthBudgetEnv_ReadsEachSetting covers that each of the four is read
// from its own variable, so a value cannot land in the wrong budget.
func TestLoadAuthBudgetEnv_ReadsEachSetting(t *testing.T) {
	t.Setenv(EnvPrefix+"AUTH_FAILURE_LIMIT", "4")
	t.Setenv(EnvPrefix+"AUTH_FAILURE_WINDOW", "30s")
	t.Setenv(EnvPrefix+"AUTH_DISTINCT_TOKEN_LIMIT", "9")
	t.Setenv(EnvPrefix+"AUTH_DISTINCT_TOKEN_WINDOW", "5m")

	got, err := loadAuthBudgetEnv()
	if err != nil {
		t.Fatalf("loadAuthBudgetEnv() error = %v, want nil", err)
	}
	want := authBudgetEnv{
		failureLimit:   4,
		failureWindow:  30 * time.Second,
		distinctLimit:  9,
		distinctWindow: 5 * time.Minute,
	}
	if got != want {
		t.Errorf("loadAuthBudgetEnv() = %+v, want %+v", got, want)
	}
}

// TestLoadAuthBudgetEnv_ZeroTurnsABudgetOff pins the meaning of zero, which is
// the value an operator reaches for to disable one and the value that must not
// be read as a budget of zero.
func TestLoadAuthBudgetEnv_ZeroTurnsABudgetOff(t *testing.T) {
	t.Setenv(EnvPrefix+"AUTH_FAILURE_LIMIT", "0")
	t.Setenv(EnvPrefix+"AUTH_FAILURE_WINDOW", "0")
	t.Setenv(EnvPrefix+"AUTH_DISTINCT_TOKEN_LIMIT", "0")
	t.Setenv(EnvPrefix+"AUTH_DISTINCT_TOKEN_WINDOW", "0")

	got, err := loadAuthBudgetEnv()
	if err != nil {
		t.Fatalf("loadAuthBudgetEnv() with zeros error = %v, want nil", err)
	}
	if got != (authBudgetEnv{}) {
		t.Errorf("loadAuthBudgetEnv() = %+v, want every field zero", got)
	}
}

// TestLoadAuthBudgetEnv_RefusesWhatItCannotRead covers every rejection path: a
// value that is not a number or a duration, and one past the maximum.
//
// Each is a refusal rather than a silent fallback, because a typo in a
// deployment manifest that quietly ran with a default the operator did not
// choose is indistinguishable from the setting working.
func TestLoadAuthBudgetEnv_RefusesWhatItCannotRead(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		value   string
		wantErr string
	}{
		{name: "failure limit is not a number", env: "AUTH_FAILURE_LIMIT", value: "ten", wantErr: "AUTH_FAILURE_LIMIT"},
		{name: "failure limit over the maximum", env: "AUTH_FAILURE_LIMIT", value: "100001", wantErr: "maximum"},
		{name: "failure window is not a duration", env: "AUTH_FAILURE_WINDOW", value: "a while", wantErr: "AUTH_FAILURE_WINDOW"},
		{name: "failure window over the maximum", env: "AUTH_FAILURE_WINDOW", value: "25h", wantErr: "AUTH_FAILURE_WINDOW"},
		{name: "distinct limit is not a number", env: "AUTH_DISTINCT_TOKEN_LIMIT", value: "many", wantErr: "AUTH_DISTINCT_TOKEN_LIMIT"},
		{name: "distinct limit over the maximum", env: "AUTH_DISTINCT_TOKEN_LIMIT", value: "100001", wantErr: "maximum"},
		{name: "distinct window is not a duration", env: "AUTH_DISTINCT_TOKEN_WINDOW", value: "soon", wantErr: "AUTH_DISTINCT_TOKEN_WINDOW"},
		{name: "distinct window over the maximum", env: "AUTH_DISTINCT_TOKEN_WINDOW", value: "25h", wantErr: "AUTH_DISTINCT_TOKEN_WINDOW"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvPrefix+tc.env, tc.value)

			got, err := loadAuthBudgetEnv()
			if err == nil {
				t.Fatalf("loadAuthBudgetEnv() with %s=%q = %+v, want a refusal", tc.env, tc.value, got)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to name %q", err, tc.wantErr)
			}
			if got != (authBudgetEnv{}) {
				t.Errorf("a refusal returned %+v, want the zero value so a caller cannot use half a reading", got)
			}
		})
	}
}
