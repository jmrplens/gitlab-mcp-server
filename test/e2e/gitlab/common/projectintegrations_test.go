//go:build e2e

// projectintegrations_test.go covers the integrations of a project and of
// a group as the project tool offers them: an inert integration set, read,
// listed and deleted; the Jira integration set with placeholder credentials
// GitLab stores without contacting the server; and the group Datadog reads
// of a group that has none, which are refused.
//
// The inert integration is emails-on-push: GitLab only stores the
// recipients and never writes to them during configuration, so no
// external service has to exist for the round trip.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/integrations"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The slugs this file configures.
const (
	inertIntegrationSlug = "emails-on-push"
	jiraIntegrationSlug  = "jira"
)

// jiraPlaceholderURL is the Jira server the integration is pointed at when
// the run has no fixture service. GitLab persists the configuration
// without contacting it, so a name that resolves nowhere is enough.
const jiraPlaceholderURL = "https://jira.example.com"

// integrationSlugs lists the slugs of an integration listing.
func integrationSlugs(listed []integrations.IntegrationItem) []string {
	slugs := make([]string, 0, len(listed))
	for _, integration := range listed {
		slugs = append(slugs, integration.Slug)
	}
	return slugs
}

// jiraURL returns the Jira server to configure: the fixture service's
// stand-in when the run has one, a placeholder otherwise.
func jiraURL(e *harness.Env) string {
	if fixture.HasFixtureService(e) {
		return fixture.ServiceURL(e, "/jira")
	}
	return jiraPlaceholderURL
}

// TestProjectIntegrations_Lifecycle_SetGetListAndDelete lists the
// integrations of a fresh project of each surface's own, sets the inert
// one and Jira, reads and lists them, and deletes both.
//
// Replaces: TestMeta_ProjectIntegrations, TestMeta_ProjectIntegrationConfig
func TestProjectIntegrations_Lifecycle_SetGetListAndDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("integrations"))
		params := map[string]any{"project_id": project.IDParam()}

		// The listing reports active integrations only, so a fresh project
		// lists none.
		empty := harness.Do[integrations.ListOutput](s, actionProjectIntegrationList, params)
		if len(empty.Integrations) != 0 {
			e.T.Errorf("a fresh project lists the integrations %v, want none", integrationSlugs(empty.Integrations))
		}

		set := harness.Do[integrations.SetIntegrationOutput](s, actionProjectIntegrationSet, withParams(params, map[string]any{
			"slug": inertIntegrationSlug, "config": map[string]any{"recipients": "e2e-integrations@example.com", "push_events": true},
		}))
		if !set.Integration.Active || set.Integration.Slug != inertIntegrationSlug {
			e.T.Errorf("integration_set answered %+v, want %s active", set.Integration, inertIntegrationSlug)
		}
		got := harness.Do[integrations.GetOutput](s, actionProjectIntegrationGet, withParams(params, map[string]any{"slug": inertIntegrationSlug}))
		if !got.Integration.Active || got.Integration.ID != set.Integration.ID {
			e.T.Errorf("integration_get answered %+v, want the active integration %d", got.Integration, set.Integration.ID)
		}

		jira := harness.Do[integrations.SetJiraOutput](s, actionProjectIntegrationSetJira, withParams(params, map[string]any{
			"url": jiraURL(e), "username": "e2e-jira-user", "password": "e2e-jira-token", "jira_auth_type": int64(0),
		}))
		if !jira.Integration.Active {
			e.T.Errorf("integration_set_jira answered %+v, want the Jira integration active", jira.Integration)
		}
		listed := harness.Do[integrations.ListOutput](s, actionProjectIntegrationList, params)
		if slugs := integrationSlugs(listed.Integrations); !containsKey(slugs, inertIntegrationSlug) || !containsKey(slugs, jiraIntegrationSlug) {
			e.T.Errorf("the project lists the integrations %v, want both %s and %s", slugs, inertIntegrationSlug, jiraIntegrationSlug)
		}

		harness.DoVoid(s, actionProjectIntegrationDelete, withParams(params, map[string]any{"slug": jiraIntegrationSlug}))
		harness.DoVoid(s, actionProjectIntegrationDelete, withParams(params, map[string]any{"slug": inertIntegrationSlug}))
		remaining := harness.Do[integrations.ListOutput](s, actionProjectIntegrationList, params)
		if len(remaining.Integrations) != 0 {
			e.T.Errorf("the project still lists the integrations %v after both deletes", integrationSlugs(remaining.Integrations))
		}
	})
}

// TestGroupIntegrations_Lifecycle_SetGetListAndDelete sets the inert
// integration on a group of each surface's own, lists and reads it,
// deletes it, and shows the Datadog read and delete of a group that never
// had one refused.
//
// Replaces: TestMeta_ProjectGroupIntegrations
func TestGroupIntegrations_Lifecycle_SetGetListAndDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("integrations"))
		params := map[string]any{"group_id": group.IDParam()}
		inert := withParams(params, map[string]any{"slug": inertIntegrationSlug})

		set := harness.Do[integrations.SetGroupIntegrationOutput](s, actionProjectIntegrationSetGroup, withParams(inert, map[string]any{
			"config": map[string]any{"recipients": "e2e-group-integrations@example.com", "push_events": true},
		}))
		if !set.Integration.Active || set.Integration.Slug != inertIntegrationSlug {
			e.T.Errorf("integration_set_group answered %+v, want %s active", set.Integration, inertIntegrationSlug)
		}
		listed := harness.Do[integrations.ListGroupIntegrationsOutput](s, actionProjectIntegrationListGroup, params)
		if !containsKey(integrationSlugs(listed.Integrations), inertIntegrationSlug) {
			e.T.Errorf("the group lists the integrations %v, want %s among them", integrationSlugs(listed.Integrations), inertIntegrationSlug)
		}
		got := harness.Do[integrations.GetGroupIntegrationOutput](s, actionProjectIntegrationGetGroup, inert)
		if !got.Integration.Active || got.Integration.ID != set.Integration.ID {
			e.T.Errorf("integration_get_group answered %+v, want the active integration %d", got.Integration, set.Integration.ID)
		}

		harness.DoVoid(s, actionProjectIntegrationDeleteGroup, inert)
		remaining := harness.Do[integrations.ListGroupIntegrationsOutput](s, actionProjectIntegrationListGroup, params)
		if containsKey(integrationSlugs(remaining.Integrations), inertIntegrationSlug) {
			e.T.Errorf("the group still lists %s after its delete", inertIntegrationSlug)
		}

		refused := harness.Refused(s, actionProjectIntegrationGetGroupDatadog, params, harness.FailureNotFound)
		assertMentions(e, "the Datadog read of a group that has none", refused, "datadog")
		refused = harness.Refused(s, actionProjectIntegrationDeleteGroupDatadog, params, harness.FailureNotFound)
		e.T.Logf("the Datadog delete of a group that has none is refused: %s", firstLine(refused))
	})
}
