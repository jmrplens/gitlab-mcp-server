//go:build e2e

// feature_flag_test.go drives the project feature flag builder's pure halves
// against the stub, and pins the shape a flag has to be created in: version 2
// with one default strategy, which is what GitLab accepts without naming an
// environment.

package fixture

import (
	"net/http"
	"testing"
)

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
