package toolutil

import (
	"errors"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// TestCapturedReaders_ReadWhatTheSDKDoesNotModel verifies each single-object
// reader decodes the fields its entity sends, on the key GitLab spells, and
// reports a capture nothing ran under.
func TestCapturedReaders_ReadWhatTheSDKDoesNotModel(t *testing.T) {
	_, untouched := gitlabclient.WithResponseCapture(t.Context())
	cases := []struct {
		name string
		read func(*gitlabclient.ResponseCapture) (any, error)
		body string
		want func(any) bool
	}{
		{
			name: "key",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedKey(c) },
			body: `{"id":1,"expires_at":"2026-05-05T00:00:00.000Z","last_used_at":"2026-04-07T00:00:00.000Z","usage_type":"auth"}`,
			want: func(v any) bool {
				k, _ := v.(KeyExtra)
				return k.ExpiresAt != nil && k.ExpiresAt.Year() == 2026 && k.LastUsedAt != nil && k.LastUsedAt.Month() == 4 && k.UsageType == "auth"
			},
		},
		{
			name: "runner",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedRunner(c) },
			body: `{"id":1,"created_at":"2025-05-03T00:00:00.000Z","created_by":{"id":2,"username":"owner","locked":true},"job_execution_status":"idle"}`,
			want: func(v any) bool {
				r, _ := v.(RunnerExtra)
				return r.CreatedAt != nil && r.CreatedBy != nil && r.CreatedBy.Username == "owner" && r.CreatedBy.Locked && r.JobExecutionStatus == "idle"
			},
		},
		{
			name: "lint",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedLint(c) },
			body: `{"valid":true,"jobs":[{"name":"job","stage":"test","before_script":[],"script":["echo"],"after_script":[],"tag_list":[],"only":{"refs":["branches"]},"except":null,"environment":null,"when":"on_success","allow_failure":false,"needs":null}]}`,
			want: func(v any) bool {
				l, _ := v.(LintExtra)
				return len(l.Jobs) == 1 && l.Jobs[0].Name == "job" && l.Jobs[0].Script[0] == "echo" && l.Jobs[0].Only != nil && l.Jobs[0].Except == nil && l.Jobs[0].When == "on_success"
			},
		},
		{
			name: "label",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedLabel(c) },
			body: `{"id":1,"description_html":"<p>Bug</p>"}`,
			want: func(v any) bool { l, _ := v.(LabelExtra); return l.DescriptionHTML == "<p>Bug</p>" },
		},
		{
			name: "pipeline",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedPipeline(c) },
			body: `{"id":1,"archived":true}`,
			want: func(v any) bool { p, _ := v.(PipelineExtra); return p.Archived },
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := testCase.read(gitlabclient.CapturedBody([]byte(testCase.body)))
			if err != nil || !testCase.want(got) {
				t.Errorf("read = %+v, %v; want the captured fields", got, err)
			}
			_, err = testCase.read(untouched)
			if !errors.Is(err, gitlabclient.ErrNoResponseCaptured) {
				t.Errorf("read on a capture nothing ran under = %v, want ErrNoResponseCaptured", err)
			}
		})
	}
}

// TestCapturedListReaders_HoldTheCountToTheSDKs verifies the two list
// readers here pair extras by position and refuse a count other than the
// SDK's with both numbers.
func TestCapturedListReaders_HoldTheCountToTheSDKs(t *testing.T) {
	runners, err := CapturedRunners(gitlabclient.CapturedBody([]byte(`[{"id":1,"job_execution_status":"idle"},{"id":2}]`)), 2)
	if err != nil || len(runners) != 2 || runners[0].JobExecutionStatus != "idle" || runners[1].JobExecutionStatus != "" {
		t.Errorf("CapturedRunners() = %+v, %v; want two extras in order", runners, err)
	}
	labels, err := CapturedLabels(gitlabclient.CapturedBody([]byte(`[{"id":1,"description_html":"<p>a</p>"}]`)), 1)
	if err != nil || len(labels) != 1 || labels[0].DescriptionHTML != "<p>a</p>" {
		t.Errorf("CapturedLabels() = %+v, %v; want one extra", labels, err)
	}
	_, err = CapturedRunners(gitlabclient.CapturedBody([]byte(`[{"id":1}]`)), 2)
	if err == nil || !strings.Contains(err.Error(), "holds 1 runners and the SDK decoded 2") {
		t.Errorf("CapturedRunners() with another count = %v, want the two numbers", err)
	}
	_, err = CapturedLabels(gitlabclient.CapturedBody([]byte(`[{"id":1}]`)), 3)
	if err == nil || !strings.Contains(err.Error(), "holds 1 labels and the SDK decoded 3") {
		t.Errorf("CapturedLabels() with another count = %v, want the two numbers", err)
	}
}
