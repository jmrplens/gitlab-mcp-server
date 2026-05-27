package projects

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestLegacyOutputWrappers_ReturnUnderlyingErrors verifies the action-spec
// compatibility wrappers propagate errors from their underlying project calls.
func TestLegacyOutputWrappers_ReturnUnderlyingErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	ctx := testutil.CancelledCtx(t)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "delete hook",
			call: func() error {
				_, err := DeleteHookOutput(ctx, client, DeleteHookInput{ProjectID: "42", HookID: 1})
				return err
			},
		},
		{
			name: "delete shared group",
			call: func() error {
				_, err := DeleteSharedGroupOutput(ctx, client, DeleteSharedGroupInput{ProjectID: "42", GroupID: 1})
				return err
			},
		},
		{
			name: "set custom header",
			call: func() error {
				_, err := SetCustomHeaderOutput(ctx, client, SetCustomHeaderInput{ProjectID: "42", HookID: 1, Key: "X-Test", Value: "value"})
				return err
			},
		},
		{
			name: "delete custom header",
			call: func() error {
				_, err := DeleteCustomHeaderOutput(ctx, client, DeleteCustomHeaderInput{ProjectID: "42", HookID: 1, Key: "X-Test"})
				return err
			},
		},
		{
			name: "set webhook URL variable",
			call: func() error {
				_, err := SetWebhookURLVariableOutput(ctx, client, SetWebhookURLVariableInput{ProjectID: "42", HookID: 1, Key: "token", Value: "secret"})
				return err
			},
		},
		{
			name: "delete webhook URL variable",
			call: func() error {
				_, err := DeleteWebhookURLVariableOutput(ctx, client, DeleteWebhookURLVariableInput{ProjectID: "42", HookID: 1, Key: "token"})
				return err
			},
		},
		{
			name: "delete fork relation",
			call: func() error {
				_, err := DeleteForkRelationOutput(ctx, client, DeleteForkRelationInput{ProjectID: "42"})
				return err
			},
		},
		{
			name: "delete approval rule",
			call: func() error {
				_, err := DeleteApprovalRuleOutput(ctx, client, DeleteApprovalRuleInput{ProjectID: "42", RuleID: 1})
				return err
			},
		},
		{
			name: "start mirroring",
			call: func() error {
				_, err := StartMirroringOutput(ctx, client, StartMirroringInput{ProjectID: "42"})
				return err
			},
		},
		{
			name: "start housekeeping",
			call: func() error {
				_, err := StartHousekeepingOutput(ctx, client, StartHousekeepingInput{ProjectID: "42"})
				return err
			},
		},
		{
			name: "delete push rule",
			call: func() error {
				_, err := DeletePushRuleOutput(ctx, client, DeletePushRuleInput{ProjectID: "42"})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("expected underlying error")
			}
		})
	}
}

// TestLegacyOutputWrappers_ReturnMessages verifies wrapper success messages.
func TestLegacyOutputWrappers_ReturnMessages(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/projects/42/hooks/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v4/projects/42/share/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /api/v4/projects/42/hooks/1/custom_headers/X-Test", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	mux.HandleFunc("DELETE /api/v4/projects/42/hooks/1/custom_headers/X-Test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /api/v4/projects/42/hooks/1/url_variables/token", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	mux.HandleFunc("DELETE /api/v4/projects/42/hooks/1/url_variables/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v4/projects/42/fork", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v4/projects/42/approval_rules/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v4/projects/42/mirror/pull", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("POST /api/v4/projects/42/housekeeping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("DELETE /api/v4/projects/42/push_rule", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	tests := []struct {
		name string
		call func() (string, error)
		want string
	}{
		{
			name: "delete hook",
			call: func() (string, error) {
				out, err := DeleteHookOutput(context.Background(), client, DeleteHookInput{ProjectID: "42", HookID: 1})
				return out.Message, err
			},
			want: "Successfully deleted webhook 1",
		},
		{
			name: "delete shared group",
			call: func() (string, error) {
				out, err := DeleteSharedGroupOutput(context.Background(), client, DeleteSharedGroupInput{ProjectID: "42", GroupID: 1})
				return out.Message, err
			},
			want: "Successfully deleted shared group 1",
		},
		{
			name: "set custom header",
			call: func() (string, error) {
				out, err := SetCustomHeaderOutput(context.Background(), client, SetCustomHeaderInput{ProjectID: "42", HookID: 1, Key: "X-Test", Value: "value"})
				return out.Message, err
			},
			want: "Custom header \"X-Test\" set",
		},
		{
			name: "delete custom header",
			call: func() (string, error) {
				out, err := DeleteCustomHeaderOutput(context.Background(), client, DeleteCustomHeaderInput{ProjectID: "42", HookID: 1, Key: "X-Test"})
				return out.Message, err
			},
			want: "Custom header \"X-Test\" deleted",
		},
		{
			name: "set webhook URL variable",
			call: func() (string, error) {
				out, err := SetWebhookURLVariableOutput(context.Background(), client, SetWebhookURLVariableInput{ProjectID: "42", HookID: 1, Key: "token", Value: "secret"})
				return out.Message, err
			},
			want: "URL variable \"token\" set",
		},
		{
			name: "delete webhook URL variable",
			call: func() (string, error) {
				out, err := DeleteWebhookURLVariableOutput(context.Background(), client, DeleteWebhookURLVariableInput{ProjectID: "42", HookID: 1, Key: "token"})
				return out.Message, err
			},
			want: "URL variable \"token\" deleted",
		},
		{
			name: "delete fork relation",
			call: func() (string, error) {
				out, err := DeleteForkRelationOutput(context.Background(), client, DeleteForkRelationInput{ProjectID: "42"})
				return out.Message, err
			},
			want: "Fork relation removed",
		},
		{
			name: "delete approval rule",
			call: func() (string, error) {
				out, err := DeleteApprovalRuleOutput(context.Background(), client, DeleteApprovalRuleInput{ProjectID: "42", RuleID: 1})
				return out.Message, err
			},
			want: "Approval rule 1 deleted",
		},
		{
			name: "start mirroring",
			call: func() (string, error) {
				out, err := StartMirroringOutput(context.Background(), client, StartMirroringInput{ProjectID: "42"})
				return out.Message, err
			},
			want: "Mirror update triggered",
		},
		{
			name: "start housekeeping",
			call: func() (string, error) {
				out, err := StartHousekeepingOutput(context.Background(), client, StartHousekeepingInput{ProjectID: "42"})
				return out.Message, err
			},
			want: "Housekeeping started",
		},
		{
			name: "delete push rule",
			call: func() (string, error) {
				out, err := DeletePushRuleOutput(context.Background(), client, DeletePushRuleInput{ProjectID: "42"})
				return out.Message, err
			},
			want: "Successfully deleted push rules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, err := tt.call()
			if err != nil {
				t.Fatalf("wrapper call error: %v", err)
			}
			if !strings.Contains(message, tt.want) {
				t.Fatalf("message = %q, want it to contain %q", message, tt.want)
			}
		})
	}
}

// TestProjectOptions_PushRuleAddGuidance verifies catalog guidance tells dynamic
// callers that creating a push rule needs a rule setting, not only project_id.
func TestProjectOptions_PushRuleAddGuidance(t *testing.T) {
	options := projectOptions("gitlab_project_add_push_rule", "push_rule")
	for _, want := range []string{"at least one rule-setting parameter", "commit_message_regex", "project_id alone"} {
		if !strings.Contains(options.Usage, want) {
			t.Fatalf("Usage = %q, want %q", options.Usage, want)
		}
	}
	if _, ok := options.ParameterGuidance["commit_message_regex"]; !ok {
		t.Fatalf("ParameterGuidance = %#v, want commit_message_regex guidance", options.ParameterGuidance)
	}
}

// TestActionSpecs_ProjectGetAndListGuidance verifies project metadata actions
// expose disambiguation and constrained sort schemas for meta/dynamic callers.
func TestActionSpecs_ProjectGetAndListGuidance(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client, false)

	getSpec := projectActionSpecByTool(t, specs, "gitlab_project_get")
	for _, want := range []string{"exact project", "group/project", "do not use search.projects"} {
		if !strings.Contains(getSpec.Usage, want) {
			t.Fatalf("get Usage = %q, want %q", getSpec.Usage, want)
		}
	}
	if guidance := getSpec.ParameterGuidance["project_id"]; guidance.SemanticRole != "scope_project" || !strings.Contains(guidance.ValueSource, "full namespace path") {
		t.Fatalf("project_id guidance = %+v, want namespace path guidance", guidance)
	}

	listSpec := projectActionSpecByTool(t, specs, "gitlab_project_list")
	if !strings.Contains(listSpec.Usage, "last_activity_at") || strings.Contains(listSpec.Usage, "last_activity_after as an order_by") && !strings.Contains(listSpec.Usage, "do not use") {
		t.Fatalf("list Usage = %q, want last_activity_at ordering guidance", listSpec.Usage)
	}
	if got := projectSchemaPropertyEnum(t, listSpec.Route.InputSchema, "order_by"); !sameProjectStringSet(got, []string{"id", "name", "path", "created_at", "updated_at", "last_activity_at"}) {
		t.Fatalf("order_by enum = %v, want accepted project ordering fields", got)
	}
	if got := projectSchemaPropertyEnum(t, listSpec.Route.InputSchema, "sort"); !sameProjectStringSet(got, []string{"asc", "desc"}) {
		t.Fatalf("sort enum = %v, want asc/desc", got)
	}
}

func projectActionSpecByTool(t *testing.T, specs []toolutil.ActionSpec, toolName string) toolutil.ActionSpec {
	t.Helper()
	for _, spec := range specs {
		if spec.IndividualTool.Name == toolName {
			return spec
		}
	}
	t.Fatalf("missing action spec for %s", toolName)
	return toolutil.ActionSpec{}
}

func projectSchemaPropertyEnum(t *testing.T, schema map[string]any, propertyName string) []string {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing: %#v", schema)
	}
	property, ok := properties[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("property %q missing: %#v", propertyName, properties)
	}
	values, ok := property["enum"].([]any)
	if !ok {
		t.Fatalf("property %q enum missing or invalid: %#v", propertyName, property["enum"])
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		text, isString := value.(string)
		if !isString {
			t.Fatalf("property %q enum contains non-string %T", propertyName, value)
		}
		out = append(out, text)
	}
	return out
}

func sameProjectStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]int, len(got))
	for _, value := range got {
		seen[value]++
	}
	for _, value := range want {
		if seen[value] == 0 {
			return false
		}
		seen[value]--
	}
	return true
}
