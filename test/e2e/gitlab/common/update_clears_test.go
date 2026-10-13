//go:build e2e

// update_clears_test.go holds the update actions to sending the value that
// empties a field. Each scenario sets a field through the server, sends it
// empty, and reads the object back, once per surface. Every one of these
// fields used to be dropped for being empty, so the update reported success
// with the old value or GitLab answered that nothing was sent (issue 1306).

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/broadcastmessages"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/civariables"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/snippets"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestUpdateClears_Project_DescriptionTopicsAndCISettings sets a project's
// description, topics, CI configuration path, clone depth and pipeline
// retention, then sends each empty or 0 in one update and holds the answer and
// a fresh read to a project with none of them: a clone depth of 0 is stored as
// 0, and a retention of 0 goes out as null, which is what stops the deletion.
func TestUpdateClears_Project_DescriptionTopicsAndCISettings(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("projclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		set := harness.Do[projects.Output](s, actionProjectUpdate, withParams(params, map[string]any{
			"description": "to be removed", "topics": []string{"projclear"}, "ci_config_path": "ci/pipeline.yml",
			"ci_default_git_depth": 20, "ci_delete_pipelines_in_seconds": 86400,
		}))
		if set.Description == "" || len(set.Topics) != 1 || set.CIConfigPath == "" ||
			set.CIDefaultGitDepth != 20 || set.CIDeletePipelinesInSeconds != 86400 {
			e.T.Fatalf("project update answered %+v, want a description, one topic, a CI configuration path, a clone depth of 20 and a retention of a day to remove", set)
		}

		cleared := harness.Do[projects.Output](s, actionProjectUpdate, withParams(params, map[string]any{
			"description": "", "topics": []string{}, "ci_config_path": "",
			"ci_default_git_depth": 0, "ci_delete_pipelines_in_seconds": 0,
		}))
		read := harness.Do[projects.Output](s, actionProjectGet, params)
		assertCleared := func(call string, out projects.Output) {
			if out.Description != "" || len(out.Topics) != 0 || out.CIConfigPath != "" {
				e.T.Errorf("project %s after the removal answered description %q, topics %v, CI configuration path %q; want none",
					call, out.Description, out.Topics, out.CIConfigPath)
			}
			if out.CIDefaultGitDepth != 0 || out.CIDeletePipelinesInSeconds != 0 {
				e.T.Errorf("project %s after the removal answered clone depth %d, retention %d seconds; want 0 and none",
					call, out.CIDefaultGitDepth, out.CIDeletePipelinesInSeconds)
			}
		}
		assertCleared("update", cleared)
		assertCleared("get", read)
	})
}

// TestUpdateClears_ProjectMilestone_DescriptionAndDates creates a milestone
// with a description and both dates, then sends all three empty in one update
// and holds the answer and a fresh read to a milestone with none of them.
func TestUpdateClears_ProjectMilestone_DescriptionAndDates(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("msclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		created := harness.Do[milestones.Output](s, actionProjectMilestoneCreate, withParams(params, map[string]any{
			"title": e.Name("msclear"), "description": "to be removed", "start_date": "2030-01-01", "due_date": "2030-01-31",
		}))
		if created.Description == "" || created.StartDate == "" || created.DueDate == "" {
			e.T.Fatalf("milestone create answered %+v, want a description and both dates to remove", created)
		}
		milestoneParams := withParams(params, map[string]any{"milestone_iid": created.IID})

		cleared := harness.Do[milestones.Output](s, actionProjectMilestoneUpdate, withParams(milestoneParams, map[string]any{
			"description": "", "start_date": "", "due_date": "",
		}))
		read := harness.Do[milestones.Output](s, actionProjectMilestoneGet, milestoneParams)
		assertCleared := func(call string, out milestones.Output) {
			if out.Description != "" || out.StartDate != "" || out.DueDate != "" {
				e.T.Errorf("milestone %s after the removal answered description %q, start %q, due %q; want none",
					call, out.Description, out.StartDate, out.DueDate)
			}
		}
		assertCleared("update", cleared)
		assertCleared("get", read)
	})
}

// TestUpdateClears_ProjectLabel_Description creates a label with a
// description, then sends it empty and holds a fresh read to a label with
// none.
func TestUpdateClears_ProjectLabel_Description(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("labelclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		name := e.Name("labelclear")

		created := harness.Do[labels.Output](s, actionProjectLabelCreate, withParams(params, map[string]any{
			"name": name, "color": labelColorBlue, "description": "to be removed",
		}))
		if created.Description == "" {
			e.T.Fatalf("label create answered %+v, want a description to remove", created)
		}
		byName := withParams(params, map[string]any{"name": name})

		cleared := harness.Do[labels.Output](s, actionProjectLabelUpdate, withParams(byName, map[string]any{"description": ""}))
		read := harness.Do[labels.Output](s, actionProjectLabelGet, withParams(params, map[string]any{"label_id": name}))
		assertCleared := func(call string, out labels.Output) {
			if out.Description != "" {
				e.T.Errorf("label %s after the removal answered description %q, want none", call, out.Description)
			}
		}
		assertCleared("update", cleared)
		assertCleared("get", read)
	})
}

// TestUpdateClears_CIVariable_ValueAndDescription creates a variable with a
// value and a description, then sends both empty and holds the answer and a
// fresh read to a variable holding neither.
func TestUpdateClears_CIVariable_ValueAndDescription(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("varclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam(), "key": variableKeyFor("E2E_CLEAR", surface)}

		created := harness.Do[civariables.Output](s, actionCIVariableCreate, withParams(withParams(params, variableCreateDefaults), map[string]any{
			"value": "to-be-emptied", "description": "to be removed",
		}))
		if created.Value == "" || created.Description == "" {
			e.T.Fatalf("variable create answered %+v, want a value and a description to remove", created)
		}

		cleared := harness.Do[civariables.Output](s, actionCIVariableUpdate, withParams(withParams(params, variableUpdateDefaults), map[string]any{
			"value": "", "description": "",
		}))
		read := harness.Do[civariables.Output](s, actionCIVariableGet, withParams(params, variableScopeDefault))
		assertCleared := func(call string, out civariables.Output) {
			if out.Value != "" || out.Description != "" {
				e.T.Errorf("variable %s after the removal answered value %q, description %q; want neither", call, out.Value, out.Description)
			}
		}
		assertCleared("update", cleared)
		assertCleared("get", read)
	})
}

// TestUpdateClears_Environment_DescriptionAndExternalURL creates an
// environment with a description and an external URL, then sends both empty
// and holds a fresh read to an environment with neither.
func TestUpdateClears_Environment_DescriptionAndExternalURL(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("envclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		created := harness.Do[environments.Output](s, actionEnvironmentCreate, withParams(params, map[string]any{
			"name": e.Name("envclear"), "description": "to be removed", "external_url": "https://envclear.example.com",
		}))
		if created.Description == "" || created.ExternalURL == "" {
			e.T.Fatalf("environment create answered %+v, want a description and an external URL to remove", created)
		}
		byID := withParams(params, map[string]any{"environment_id": created.ID})

		cleared := harness.Do[environments.Output](s, actionEnvironmentUpdate, withParams(byID, map[string]any{"description": "", "external_url": ""}))
		read := harness.Do[environments.Output](s, actionEnvironmentGet, byID)
		assertCleared := func(call string, out environments.Output) {
			if out.Description != "" || out.ExternalURL != "" {
				e.T.Errorf("environment %s after the removal answered description %q, external URL %q; want neither", call, out.Description, out.ExternalURL)
			}
		}
		assertCleared("update", cleared)
		assertCleared("get", read)
	})
}

// TestUpdateClears_ProjectSnippet_Description creates a snippet with a
// description, then sends it empty and holds the answer and a fresh read to
// a snippet with none.
func TestUpdateClears_ProjectSnippet_Description(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("snipclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		created := harness.Do[snippets.Output](s, actionSnippetProjectCreate, withParams(params, map[string]any{
			"title": e.Name("snipclear"), "visibility": "private", "description": "to be removed",
			"files": []map[string]any{{"file_path": "clear.txt", "content": "clear"}},
		}))
		if created.Description == "" {
			e.T.Fatalf("snippet create answered %+v, want a description to remove", created)
		}
		snippetParams := withParams(params, map[string]any{"snippet_id": created.ID})

		cleared := harness.Do[snippets.Output](s, actionSnippetProjectUpdate, withParams(snippetParams, map[string]any{"description": ""}))
		read := harness.Do[snippets.Output](s, actionSnippetProjectGet, snippetParams)
		assertCleared := func(call string, out snippets.Output) {
			if out.Description != "" {
				e.T.Errorf("snippet %s after the removal answered description %q, want none", call, out.Description)
			}
		}
		assertCleared("update", cleared)
		assertCleared("get", read)
	})
}

// TestUpdateClears_BroadcastMessage_TargetPathAndAudience creates a broadcast
// message shown on one path to Developers, then sends the path empty and the
// audience as [], which GitLab reads as every page and every role, and holds
// a fresh read to a message with neither. The audience is the one field here
// that only an empty list clears, which client-go's options leave out of the
// body (upstream register row 122).
func TestUpdateClears_BroadcastMessage_TargetPathAndAudience(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		created := harness.Do[broadcastmessages.CreateOutput](s, actionAdminBroadcastCreate, map[string]any{
			"message": "E2E clear " + e.Name("broadcast"), "target_path": "*/clear/*", "target_access_levels": []int64{30},
		})
		if created.Message.ID == 0 || created.Message.TargetPath == "" || len(created.Message.TargetAccessLevels) != 1 {
			e.T.Fatalf("broadcast message create answered %+v, want a path and one level to remove", created.Message)
		}
		byID := map[string]any{"id": created.Message.ID}
		adminDeferDelete(e, s, "broadcast message", actionAdminBroadcastDelete, byID)

		cleared := harness.Do[broadcastmessages.UpdateOutput](s, actionAdminBroadcastUpdate, withParams(byID, map[string]any{
			"target_path": "", "target_access_levels": []int64{},
		}))
		read := harness.Do[broadcastmessages.GetOutput](s, actionAdminBroadcastGet, byID)
		assertCleared := func(call string, message broadcastmessages.MessageItem) {
			if message.TargetPath != "" || len(message.TargetAccessLevels) != 0 {
				e.T.Errorf("broadcast message %s after the removal answered path %q, levels %v; want neither",
					call, message.TargetPath, message.TargetAccessLevels)
			}
		}
		assertCleared("update", cleared.Message)
		assertCleared("get", read.Message)
	})
}
