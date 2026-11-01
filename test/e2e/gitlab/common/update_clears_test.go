// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// update_clears_test.go holds the update actions to sending the value that
// empties a field. Each scenario sets a field through the server, sends it
// empty, and reads the object back, once per surface. Every one of these
// fields used to be dropped for being empty, so the update reported success
// with the old value or GitLab answered that nothing was sent (issue 1306).

package common

import (
	"context"
	"maps"
	"slices"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/appearance"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/broadcastmessages"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/civariables"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelinetriggers"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/protectedpackages"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/runners"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/snippets"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/systemhooks"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
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

// appearanceClearedFields are the texts and colors of the instance appearance
// an update empties, each with the value the scenario sets first.
var appearanceClearedFields = map[string]string{
	"site_name": "E2E site", "title": "E2E title", "description": "E2E description",
	"pwa_name": "E2E app", "pwa_short_name": "E2E", "pwa_description": "E2E app description",
	"header_message": "E2E header", "footer_message": "E2E footer",
	"message_background_color": "#123456", "message_font_color": "#654321",
	"member_guidelines": "E2E members", "new_project_guidelines": "E2E projects", "profile_image_guidelines": "E2E images",
}

// appearanceValues returns the value of each cleared field in item, by the
// parameter that sets it.
func appearanceValues(item appearance.Item) map[string]string {
	return map[string]string{
		"site_name": item.SiteName, "title": item.Title, "description": item.Description,
		"pwa_name": item.PWAName, "pwa_short_name": item.PWAShortName, "pwa_description": item.PWADescription,
		"header_message": item.HeaderMessage, "footer_message": item.FooterMessage,
		"message_background_color": item.MessageBackgroundColor, "message_font_color": item.MessageFontColor,
		"member_guidelines": item.MemberGuidelines, "new_project_guidelines": item.NewProjectGuidelines,
		"profile_image_guidelines": item.ProfileImageGuidelines,
	}
}

// TestUpdateClears_Appearance_TextsAndColors sets every text and both
// message colors of the instance appearance, then sends each empty in one
// update and holds the answer and a fresh read to an appearance with none of
// them. Appearance validates a color only when one is given
// (app/models/appearance.rb), so an empty one is stored as such. What the run
// found is put back afterwards.
func TestUpdateClears_Appearance_TextsAndColors(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		before := appearanceValues(harness.Do[appearance.GetOutput](s, actionAdminAppearanceGet, nil).Appearance)
		e.Defer("appearance texts", func(ctx context.Context) error {
			restore := map[string]any{}
			for key, value := range before {
				restore[key] = value
			}
			_, err := harness.Undo[appearance.UpdateOutput](ctx, s, actionAdminAppearanceUpd, restore)
			return err
		})

		set, empty, none := map[string]any{}, map[string]any{}, map[string]string{}
		for key, value := range appearanceClearedFields {
			set[key], empty[key], none[key] = value, "", ""
		}
		stored := appearanceValues(harness.Do[appearance.UpdateOutput](s, actionAdminAppearanceUpd, set).Appearance)
		if !maps.Equal(stored, appearanceClearedFields) {
			e.T.Fatalf("appearance update answered %v, want %v before the removal", stored, appearanceClearedFields)
		}

		cleared := harness.Do[appearance.UpdateOutput](s, actionAdminAppearanceUpd, empty)
		read := harness.Do[appearance.GetOutput](s, actionAdminAppearanceGet, nil)
		assertCleared := func(call string, item appearance.Item) {
			if values := appearanceValues(item); !maps.Equal(values, none) {
				e.T.Errorf("appearance %s after the removal answered %v, want every one empty", call, values)
			}
		}
		assertCleared("update", cleared.Appearance)
		assertCleared("get", read.Appearance)
	})
}

// TestUpdateClears_BroadcastMessage_ColorAndFont creates a broadcast message
// with a background and a font color, then sends both empty and holds the
// answer and a fresh read to a message with neither: the model validates a
// color only when one is given (app/models/system/broadcast_message.rb).
func TestUpdateClears_BroadcastMessage_ColorAndFont(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		created := harness.Do[broadcastmessages.CreateOutput](s, actionAdminBroadcastCreate, map[string]any{
			"message": "E2E colors " + e.Name("broadcast"), "color": "#123456", "font": "#654321",
		})
		if created.Message.ID == 0 || created.Message.Color == "" || created.Message.Font == "" {
			e.T.Fatalf("broadcast message create answered %+v, want both colors to remove", created.Message)
		}
		byID := map[string]any{"id": created.Message.ID}
		adminDeferDelete(e, s, "broadcast message", actionAdminBroadcastDelete, byID)

		cleared := harness.Do[broadcastmessages.UpdateOutput](s, actionAdminBroadcastUpdate, withParams(byID, map[string]any{"color": "", "font": ""}))
		read := harness.Do[broadcastmessages.GetOutput](s, actionAdminBroadcastGet, byID)
		assertCleared := func(call string, message broadcastmessages.MessageItem) {
			if message.Color != "" || message.Font != "" {
				e.T.Errorf("broadcast message %s after the removal answered color %q, font %q; want neither", call, message.Color, message.Font)
			}
		}
		assertCleared("update", cleared.Message)
		assertCleared("get", read.Message)
	})
}

// TestUpdateClears_ProjectHook_Name adds a named webhook to a project, then
// sends the name empty and holds the answer and a fresh read to a hook with
// none: a hook's name is only held to a length (app/models/concerns/web_hooks/hook.rb).
func TestUpdateClears_ProjectHook_Name(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("hookclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		added := harness.Do[projects.HookOutput](s, actionProjectHookAdd, withParams(params, map[string]any{
			"url": "https://example.com/hook-clear", "push_events": true, "name": e.Name("hookclear"),
		}))
		if added.ID == 0 || added.Name == "" {
			e.T.Fatalf("hook_add answered %+v, want a named hook to remove the name of", added)
		}
		hook := withParams(params, map[string]any{"hook_id": added.ID})

		cleared := harness.Do[projects.HookOutput](s, actionProjectHookEdit, withParams(hook, map[string]any{"name": ""}))
		read := harness.Do[projects.HookOutput](s, actionProjectHookGet, hook)
		if cleared.Name != "" || read.Name != "" {
			e.T.Errorf("after the removal the hook edit answered name %q and the read %q, want none", cleared.Name, read.Name)
		}
		harness.DoVoid(s, actionProjectHookDelete, hook)
	})
}

// TestUpdateClears_SystemHook_Name registers a named system hook, then sends
// the name empty and holds the answer and a fresh read to a hook with none.
func TestUpdateClears_SystemHook_Name(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		added := harness.Do[systemhooks.AddOutput](s, actionAdminSystemHookAdd, map[string]any{
			"url": "https://e2e-test.example.com/hook-clear", "name": e.Name("syshookclear"),
		})
		if added.Hook.ID == 0 || added.Hook.Name == "" {
			e.T.Fatalf("system_hook_add answered %+v, want a named hook to remove the name of", added.Hook)
		}
		byID := map[string]any{"id": added.Hook.ID}
		adminDeferDelete(e, s, "system hook", actionAdminSystemHookDelete, byID)

		cleared := harness.Do[systemhooks.EditOutput](s, actionAdminSystemHookEdit, withParams(byID, map[string]any{"name": ""}))
		read := harness.Do[systemhooks.GetOutput](s, actionAdminSystemHookGet, byID)
		if cleared.Hook.Name != "" || read.Hook.Name != "" {
			e.T.Errorf("after the removal the system hook edit answered name %q and the read %q, want none", cleared.Hook.Name, read.Hook.Name)
		}
	})
}

// TestUpdateClears_Runner_DescriptionAndMaintenanceNote gives a project
// runner a maintenance note, then sends it and the description empty and
// holds the answer and a fresh read to a runner with neither.
func TestUpdateClears_Runner_DescriptionAndMaintenanceNote(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("runnerclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		runner := fixture.NewProjectRunner(e, project)
		params := map[string]any{"runner_id": runner.ID}

		set := harness.Do[runners.DetailsOutput](s, actionRunnerUpdate, withParams(params, map[string]any{"maintenance_note": "to be removed"}))
		if set.Description == "" || set.MaintenanceNote == "" {
			e.T.Fatalf("runner update answered %+v, want a description and a maintenance note to remove", set)
		}
		cleared := harness.Do[runners.DetailsOutput](s, actionRunnerUpdate, withParams(params, map[string]any{"description": "", "maintenance_note": ""}))
		read := harness.Do[runners.DetailsOutput](s, actionRunnerGet, params)
		assertCleared := func(call string, out runners.DetailsOutput) {
			if out.Description != "" || out.MaintenanceNote != "" {
				e.T.Errorf("runner %s after the removal answered description %q, maintenance note %q; want neither", call, out.Description, out.MaintenanceNote)
			}
		}
		assertCleared("update", cleared)
		assertCleared("get", read)
	})
}

// TestUpdateClears_PipelineTrigger_Description creates a trigger with a
// description, then sends it empty and holds the answer and a fresh read to
// a trigger with none.
func TestUpdateClears_PipelineTrigger_Description(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("trigclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		created := harness.Do[pipelinetriggers.Output](s, actionPipelineTriggerCreate, withParams(params, map[string]any{"description": e.Name("trigclear")}))
		if created.ID == 0 || created.Description == "" {
			e.T.Fatalf("trigger create answered %+v, want a description to remove", created)
		}
		byID := withParams(params, map[string]any{"trigger_id": created.ID})

		cleared := harness.Do[pipelinetriggers.Output](s, actionPipelineTriggerUpdate, withParams(byID, map[string]any{"description": ""}))
		read := harness.Do[pipelinetriggers.Output](s, actionPipelineTriggerGet, byID)
		if cleared.Description != "" || read.Description != "" {
			e.T.Errorf("after the removal the trigger update answered description %q and the read %q, want none", cleared.Description, read.Description)
		}
		harness.DoVoid(s, actionPipelineTriggerDelete, byID)
	})
}

// TestUpdateClears_User_Organization gives a user an organization, then
// sends it empty and holds the answer and a fresh read to a user with none
// ("Empty string or nil clears the field", lib/api/users.rb).
func TestUpdateClears_User_Organization(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		user := fixture.NewUser(e, "orgclear")
		params := map[string]any{"user_id": user.ID}

		set := harness.Do[users.Output](s, actionUserModify, withParams(params, map[string]any{"organization": "E2E organization"}))
		if set.Organization == "" {
			e.T.Fatalf("user modify answered %+v, want an organization to remove", set)
		}
		cleared := harness.Do[users.Output](s, actionUserModify, withParams(params, map[string]any{"organization": ""}))
		read := harness.Do[users.Output](s, actionUserGet, params)
		if cleared.Organization != "" || read.Organization != "" {
			e.T.Errorf("after the removal the user modify answered organization %q and the read %q, want none", cleared.Organization, read.Organization)
		}
	})
}

// TestUpdateClears_Release_NameAndNotes creates a release with a title and
// notes, then sends both empty and holds the answer and a fresh read to a
// release with neither: Release#name falls back to the tag only for a name
// never stored (app/models/release.rb), and an empty one is stored.
func TestUpdateClears_Release_NameAndNotes(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("relclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		tag := createTag(e, s, project, e.Name("v0.1.0"))
		params := map[string]any{"project_id": project.IDParam(), "tag_name": tag.Name}

		created := harness.Do[releases.Output](s, actionReleaseCreate, withParams(params, map[string]any{"name": "E2E release", "description": "to be removed"}))
		if created.Name == "" || created.Description == "" {
			e.T.Fatalf("release create answered %+v, want a title and notes to remove", created)
		}
		cleared := harness.Do[releases.Output](s, actionReleaseUpdate, withParams(params, map[string]any{"name": "", "description": ""}))
		read := harness.Do[releases.Output](s, actionReleaseGet, params)
		assertCleared := func(call string, out releases.Output) {
			if out.Name != "" || out.Description != "" {
				e.T.Errorf("release %s after the removal answered name %q, notes %q; want neither", call, out.Name, out.Description)
			}
		}
		assertCleared("update", cleared)
		assertCleared("get", read)
	})
}

// TestUpdateClears_ProjectMember_Expiry adds a member to a project with an
// expiry, then sends the expiry empty, which goes out as null, beside the
// access level the edit requires, and holds the answer and a fresh read to a
// membership that never expires.
func TestUpdateClears_ProjectMember_Expiry(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("mexpclear"))
		user := fixture.NewUser(e, "mexpclear")
		member := map[string]any{"project_id": project.IDParam(), "user_id": user.ID}

		added := harness.Do[members.Output](s, actionProjectMemberAdd, withParams(member, map[string]any{
			"access_level": int64(gl.DeveloperPermissions), "expires_at": "2030-12-31",
		}))
		if added.ExpiresAt == "" {
			e.T.Fatalf("member_add answered %+v, want an expiry to remove", added)
		}
		cleared := harness.Do[members.Output](s, actionProjectMemberEdit, withParams(member, map[string]any{
			"access_level": int64(gl.DeveloperPermissions), "expires_at": "",
		}))
		read := harness.Do[members.Output](s, actionProjectMemberGet, member)
		if cleared.ExpiresAt != "" || read.ExpiresAt != "" {
			e.T.Errorf("after the removal the member edit answered expiry %q and the read %q, want none", cleared.ExpiresAt, read.ExpiresAt)
		}
	})
}

// TestUpdateClears_GroupMember_Expiry adds a member to a group with an
// expiry, then sends the expiry empty, which goes out as null, beside the
// access level the edit requires, and holds the answer and a fresh read to a
// membership that never expires.
func TestUpdateClears_GroupMember_Expiry(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("gexpclear"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		user := fixture.NewUser(e, "gexpclear")
		member := map[string]any{"group_id": group.IDParam(), "user_id": user.ID}

		added := harness.Do[groupmembers.Output](s, actionGroupMemberAdd, withParams(member, map[string]any{
			"access_level": int64(gl.DeveloperPermissions), "expires_at": "2030-12-31",
		}))
		if added.ExpiresAt == "" {
			e.T.Fatalf("group_member_add answered %+v, want an expiry to remove", added)
		}
		cleared := harness.Do[groupmembers.Output](s, actionGroupMemberEdit, withParams(member, map[string]any{
			"access_level": int64(gl.DeveloperPermissions), "expires_at": "",
		}))
		read := harness.Do[groupmembers.Output](s, actionGroupMemberGet, member)
		if cleared.ExpiresAt != "" || read.ExpiresAt != "" {
			e.T.Errorf("after the removal the group member edit answered expiry %q and the read %q, want none", cleared.ExpiresAt, read.ExpiresAt)
		}
	})
}

// packageDeleteLevelFlag is the flag behind a package protection rule's
// minimum access level for delete.
const packageDeleteLevelFlag = "packages_protected_packages_delete"

// TestUpdateClears_PackageProtectionRule_PushLevel creates a rule that
// protects pushes, then protects deletes in its place, sending the push
// level empty, which goes out as null, and holds the answer and a fresh
// listing to a rule with no push level. A rule keeps at least one of the
// two (app/models/packages/protection/rule.rb), so the delete level is set
// in the same call, and the route drops a delete level unless the
// packages_protected_packages_delete flag is on (lib/api/project_packages_protection_rules.rb),
// which GitLab ships off: with it off, the rule would be left with neither
// and refused, so the scenario turns it on for its run.
func TestUpdateClears_PackageProtectionRule_PushLevel(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	fixture.PinFeature(e, packageDeleteLevelFlag, true)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("pkgclear"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		pattern := "e2e-clear-" + string(surface) + "-*"

		created := harness.Do[protectedpackages.Output](s, actionPackageProtectionRuleCreate, withParams(params, map[string]any{
			"package_name_pattern": pattern, "package_type": "generic", "minimum_access_level_for_push": "maintainer",
		}))
		if created.ID == 0 || created.MinimumAccessLevelForPush == "" {
			e.T.Fatalf("package protection rule create answered %+v, want a push level to remove", created)
		}
		byRule := withParams(params, map[string]any{"rule_id": created.ID})

		cleared := harness.Do[protectedpackages.Output](s, actionPackageProtectionRuleUpdate, withParams(byRule, map[string]any{
			"package_name_pattern": pattern, "package_type": "generic",
			"minimum_access_level_for_push": "", "minimum_access_level_for_delete": "owner",
		}))
		if cleared.MinimumAccessLevelForPush != "" || cleared.MinimumAccessLevelForDelete != "owner" {
			e.T.Errorf("package protection rule update answered push %q, delete %q; want none and owner",
				cleared.MinimumAccessLevelForPush, cleared.MinimumAccessLevelForDelete)
		}
		listed := harness.Do[protectedpackages.ListOutput](s, actionPackageProtectionRuleList, params)
		at := slices.IndexFunc(listed.Rules, func(rule protectedpackages.Output) bool { return rule.ID == created.ID })
		if at < 0 {
			e.T.Fatalf("the listing holds no rule %d after the update", created.ID)
		}
		if rule := listed.Rules[at]; rule.MinimumAccessLevelForPush != "" || rule.MinimumAccessLevelForDelete != "owner" {
			e.T.Errorf("the listing holds rule %d with push %q, delete %q; want none and owner",
				rule.ID, rule.MinimumAccessLevelForPush, rule.MinimumAccessLevelForDelete)
		}
		harness.DoVoid(s, actionPackageProtectionRuleDelete, byRule)
	})
}
