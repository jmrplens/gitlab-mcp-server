package toolutil

import (
	"errors"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
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

// TestCapturedTokenReaders_ReadEachTokenEntity verifies the three token
// readers: the fields every personal access token entity sends, the
// impersonation token's own three, and the resource token's two, each read
// off the key GitLab spells.
func TestCapturedTokenReaders_ReadEachTokenEntity(t *testing.T) {
	body := `{"id":1,"granular":true,"last_used_ips":["192.0.2.10"],` +
		`"granular_scopes":[{"access":"personal_projects","permissions":["read_job"],"project_id":3,"group_id":0}],` +
		`"impersonation":true,"description":"acting as","user_id":9,"resource_type":"project","resource_id":77}`

	plain, err := CapturedToken(gitlabclient.CapturedBody([]byte(body)))
	if err != nil || !plain.Granular || len(plain.GranularScopes) != 1 || plain.GranularScopes[0].Access != "personal_projects" ||
		plain.GranularScopes[0].ProjectID != 3 || len(plain.LastUsedIPs) != 1 {
		t.Errorf("CapturedToken() = %+v, %v; want the three shared fields", plain, err)
	}

	impersonation, err := CapturedImpersonationToken(gitlabclient.CapturedBody([]byte(body)))
	if err != nil || !impersonation.Impersonation || impersonation.Description != "acting as" || impersonation.UserID != 9 || !impersonation.Granular {
		t.Errorf("CapturedImpersonationToken() = %+v, %v; want its own three beside the shared ones", impersonation, err)
	}

	resource, err := CapturedResourceToken(gitlabclient.CapturedBody([]byte(body)))
	if err != nil || resource.ResourceType != "project" || resource.ResourceID != 77 || !resource.Granular {
		t.Errorf("CapturedResourceToken() = %+v, %v; want the resource pair beside the shared ones", resource, err)
	}
}

// TestCapturedTokenReaders_ACaptureNothingRanUnder verifies each of the three
// token readers reports the empty capture rather than answering with a zero
// token.
func TestCapturedTokenReaders_ACaptureNothingRanUnder(t *testing.T) {
	_, untouched := gitlabclient.WithResponseCapture(t.Context())
	reads := []struct {
		name string
		call func() error
	}{
		{name: "token", call: func() error { _, readErr := CapturedToken(untouched); return readErr }},
		{name: "impersonation token", call: func() error { _, readErr := CapturedImpersonationToken(untouched); return readErr }},
		{name: "resource access token", call: func() error { _, readErr := CapturedResourceToken(untouched); return readErr }},
	}
	for _, read := range reads {
		t.Run(read.name, func(t *testing.T) {
			if err := read.call(); !errors.Is(err, gitlabclient.ErrNoResponseCaptured) {
				t.Errorf("read = %v, want ErrNoResponseCaptured", err)
			}
		})
	}
}

// TestCapturedTokenListReaders_HoldTheCountToTheSDKs verifies the three token
// list readers pair by position and refuse a count other than the SDK's with
// both numbers.
func TestCapturedTokenListReaders_HoldTheCountToTheSDKs(t *testing.T) {
	cases := []struct {
		name string
		read func(*gitlabclient.ResponseCapture, int) (int, error)
		want string
	}{
		{
			name: "tokens",
			read: func(c *gitlabclient.ResponseCapture, n int) (int, error) {
				got, err := CapturedTokens(c, n)
				return len(got), err
			},
			want: "holds 2 tokens and the SDK decoded 1",
		},
		{
			name: "impersonation tokens",
			read: func(c *gitlabclient.ResponseCapture, n int) (int, error) {
				got, err := CapturedImpersonationTokens(c, n)
				return len(got), err
			},
			want: "holds 2 impersonation tokens and the SDK decoded 1",
		},
		{
			name: "resource access tokens",
			read: func(c *gitlabclient.ResponseCapture, n int) (int, error) {
				got, err := CapturedResourceTokens(c, n)
				return len(got), err
			},
			want: "holds 2 resource access tokens and the SDK decoded 1",
		},
	}
	body := []byte(`[{"id":1,"granular":true},{"id":2}]`)
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			count, err := testCase.read(gitlabclient.CapturedBody(body), 2)
			if err != nil || count != 2 {
				t.Errorf("read of two = %d, %v; want two extras", count, err)
			}
			_, err = testCase.read(gitlabclient.CapturedBody(body), 1)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("read with another count = %v, want %q", err, testCase.want)
			}
		})
	}
}

// capturedReaderCase is one reader in the table below: the body GitLab answers
// with, and what the reader has to have taken out of it.
type capturedReaderCase struct {
	name string
	read func(*gitlabclient.ResponseCapture) (any, error)
	body string
	want func(any) bool
}

// readTheSentPackage reports whether the package reader decoded every key of
// the fixture, the nested version with its tag and its pipeline included. It
// sits out here because a predicate reaching three objects deep is the one
// shape the table cannot hold as a literal and stay readable.
func readTheSentPackage(v any) bool {
	e, _ := v.([]PackageExtra)
	if len(e) != 1 || e[0].CreatorID != 57 || e[0].ConanPackageName != "my-pkg" ||
		e[0].ProjectID != 42 || e[0].ProjectPath != "group/project" || len(e[0].Versions) != 1 {
		return false
	}
	version := e[0].Versions[0]
	return version.Version == "0.9.0" && len(version.Tags) == 1 && version.Tags[0].Name == "stable" &&
		version.Pipeline != nil && version.Pipeline.IID == 4 &&
		version.Pipeline.User != nil && version.Pipeline.User.Username == "alice"
}

// readTheSentNamespace reports whether the namespace reader decoded all eight
// keys, which is more than a table cell holds comfortably.
func readTheSentNamespace(v any) bool {
	e, _ := v.(NamespaceExtra)
	return e.ProjectsCount == 12 && e.RootRepositorySize == 34567 &&
		e.SharedRunnersMinutesLimit != nil && *e.SharedRunnersMinutesLimit == 400 &&
		e.ExtraSharedRunnersMinutesLimit != nil && e.AdditionalPurchasedStorageSize != nil &&
		e.AdditionalPurchasedStorageEndsOn == "2027-03-31" &&
		e.MaxSeatsUsedChangedAt != nil && e.EndDate == "2027-01-31"
}

// tailReaderCases is the table itself, out here rather than inside the test, so
// that the test is the loop it runs and nothing else.
//
// Measured rather than assumed, because the obvious question is whether moving
// the table anywhere helps at all. Inline, the test scored 26 on gocognit, 37
// on gocyclo and 13 on maintidx, and needed three suppressions to build. Out
// here gocognit is genuinely gone, since the nesting it counts was the test's;
// the other two follow the closures and land on this function at 33 and 14. So
// the trade is two suppressions on a function that is nothing but data, against
// three on the function that does the work, and the test now reads as the loop
// it is.
//
// The score is the number of readers. A reader hiding a branch would be a
// finding rather than a rounding error, and none of them has one.
//
//nolint:gocyclo,maintidx // one predicate per entity in a table, so the score is the entity count.
func tailReaderCases() []capturedReaderCase {
	// first reads one element out of a list reader, so a shape with no
	// single-object reader is still held to its fields here.
	first := func(read func(*gitlabclient.ResponseCapture, int) (any, error)) func(*gitlabclient.ResponseCapture) (any, error) {
		return func(c *gitlabclient.ResponseCapture) (any, error) { return read(c, 1) }
	}
	return []capturedReaderCase{
		{
			name: "topic",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedTopic(c) },
			body: `{"id":1,"organization_id":7}`,
			want: func(v any) bool { e, _ := v.(TopicExtra); return e.OrganizationID == 7 },
		},
		{
			name: "appearance",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedAppearance(c) },
			body: `{"title":"GitLab","site_name":"Example GitLab"}`,
			want: func(v any) bool { e, _ := v.(AppearanceExtra); return e.SiteName == "Example GitLab" },
		},
		{
			name: "broadcast message",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedBroadcastMessage(c) },
			body: `{"id":1,"color":"#e75e40"}`,
			want: func(v any) bool { e, _ := v.(BroadcastMessageExtra); return e.Color == "#e75e40" },
		},
		{
			name: "cluster agent",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedClusterAgent(c) },
			body: `{"id":1,"is_receptive":true}`,
			want: func(v any) bool { e, _ := v.(ClusterAgentExtra); return e.IsReceptive },
		},
		{
			name: "license template",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedLicenseTemplate(c) },
			body: `{"key":"mit","popular":true}`,
			want: func(v any) bool { e, _ := v.(LicenseTemplateExtra); return e.Popular },
		},
		{
			name: "secure file",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedSecureFile(c) },
			body: `{"id":1,"file_extension":"jks"}`,
			want: func(v any) bool { e, _ := v.(SecureFileExtra); return e.FileExtension == "jks" },
		},
		{
			name: "pipeline trigger",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedPipelineTrigger(c) },
			body: `{"id":1,"expires_at":"2026-05-05T00:00:00.000Z"}`,
			want: func(v any) bool {
				e, _ := v.(PipelineTriggerExtra)
				return e.ExpiresAt != nil && e.ExpiresAt.Year() == 2026
			},
		},
		{
			name: "protected branch",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedProtectedBranch(c) },
			body: `{"id":1,"inherited":true}`,
			want: func(v any) bool { e, _ := v.(ProtectedBranchExtra); return e.Inherited },
		},
		{
			name: "merge request version",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedMergeRequestDiff(c) },
			body: `{"id":1,"patch_id_sha":"abc123"}`,
			want: func(v any) bool { e, _ := v.(MergeRequestDiffExtra); return e.PatchIDSHA == "abc123" },
		},
		{
			name: "SCIM identity",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedSCIMIdentity(c) },
			body: `{"user_id":1,"extern_uid":"uid-1"}`,
			want: func(v any) bool { e, _ := v.(SCIMIdentityExtra); return e.ExternUID == "uid-1" },
		},
		{
			name: "feature",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedFeature(c) },
			body: `{"name":"flag","definition":{"feature_issue_url":"https://example/1","intended_to_rollout_by":"17.0"}}`,
			want: func(v any) bool {
				e, _ := v.(FeatureExtra)
				return e.Definition.FeatureIssueURL == "https://example/1" && e.Definition.IntendedToRolloutBy == "17.0"
			},
		},
		{
			name: "feature flag user list",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedFeatureFlagUserList(c) },
			body: `{"id":1,"path":"/p","edit_path":"/e"}`,
			want: func(v any) bool {
				e, _ := v.(FeatureFlagUserListExtra)
				return e.Path == "/p" && e.EditPath == "/e"
			},
		},
		{
			name: "commit comment",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedCommitComment(c) },
			body: `{"note":"hi","created_at":"2026-04-07T00:00:00.000Z"}`,
			want: func(v any) bool {
				e, _ := v.(CommitCommentExtra)
				return e.CreatedAt != nil && e.CreatedAt.Month() == 4
			},
		},
		{
			name: "deployment",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedDeployment(c) },
			body: `{"id":1,"pending_approval_count":2,"approvals":[{"status":"approved","comment":"ship it","user":{"id":3,"username":"rev"}}],` +
				`"approval_summary":{"rules":[{"id":5,"required_approvals":2,"access_level":40}]}}`,
			want: func(v any) bool {
				e, _ := v.(DeploymentExtra)
				return e.PendingApprovalCount == 2 && len(e.Approvals) == 1 && e.Approvals[0].Status == "approved" &&
					e.Approvals[0].User != nil && e.Approvals[0].User.Username == "rev" &&
					e.ApprovalSummary != nil && len(e.ApprovalSummary.Rules) == 1 && e.ApprovalSummary.Rules[0].RequiredApprovals == 2
			},
		},
		{
			name: "pages domain",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedPagesDomain(c) },
			body: `{"domain":"example.com","certificate_expiration":{"expired":false,"expiration":"2026-09-01T00:00:00.000Z"}}`,
			want: func(v any) bool {
				e, _ := v.(PagesDomainExtra)
				return e.CertificateExpiration != nil && !e.CertificateExpiration.Expired &&
					e.CertificateExpiration.Expiration != nil && e.CertificateExpiration.Expiration.Year() == 2026
			},
		},
		{
			name: "resource state event",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedResourceStateEvent(c) },
			body: `{"id":1,"source_commit":"abc123","source_merge_request_id":7}`,
			want: func(v any) bool {
				e, _ := v.(ResourceStateEventExtra)
				return e.SourceCommit == "abc123" && e.SourceMergeRequestID == 7
			},
		},
		{
			name: "resource milestone event",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedResourceMilestoneEvent(c) },
			body: `{"id":1,"state":"opened"}`,
			want: func(v any) bool { e, _ := v.(ResourceMilestoneEventExtra); return e.State == "opened" },
		},
		{
			name: "todo",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedTodo(c) },
			body: `{"id":1,"updated_at":"2026-04-07T00:00:00.000Z","group":{"id":4,"name":"Acme","path":"acme","kind":"group","full_path":"acme"}}`,
			want: func(v any) bool {
				e, _ := v.(TodoExtra)
				return e.UpdatedAt != nil && e.Group != nil && e.Group.FullPath == "acme" && e.Group.Kind == "group"
			},
		},
		{
			name: "board",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedBoard(c) },
			body: `{"id":1,"group":{"id":4,"name":"Acme","web_url":"https://example/groups/acme"}}`,
			want: func(v any) bool {
				e, _ := v.(BoardExtra)
				return e.Group != nil && e.Group.ID == 4 && e.Group.WebURL == "https://example/groups/acme"
			},
		},
		{
			name: "milestone",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedMilestone(c) },
			body: `{"id":1,"web_url":"https://example/m/1","project_id":9}`,
			want: func(v any) bool {
				e, _ := v.(MilestoneExtra)
				return e.WebURL == "https://example/m/1" && e.ProjectID == 9
			},
		},
		{
			name: "board list",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedBoardList(c) },
			body: `{"id":1,"limit_metric":"all_metrics"}`,
			want: func(v any) bool { e, _ := v.(BoardListExtra); return e.LimitMetric == "all_metrics" },
		},
		{
			name: "CI variable",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedCIVariable(c) },
			body: `{"key":"TOKEN","environment_scope":"production","hidden":true}`,
			want: func(v any) bool {
				e, _ := v.(CIVariableExtra)
				return e.EnvironmentScope == "production" && e.Hidden
			},
		},
		{
			name: "registry repository",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedRegistryRepository(c) },
			body: `{"id":1,"size":1024,"delete_api_path":"/api/v4/registry/repositories/1"}`,
			want: func(v any) bool {
				e, _ := v.(RegistryRepositoryExtra)
				return e.Size == 1024 && e.DeleteAPIPath == "/api/v4/registry/repositories/1"
			},
		},
		{
			name: "wiki page",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedWiki(c) },
			body: `{"slug":"home","wiki_page_meta_id":11,"front_matter":{"title":"Home"}}`,
			want: func(v any) bool {
				e, _ := v.(WikiExtra)
				return e.WikiPageMetaID == 11 && e.FrontMatter["title"] == "Home"
			},
		},
		{
			name: "storage move",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedStorageMove(c) },
			body: `{"id":1,"error_message":"disk full"}`,
			want: func(v any) bool { e, _ := v.(StorageMoveExtra); return e.ErrorMessage == "disk full" },
		},
		{
			name: "service account",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedServiceAccount(c) },
			body: `{"id":1,"public_email":"svc@example","unconfirmed_email":"new@example"}`,
			want: func(v any) bool {
				e, _ := v.(ServiceAccountExtra)
				return e.PublicEmail == "svc@example" && e.UnconfirmedEmail == "new@example"
			},
		},
		{
			name: "runner manager",
			read: first(func(c *gitlabclient.ResponseCapture, n int) (any, error) { return CapturedRunnerManagers(c, n) }),
			body: `[{"id":1,"job_execution_status":"idle"}]`,
			want: func(v any) bool {
				e, _ := v.([]RunnerManagerExtra)
				return len(e) == 1 && e[0].JobExecutionStatus == "idle"
			},
		},
		{
			name: "feature definition",
			read: first(func(c *gitlabclient.ResponseCapture, n int) (any, error) { return CapturedFeatureDefinitions(c, n) }),
			body: `[{"name":"flag","feature_issue_url":"https://example/2","intended_to_rollout_by":"17.1"}]`,
			want: func(v any) bool {
				e, _ := v.([]FeatureDefinitionExtra)
				return len(e) == 1 && e[0].FeatureIssueURL == "https://example/2" && e[0].IntendedToRolloutBy == "17.1"
			},
		},
		{
			name: "dependency",
			read: first(func(c *gitlabclient.ResponseCapture, n int) (any, error) { return CapturedDependencies(c, n) }),
			body: `[{"name":"rails","malware":true}]`,
			want: func(v any) bool {
				e, _ := v.([]DependencyExtra)
				return len(e) == 1 && e[0].Malware != nil && *e[0].Malware
			},
		},
		{
			name: "bridge",
			read: first(func(c *gitlabclient.ResponseCapture, n int) (any, error) { return CapturedBridges(c, n) }),
			body: `[{"id":1,"project":{"ci_job_token_scope_enabled":true}}]`,
			want: func(v any) bool {
				e, _ := v.([]BridgeExtra)
				return len(e) == 1 && e[0].Project != nil && e[0].Project.CIJobTokenScopeEnabled
			},
		},
		{
			name: "system hook",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedSystemHook(c) },
			body: `{"id":1,"push_events_branch_filter":"release/*","branch_filter_strategy":"wildcard",` +
				`"alert_status":"executable","disabled_until":"2026-02-03T04:05:06Z",` +
				`"custom_webhook_template":"{}","custom_headers":[{"key":"X-Env"}],"organization_id":7}`,
			want: func(v any) bool {
				e, _ := v.(SystemHookExtra)
				return e.PushEventsBranchFilter == "release/*" && e.BranchFilterStrategy == "wildcard" &&
					e.AlertStatus == "executable" && e.DisabledUntil != nil && e.CustomWebhookTemplate == "{}" &&
					len(e.CustomHeaders) == 1 && e.CustomHeaders[0].Key == "X-Env" && e.OrganizationID == 7
			},
		},
		{
			name: "deploy key",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedDeployKey(c) },
			body: `{"id":1,"last_used_at":"2026-04-07T08:09:10Z","usage_type":"auth_and_signing",` +
				`"projects_with_write_access":[{"id":11,"path_with_namespace":"group/writer","created_at":"2026-01-02T03:04:05Z"}],` +
				`"projects_with_readonly_access":[{"id":12,"path_with_namespace":"group/reader"}]}`,
			want: func(v any) bool {
				e, _ := v.(DeployKeyExtra)
				return e.LastUsedAt != nil && e.UsageType == "auth_and_signing" &&
					len(e.ProjectsWithWriteAccess) == 1 && e.ProjectsWithWriteAccess[0].ID == 11 &&
					e.ProjectsWithWriteAccess[0].CreatedAt != nil &&
					len(e.ProjectsWithReadonlyAccess) == 1 && e.ProjectsWithReadonlyAccess[0].ID == 12
			},
		},
		{
			name: "event",
			read: first(func(c *gitlabclient.ResponseCapture, n int) (any, error) { return CapturedEvents(c, n) }),
			body: `[{"id":1,"imported":true,"imported_from":"github",` +
				`"wiki_page":{"format":"markdown","slug":"home","title":"Home","wiki_page_meta_id":77}}]`,
			want: func(v any) bool {
				e, _ := v.([]EventExtra)
				return len(e) == 1 && e[0].Imported && e[0].ImportedFrom == "github" &&
					e[0].WikiPage != nil && e[0].WikiPage.Slug == "home" && e[0].WikiPage.WikiPageMetaID == 77
			},
		},
		{
			name: "namespace",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedNamespace(c) },
			body: `{"id":1,"projects_count":12,"root_repository_size":34567,` +
				`"shared_runners_minutes_limit":400,"extra_shared_runners_minutes_limit":50,` +
				`"additional_purchased_storage_size":10240,"additional_purchased_storage_ends_on":"2027-03-31",` +
				`"max_seats_used_changed_at":"2026-05-06T07:08:09Z","end_date":"2027-01-31"}`,
			want: readTheSentNamespace,
		},
		{
			name: "package",
			read: first(func(c *gitlabclient.ResponseCapture, n int) (any, error) { return CapturedPackages(c, n) }),
			body: `[{"id":10,"creator_id":57,"conan_package_name":"my-pkg","project_id":42,` +
				`"project_path":"group/project","versions":[{"id":9,"version":"0.9.0",` +
				`"tags":[{"id":3,"package_id":9,"name":"stable"}],` +
				`"pipeline":{"id":77,"iid":4,"sha":"abc123","user":{"id":5,"username":"alice"}}}]}]`,
			want: readTheSentPackage,
		},
		{
			name: "snippet",
			read: func(c *gitlabclient.ResponseCapture) (any, error) { return CapturedSnippet(c) },
			body: `{"id":42,"imported":true,"imported_from":"github",` +
				`"ssh_url_to_repo":"git@example:snippets/42.git","http_url_to_repo":"https://example/snippets/42.git"}`,
			want: func(v any) bool {
				e, _ := v.(SnippetExtra)
				return e.Imported && e.ImportedFrom == "github" &&
					e.SSHURLToRepo == "git@example:snippets/42.git" &&
					e.HTTPURLToRepo == "https://example/snippets/42.git"
			},
		},
	}
}

// TestCapturedReaders_ReadEachTailEntity verifies every reader added for the
// fields GitLab sends on the entities client-go models incompletely: each one
// decodes its shape off the keys GitLab spells, and each reports a capture
// nothing ran under. A reader whose shape is only ever read as a list is
// exercised through its list reader, since the two share the shape.
func TestCapturedReaders_ReadEachTailEntity(t *testing.T) {
	_, untouched := gitlabclient.WithResponseCapture(t.Context())
	for _, testCase := range tailReaderCases() {
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

// TestCapturedTailListReaders_HoldTheCountToTheSDKs verifies every list reader
// added for those entities refuses a count other than the SDK's, naming both
// numbers and the entity, so a capture that fell out of step with the decode
// cannot pair an extra with the wrong object.
func TestCapturedTailListReaders_HoldTheCountToTheSDKs(t *testing.T) {
	for _, testCase := range []struct {
		name string
		read func(*gitlabclient.ResponseCapture, int) (int, error)
	}{
		{"topics", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedTopics(c, n)
			return len(x), e
		}},
		{"broadcast messages", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedBroadcastMessages(c, n)
			return len(x), e
		}},
		{"cluster agents", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedClusterAgents(c, n)
			return len(x), e
		}},
		{"license templates", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedLicenseTemplates(c, n)
			return len(x), e
		}},
		{"secure files", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedSecureFiles(c, n)
			return len(x), e
		}},
		{"pipeline triggers", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedPipelineTriggers(c, n)
			return len(x), e
		}},
		{"protected branches", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedProtectedBranches(c, n)
			return len(x), e
		}},
		{"merge request versions", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedMergeRequestDiffs(c, n)
			return len(x), e
		}},
		{"SCIM identities", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedSCIMIdentities(c, n)
			return len(x), e
		}},
		{"runner managers", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedRunnerManagers(c, n)
			return len(x), e
		}},
		{"feature definitions", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedFeatureDefinitions(c, n)
			return len(x), e
		}},
		{"features", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedFeatures(c, n)
			return len(x), e
		}},
		{"feature flag user lists", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedFeatureFlagUserLists(c, n)
			return len(x), e
		}},
		{"commit comments", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedCommitComments(c, n)
			return len(x), e
		}},
		{"dependencies", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedDependencies(c, n)
			return len(x), e
		}},
		{"deployments", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedDeployments(c, n)
			return len(x), e
		}},
		{"pages domains", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedPagesDomains(c, n)
			return len(x), e
		}},
		{"resource state events", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedResourceStateEvents(c, n)
			return len(x), e
		}},
		{"resource milestone events", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedResourceMilestoneEvents(c, n)
			return len(x), e
		}},
		{"todos", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedTodos(c, n)
			return len(x), e
		}},
		{"boards", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedBoards(c, n)
			return len(x), e
		}},
		{"bridges", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedBridges(c, n)
			return len(x), e
		}},
		{"milestones", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedMilestones(c, n)
			return len(x), e
		}},
		{"board lists", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedBoardLists(c, n)
			return len(x), e
		}},
		{"CI variables", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedCIVariables(c, n)
			return len(x), e
		}},
		{"registry repositories", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedRegistryRepositories(c, n)
			return len(x), e
		}},
		{"wiki pages", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedWikis(c, n)
			return len(x), e
		}},
		{"storage moves", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedStorageMoves(c, n)
			return len(x), e
		}},
		{"service accounts", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedServiceAccounts(c, n)
			return len(x), e
		}},
		{"system hooks", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedSystemHooks(c, n)
			return len(x), e
		}},
		{"deploy keys", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedDeployKeys(c, n)
			return len(x), e
		}},
		{"events", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedEvents(c, n)
			return len(x), e
		}},
		{"namespaces", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedNamespaces(c, n)
			return len(x), e
		}},
		{"packages", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedPackages(c, n)
			return len(x), e
		}},
		{"snippets", func(c *gitlabclient.ResponseCapture, n int) (int, error) {
			x, e := CapturedSnippets(c, n)
			return len(x), e
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			body := []byte(`[{},{}]`)
			count, err := testCase.read(gitlabclient.CapturedBody(body), 2)
			if err != nil || count != 2 {
				t.Errorf("read of two = %d, %v; want two extras", count, err)
			}
			_, err = testCase.read(gitlabclient.CapturedBody(body), 1)
			want := "holds 2 " + testCase.name + " and the SDK decoded 1"
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("read with another count = %v, want %q", err, want)
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
