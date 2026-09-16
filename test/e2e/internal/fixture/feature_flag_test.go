//go:build e2e

// feature_flag_test.go drives the project feature flag builder's pure halves
// against the stub, and pins the shape a flag has to be created in: version 2
// with one default strategy, which is what GitLab accepts without naming an
// environment, and a name inside the bounds GitLab holds a flag name to.

package fixture

import (
	"net/http"
	"regexp"
	"testing"
)

// What GitLab accepts as a project feature flag name, read from the model
// that validates it: `length: 2..63` and Gitlab::Regex.feature_flag_regex in
// app/models/operations/feature_flag.rb.
const (
	featureFlagNameMinLength = 2
	featureFlagNameMaxLength = 63
)

var featureFlagNamePattern = regexp.MustCompile(`\A[a-z]([-_a-z0-9]*[a-z0-9])?\z`)

// TestFeatureFlagNameFrom_RunScoping_StaysInsideGitLabsBounds checks the one
// property a flag name has to have: whatever run-scoped name it is built
// from, it is short enough and spelled the way GitLab's own validation
// demands.
//
// The long cases are not hypothetical. A run-scoped name is the run ID and
// the test name, which is already fifty-five characters with no test name at
// all and passes ninety under a nested subtest, so a name passed through
// unchanged would have every create refused with "Name is too long".
func TestFeatureFlagNameFrom_RunScoping_StaysInsideGitLabsBounds(t *testing.T) {
	cases := []struct {
		name  string
		given string
	}{
		{name: "empty", given: ""},
		{name: "a plain prefix", given: "flag"},
		{
			name:  "a run-scoped name",
			given: "flag-20260916t101112z-a97711a8e8-fixture-b3f0a1c2d4-1",
		},
		{
			name:  "a nested subtest name",
			given: "flag-testmodeleval-claude-opus-5-issue-create-d-20260916t101112z-a97711a8e8-fixture-b3f0a1c2d4-12",
		},
		{name: "characters GitLab refuses", given: "FLAG.Name_With/Slashes-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := featureFlagNameFrom(tc.given)
			if len(got) < featureFlagNameMinLength || len(got) > featureFlagNameMaxLength {
				t.Errorf("featureFlagNameFrom(%q) = %q, %d characters, want between %d and %d",
					tc.given, got, len(got), featureFlagNameMinLength, featureFlagNameMaxLength)
			}
			if !featureFlagNamePattern.MatchString(got) {
				t.Errorf("featureFlagNameFrom(%q) = %q, want a name matching %s", tc.given, got, featureFlagNamePattern)
			}
		})
	}
}

// TestFeatureFlagNameFrom_TwoNames_StayApart checks the uniqueness the hash
// is there for: two flags built in one project come from two run-scoped
// names, and a name that collapsed them would have the second create refused
// as taken.
func TestFeatureFlagNameFrom_TwoNames_StayApart(t *testing.T) {
	first := featureFlagNameFrom("flag-testcase-20260916t101112z-a97711a8e8-fixture-b3f0a1c2d4-1")
	second := featureFlagNameFrom("flag-testcase-20260916t101112z-a97711a8e8-fixture-b3f0a1c2d4-2")
	if first == second {
		t.Errorf("featureFlagNameFrom() named two flags of one project %q, want two names", first)
	}
}

// TestCreateProjectFeatureFlag_Created_SendsVersionTwoAndADefaultStrategy
// checks the two fields GitLab refuses the create without, and reads back
// the name a case addresses the flag by.
func TestCreateProjectFeatureFlag_Created_SendsVersionTwoAndADefaultStrategy(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/4/feature_flags", stubCreated(map[string]any{
		"name": "e2e-flag", "active": true, "version": featureFlagVersion,
	}))

	got, err := createProjectFeatureFlag(t.Context(), client, 4, "e2e-flag")
	if err != nil {
		t.Fatalf("createProjectFeatureFlag() error = %v, want nil", err)
	}
	want := ProjectFeatureFlag{Name: "e2e-flag", Active: true}
	if got != want {
		t.Errorf("createProjectFeatureFlag() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createProjectFeatureFlag() sent %d requests, want 1", len(requests))
	}
	body := requests[0].Body
	if body["version"] != featureFlagVersion {
		t.Errorf("createProjectFeatureFlag() sent version %v, want %q", body["version"], featureFlagVersion)
	}
	// Read from what was sent rather than from what the stub answered: the
	// flag a case switches off has to arrive switched on, and the answer
	// would say so whatever the builder asked for.
	if body["active"] != true {
		t.Errorf("createProjectFeatureFlag() sent active %v, want true", body["active"])
	}
	strategies, _ := body["strategies"].([]any)
	if len(strategies) != 1 {
		t.Fatalf("createProjectFeatureFlag() sent %d strategies, want 1", len(strategies))
	}
	strategy, _ := strategies[0].(map[string]any)
	if strategy["name"] != featureFlagStrategyName {
		t.Errorf("createProjectFeatureFlag() sent strategy %v, want the default one", strategy["name"])
	}
	scopes, _ := strategy["scopes"].([]any)
	if len(scopes) != 1 {
		t.Fatalf("createProjectFeatureFlag() sent %d scopes, want the one wildcard scope", len(scopes))
	}
	scope, _ := scopes[0].(map[string]any)
	if scope["environment_scope"] != featureFlagScope {
		t.Errorf("createProjectFeatureFlag() sent environment_scope %v, want %q", scope["environment_scope"], featureFlagScope)
	}
}

// TestDeleteProjectFeatureFlag_Endings_ToleratesOneACaseDeleted checks that a
// flag a case already deleted is not a cleanup failure and any other refusal
// is.
func TestDeleteProjectFeatureFlag_Endings_ToleratesOneACaseDeleted(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "deleted", answer: stubNoContent()},
		{name: "already gone", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/projects/4/feature_flags/e2e-flag", tc.answer)

			err := deleteProjectFeatureFlag(t.Context(), client, 4, "e2e-flag")
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteProjectFeatureFlag() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
