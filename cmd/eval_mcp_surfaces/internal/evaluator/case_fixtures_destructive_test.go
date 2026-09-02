// case_fixtures_destructive_test.go drives the destructive and mutating case
// fixtures against a fake GitLab. Each fixture provisions the disposable
// resource its case then destroys, so the assertion is that every declared
// output is populated: that is exactly the contract the fixture engine checks
// before a case runs.

package evaluator

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// newDestructiveFixtureClient starts a fake GitLab that answers the whole
// destructive fixture surface and returns a client bound to it.
func newDestructiveFixtureClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(path, "/discussions"):
			fmt.Fprint(w, gitLabFixtureDiscussion)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/job_token_scope/allowlist"):
			fmt.Fprint(w, `[{"id":101,"path_with_namespace":"my-org/tools/target"}]`)
		default:
			if body, ok := fakeGitLabListBody(r.Method, path); ok {
				fmt.Fprint(w, body)
				return
			}
			fmt.Fprint(w, gitLabFixtureObject)
		}
	}))
	t.Cleanup(server.Close)
	return newFixtureTestClient(t, server.URL)
}

// TestDestructiveCaseFixtures_ProvisionEveryDeclaredOutput verifies each
// destructive and award fixture runs its GitLab provisioning against a fake
// instance and returns every output its spec declares, then passes its own
// Validate hook. A fixture that silently stops producing an identifier its
// case prompt interpolates fails here rather than mid-evaluation.
func TestDestructiveCaseFixtures_ProvisionEveryDeclaredOutput(t *testing.T) {
	client := newDestructiveFixtureClient(t)
	env := FixtureContext{Client: client, RuntimeEdition: EvalCaseEdition(editionCE), ModelName: "model", RunIndex: 1, RunSuffix: "suffix", CaseID: "MT-FIXTURE"}
	fixtures := []CaseFixtureSpec{
		GroupDeleteFixture,
		IssueDeleteFixture,
		ProjectCIVariableDeleteFixture,
		RepositoryFileDeleteFixture,
		MilestoneDeleteFixture,
		ReleaseDeleteFixture,
		ProjectAccessTokenRevokeFixture,
		ProjectArchiveFixture,
		PackageDeleteFixture,
		PipelineDeleteFixture,
		PipelineTriggerDeleteFixture,
		PipelineScheduleDeleteFixture,
		RunnerRemoveFixture,
		EnvironmentStopFixture,
		SnippetDeleteFixture,
		BroadcastMessageDeleteFixture,
		ProjectHookDeleteFixture,
		ProjectBadgeDeleteFixture,
		DraftNotePublishAllFixture,
		InstanceCIVariableDeleteFixture,
		BranchDeleteFixture,
		TagDeleteFixture,
		UserBlockFixture,
		FeatureFlagDeleteFixture,
		WikiDeleteFixture,
		DeployKeyLifecycleFixture,
		DeployKeyDeleteFixture,
		DeployTokenDeleteFixture,
		CommitDiscussionDeleteNoteFixture,
		BranchProtectionLifecycleFixture,
		JobTokenScopeProjectFixture,
		FailedJobArtifactFixture,
		MergeRequestAwardEmojiFixture,
		IssueAwardEmojiFixture,
		ProjectServiceAccountFixture,
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			output, err := fixture.Ensure(t.Context(), env)
			if err != nil {
				t.Fatalf("%s Ensure() error = %v", fixture.Name, err)
			}
			if validateErr := validateFixtureOutput(fixture, output); validateErr != nil {
				t.Fatalf("%s outputs = %v, error = %v", fixture.Name, output, validateErr)
			}
			if fixture.Validate == nil {
				t.Fatalf("%s has no Validate hook", fixture.Name)
			}
			if validateErr := fixture.Validate(t.Context(), env, output); validateErr != nil {
				t.Fatalf("%s Validate() error = %v", fixture.Name, validateErr)
			}
		})
	}
}

// TestMergeableMergeRequestFixture_PreparesAnApprovallessMergeableMR verifies
// the merge fixture creates its own project and branch, clears the approval
// requirements, and only returns once GitLab reports the merge request
// mergeable.
func TestMergeableMergeRequestFixture_PreparesAnApprovallessMergeableMR(t *testing.T) {
	var patched bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/merge_requests"):
			fmt.Fprint(w, `[{"id":1,"iid":7,"detailed_merge_status":"mergeable"}]`)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/approval_rules"):
			fmt.Fprint(w, `[{"id":5,"approvals_required":1,"approved":false}]`)
		case strings.HasSuffix(path, "/approvals"):
			patched = true
			fmt.Fprint(w, `{"approvals_required":0,"approvals_left":0}`)
		default:
			fmt.Fprint(w, gitLabFixtureObject)
		}
	}))
	defer server.Close()
	env := FixtureContext{Client: newFixtureTestClient(t, server.URL), ModelName: "model", RunIndex: 1, RunSuffix: "suffix"}

	output, err := MergeableMergeRequestFixture.Ensure(t.Context(), env)
	if err != nil {
		t.Fatalf("MergeableMergeRequestFixture.Ensure() error = %v", err)
	}
	if validateErr := validateFixtureOutput(MergeableMergeRequestFixture, output); validateErr != nil {
		t.Fatalf("outputs = %v, error = %v", output, validateErr)
	}
	if output["merge_request_iid"] != "7" || !strings.HasPrefix(output["source_branch"], "eval-merge-") {
		t.Fatalf("output = %v, want the seeded merge request and branch", output)
	}
	if !patched {
		t.Fatal("approval configuration was never cleared")
	}
}

// TestMergeableMergeRequestFixture_WithoutClient_ReturnsError verifies the
// fixture refuses to run when no GitLab client was supplied.
func TestMergeableMergeRequestFixture_WithoutClient_ReturnsError(t *testing.T) {
	_, err := MergeableMergeRequestFixture.Ensure(t.Context(), FixtureContext{})
	if err == nil || !strings.Contains(err.Error(), "mergeable merge request fixture requires GitLab client") {
		t.Fatalf("Ensure() error = %v, want missing client error", err)
	}
}

// TestCanIgnoreApprovalConfigurationError_AcceptsUnsupportedEndpoints
// verifies the approval cleanup treats a CE instance without the Enterprise
// approval endpoints as success and still reports other failures.
func TestCanIgnoreApprovalConfigurationError_AcceptsUnsupportedEndpoints(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{name: "bad request", status: http.StatusBadRequest, want: true},
		{name: "forbidden", status: http.StatusForbidden, want: true},
		{name: "not found", status: http.StatusNotFound, want: true},
		{name: "server error", status: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &gl.ErrorResponse{Response: &http.Response{StatusCode: tc.status}, Message: "boom"}
			if got := canIgnoreApprovalConfigurationError(err); got != tc.want {
				t.Fatalf("canIgnoreApprovalConfigurationError(%d) = %t, want %t", tc.status, got, tc.want)
			}
		})
	}
}

// TestValidateJobTokenScopeAllowlistTarget_MissingTarget_ReturnsError verifies
// the job token fixture fails when the target project never reaches the
// source project's inbound allowlist.
func TestValidateJobTokenScopeAllowlistTarget_MissingTarget_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":999}]`)
	}))
	defer server.Close()
	err := validateJobTokenScopeAllowlistTarget(t.Context(), newFixtureTestClient(t, server.URL), 1, 2)
	if err == nil || !strings.Contains(err.Error(), "target project 2 is not in source project 1 allowlist") {
		t.Fatalf("validateJobTokenScopeAllowlistTarget() error = %v, want missing target error", err)
	}
}

// TestSeedMergeRequestFixture_CreateFails_ReturnsWrappedError verifies a file
// commit that GitLab rejects is reported with the merge-request fixture
// context rather than as a bare API error.
func TestSeedMergeRequestFixture_CreateFails_ReturnsWrappedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"500"}`)
	}))
	defer server.Close()
	err := seedMergeRequestFixture(t.Context(), newFixtureTestClient(t, server.URL), "my-org/app", "feature/x")
	if err == nil || !strings.Contains(err.Error(), "prepare mergeable MR file") {
		t.Fatalf("seedMergeRequestFixture() error = %v, want wrapped commit failure", err)
	}
}

// TestCreateMergeableMRTemporaryProject_NonCollisionFailure_StopsRetrying
// verifies project creation retries only a name collision and reports any
// other failure immediately.
func TestCreateMergeableMRTemporaryProject_NonCollisionFailure_StopsRetrying(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"403 Forbidden"}`)
	}))
	defer server.Close()
	_, err := createMergeableMRTemporaryProject(t.Context(), newFixtureTestClient(t, server.URL))
	if err == nil || !strings.Contains(err.Error(), "create standalone project") {
		t.Fatalf("createMergeableMRTemporaryProject() error = %v, want creation failure", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want a single attempt for a non-collision failure", attempts)
	}
}

// TestDestructiveAttemptFixture_EnsureFailure_PropagatesError verifies a
// destructive fixture whose provisioning step fails reports the error instead
// of returning a partial output.
func TestDestructiveAttemptFixture_EnsureFailure_PropagatesError(t *testing.T) {
	client := newDestructiveFixtureClient(t)
	fixture := destructiveAttemptFixture("boom", []string{"project_id"}, func(context.Context, *liveFixturePreparer) error {
		return errors.New("provisioning failed")
	}, nil)
	if _, err := fixture.Ensure(t.Context(), FixtureContext{Client: client}); err == nil || !strings.Contains(err.Error(), "provisioning failed") {
		t.Fatalf("Ensure() error = %v, want provisioning failure", err)
	}
}

// TestDestructiveAttemptFixture_WithoutClient_ReturnsError verifies the shared
// destructive fixture builder refuses to provision without a GitLab client.
func TestDestructiveAttemptFixture_WithoutClient_ReturnsError(t *testing.T) {
	fixture := destructiveAttemptFixture("no-client", []string{"project_id"}, nil, nil)
	if _, err := fixture.Ensure(t.Context(), FixtureContext{}); err == nil || !strings.Contains(err.Error(), "typed live fixture requires GitLab client") {
		t.Fatalf("Ensure() error = %v, want missing client error", err)
	}
}
