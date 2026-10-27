// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// client_access.go waits for a credential that is not a user's own token to
// see the projects it was given: a group or project access token, whose bot
// user GitLab grants the projects of its group from a background job and
// answers 404 for them until that job has run.

package fixture

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// AwaitClientProjectAccess holds until client can read every project named.
// It is the wait AwaitProjectAccess makes for a user, which asks with a
// classic token it mints for that user, asked instead with a client the
// caller built on the credential its session runs on: a group or project
// access token, whose bot user GitLab created along with it.
//
// A scenario whose subject is the refusal such a credential gets needs it
// before its first call, since until the grant lands GitLab answers that call
// 404, for a project the credential cannot see, which is not the refusal the
// scenario is about. The wait and its budget are the user wait's, so a
// change to either reaches both.
func AwaitClientProjectAccess(e *harness.Env, client *gitlabclient.Client, projects ...Project) {
	e.T.Helper()
	ids := make([]int64, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ID)
	}
	DrainSidekiq(e.Ctx, e.Client())
	if err := waitForGrantedAccess(e.Ctx, client, []GranularScope{{ProjectIDs: ids}}, budget(e, accessWait, enterpriseAccessWait)); err != nil {
		e.T.Fatalf("waiting for the client to read projects %v: %v", ids, err)
	}
}
