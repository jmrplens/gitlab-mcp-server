package projects

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
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
		call func(t *testing.T) string
		want string
	}{
		{
			name: "delete hook",
			call: func(t *testing.T) string {
				t.Helper()
				out, err := DeleteHookOutput(context.Background(), client, DeleteHookInput{ProjectID: "42", HookID: 1})
				if err != nil {
					t.Fatalf("DeleteHookOutput() error: %v", err)
				}
				return out.Message
			},
			want: "Successfully deleted webhook 1",
		},
		{
			name: "delete shared group",
			call: func(t *testing.T) string {
				t.Helper()
				out, err := DeleteSharedGroupOutput(context.Background(), client, DeleteSharedGroupInput{ProjectID: "42", GroupID: 1})
				if err != nil {
					t.Fatalf("DeleteSharedGroupOutput() error: %v", err)
				}
				return out.Message
			},
			want: "Successfully deleted shared group 1",
		},
		{
			name: "set custom header",
			call: func(t *testing.T) string {
				t.Helper()
				out, err := SetCustomHeaderOutput(context.Background(), client, SetCustomHeaderInput{ProjectID: "42", HookID: 1, Key: "X-Test", Value: "value"})
				if err != nil {
					t.Fatalf("SetCustomHeaderOutput() error: %v", err)
				}
				return out.Message
			},
			want: "Custom header \"X-Test\" set",
		},
		{
			name: "delete custom header",
			call: func(t *testing.T) string {
				t.Helper()
				out, err := DeleteCustomHeaderOutput(context.Background(), client, DeleteCustomHeaderInput{ProjectID: "42", HookID: 1, Key: "X-Test"})
				if err != nil {
					t.Fatalf("DeleteCustomHeaderOutput() error: %v", err)
				}
				return out.Message
			},
			want: "Custom header \"X-Test\" deleted",
		},
		{
			name: "set webhook URL variable",
			call: func(t *testing.T) string {
				t.Helper()
				out, err := SetWebhookURLVariableOutput(context.Background(), client, SetWebhookURLVariableInput{ProjectID: "42", HookID: 1, Key: "token", Value: "secret"})
				if err != nil {
					t.Fatalf("SetWebhookURLVariableOutput() error: %v", err)
				}
				return out.Message
			},
			want: "URL variable \"token\" set",
		},
		{
			name: "delete webhook URL variable",
			call: func(t *testing.T) string {
				t.Helper()
				out, err := DeleteWebhookURLVariableOutput(context.Background(), client, DeleteWebhookURLVariableInput{ProjectID: "42", HookID: 1, Key: "token"})
				if err != nil {
					t.Fatalf("DeleteWebhookURLVariableOutput() error: %v", err)
				}
				return out.Message
			},
			want: "URL variable \"token\" deleted",
		},
		{
			name: "delete fork relation",
			call: func(t *testing.T) string {
				t.Helper()
				out, err := DeleteForkRelationOutput(context.Background(), client, DeleteForkRelationInput{ProjectID: "42"})
				if err != nil {
					t.Fatalf("DeleteForkRelationOutput() error: %v", err)
				}
				return out.Message
			},
			want: "Fork relation removed",
		},
		{
			name: "delete approval rule",
			call: func(t *testing.T) string {
				t.Helper()
				out, err := DeleteApprovalRuleOutput(context.Background(), client, DeleteApprovalRuleInput{ProjectID: "42", RuleID: 1})
				if err != nil {
					t.Fatalf("DeleteApprovalRuleOutput() error: %v", err)
				}
				return out.Message
			},
			want: "Approval rule 1 deleted",
		},
		{
			name: "start mirroring",
			call: func(t *testing.T) string {
				t.Helper()
				out, err := StartMirroringOutput(context.Background(), client, StartMirroringInput{ProjectID: "42"})
				if err != nil {
					t.Fatalf("StartMirroringOutput() error: %v", err)
				}
				return out.Message
			},
			want: "Mirror update triggered",
		},
		{
			name: "start housekeeping",
			call: func(t *testing.T) string {
				t.Helper()
				out, err := StartHousekeepingOutput(context.Background(), client, StartHousekeepingInput{ProjectID: "42"})
				if err != nil {
					t.Fatalf("StartHousekeepingOutput() error: %v", err)
				}
				return out.Message
			},
			want: "Housekeeping started",
		},
		{
			name: "delete push rule",
			call: func(t *testing.T) string {
				t.Helper()
				out, err := DeletePushRuleOutput(context.Background(), client, DeletePushRuleInput{ProjectID: "42"})
				if err != nil {
					t.Fatalf("DeletePushRuleOutput() error: %v", err)
				}
				return out.Message
			},
			want: "Successfully deleted push rules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if message := tt.call(t); !strings.Contains(message, tt.want) {
				t.Fatalf("message = %q, want it to contain %q", message, tt.want)
			}
		})
	}
}
