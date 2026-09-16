//go:build e2e

// job_token_scope.go puts one project on another's CI job token inbound
// allowlist.
//
// Two steps, and the first is easy to forget: the allowlist is only consulted
// while the source project's job token scope is switched on, and GitLab
// accepts an entry added to a project whose scope is off without saying that
// nothing will read it. The builder therefore switches it on first and reads
// the allowlist back afterwards, so a fixture that silently built nothing
// fails here rather than in the case that depends on it.

package fixture

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// allowlistPageSize is how many projects one allowlist page carries. The
// fixture's own allowlist holds one entry; the page size is what keeps the
// read-back from paging on a project somebody else configured.
const allowlistPageSize = 100

// JobTokenScope is the allowlist entry a builder created.
type JobTokenScope struct {
	// SourceProjectID is the project whose job token may reach the target.
	SourceProjectID int64
	// TargetProjectID is the project the entry names, and is the fact a
	// case addresses the entry by.
	TargetProjectID int64
}

// NewJobTokenScope switches the source project's job token scope on, adds the
// target to its inbound allowlist, and registers the entry's removal.
func NewJobTokenScope(e *harness.Env, source, target Project) JobTokenScope {
	e.T.Helper()

	scope, err := retryTransient(e, "allow job token project", createRetries, func() (JobTokenScope, error) {
		return allowJobTokenProject(e.Ctx, e.Client(), source.ID, target.ID)
	})
	if err != nil {
		e.T.Fatalf("allowing project %d on the job token allowlist of project %d: %v", target.ID, source.ID, err)
	}

	e.Defer(fmt.Sprintf("job token allowlist entry %d of project %d", target.ID, source.ID), func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		_, removeErr := e.Client().GL().JobTokenScope.RemoveProjectFromJobScopeAllowList(source.ID, target.ID, gl.WithContext(ctx))
		if removeErr != nil && !IsStatus(removeErr, http.StatusNotFound) {
			return fmt.Errorf("removing project %d from the job token allowlist of project %d: %w", target.ID, source.ID, removeErr)
		}
		return nil
	})
	return scope
}

// allowJobTokenProject is the whole of what the builder asks GitLab for: the
// setting, the entry, and the read-back that proves the entry is there.
func allowJobTokenProject(ctx context.Context, client *gitlabclient.Client, sourceID, targetID int64) (JobTokenScope, error) {
	if _, err := client.GL().JobTokenScope.PatchProjectJobTokenAccessSettings(sourceID,
		&gl.PatchProjectJobTokenAccessSettingsOptions{Enabled: true}, gl.WithContext(ctx)); err != nil {
		return JobTokenScope{}, fmt.Errorf("switching the job token scope of project %d on: %w", sourceID, err)
	}

	_, _, addErr := client.GL().JobTokenScope.AddProjectToJobScopeAllowList(sourceID,
		&gl.JobTokenInboundAllowOptions{TargetProjectID: &targetID}, gl.WithContext(ctx))
	// A refused add is not judged here, because the refusal a retried attempt
	// gets is not a status anything can match on: GitLab answers an entry a
	// previous attempt already added with 400 and "This project is already in
	// the job token allowlist.", since the endpoint hands the service's
	// message to bad_request! rather than reporting a conflict. The read-back
	// decides instead, which is the question the builder actually has: an
	// allowlist holding the target is what the caller asked for however the
	// add ended, and one that does not carries the add's own refusal, which
	// is what names the cause.
	if confirmErr := confirmJobTokenAllowlist(ctx, client, sourceID, targetID); confirmErr != nil {
		if addErr == nil {
			return JobTokenScope{}, confirmErr
		}
		return JobTokenScope{}, fmt.Errorf("adding project %d to the allowlist of project %d: %w",
			targetID, sourceID, errors.Join(addErr, confirmErr))
	}
	return JobTokenScope{SourceProjectID: sourceID, TargetProjectID: targetID}, nil
}

// confirmJobTokenAllowlist reads the allowlist back and reports one that does
// not name the target.
func confirmJobTokenAllowlist(ctx context.Context, client *gitlabclient.Client, sourceID, targetID int64) error {
	allowed, _, err := client.GL().JobTokenScope.GetProjectJobTokenInboundAllowList(sourceID,
		&gl.GetJobTokenInboundAllowListOptions{PerPage: allowlistPageSize}, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading the job token allowlist of project %d: %w", sourceID, err)
	}
	for _, project := range allowed {
		if project != nil && project.ID == targetID {
			return nil
		}
	}
	return fmt.Errorf("project %d is not on the job token allowlist of project %d after being added", targetID, sourceID)
}
