// plan_limits_test.go contains unit tests for the plan limit MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package planlimits

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// planLimitJSON identifies the plan limit JSON constant used by this package:
// every key lib/api/entities/plan_limit.rb exposes, which is what both plan
// limit routes answer with.
//
// No two limits share a value on purpose. GitLab really does serve the same
// number for several package formats, and a fixture that copies that makes a
// converter reading maven out of pypi indistinguishable from a correct one.
const planLimitJSON = `{
	"cargo_max_file_size": 6442450944,
	"conan_max_file_size": 3221225472,
	"generic_packages_max_file_size": 5368709120,
	"helm_max_file_size": 5242880,
	"maven_max_file_size": 2147483648,
	"npm_max_file_size": 524288000,
	"nuget_max_file_size": 419430400,
	"pypi_max_file_size": 1610612736,
	"terraform_module_max_file_size": 1073741824,
	"ci_instance_level_variables": 25,
	"ci_pipeline_size": 1,
	"ci_active_jobs": 2,
	"ci_project_subscriptions": 3,
	"ci_pipeline_schedules": 10,
	"ci_needs_size_limit": 50,
	"ci_registered_group_runners": 1000,
	"ci_registered_project_runners": 1001,
	"dotenv_variables": 20,
	"dotenv_size": 5120,
	"enforcement_limit": 15000,
	"notification_limit": 14000,
	"storage_size_limit": 16000,
	"pipeline_hierarchy_size": 999,
	"max_pipelines_per_merge_train": 21,
	"service_desk_outbound_emails_per_hour": 100,
	"service_desk_outbound_emails_per_day": 700,
	"web_hook_calls": 500,
	"web_hook_calls_low": 501,
	"web_hook_calls_mid": 502,
	"limits_history": {
		"enforcement_limit": [{"timestamp": 1686909124, "user_id": 1, "username": "root", "value": 5}],
		"notification_limit": [{"timestamp": 1686909200, "user_id": 2, "username": "operator", "value": 7}]
	}
}`

// olderPlanLimitJSON is the answer of an instance that predates every limit
// client-go does not model: the eight package file sizes and nothing else.
const olderPlanLimitJSON = `{
	"conan_max_file_size": 3221225472,
	"generic_packages_max_file_size": 5368709120,
	"helm_max_file_size": 5242880,
	"maven_max_file_size": 2147483648,
	"npm_max_file_size": 524288000,
	"nuget_max_file_size": 419430400,
	"pypi_max_file_size": 1610612736,
	"terraform_module_max_file_size": 1073741824
}`

// wantPlanLimits is planLimitJSON read as the output it has to produce: the
// answer to "which GitLab key did this field come from", one row per field.
var wantPlanLimits = PlanLimitItem{
	ConanMaxFileSize:           3221225472,
	GenericPackagesMaxFileSize: 5368709120,
	HelmMaxFileSize:            5242880,
	MavenMaxFileSize:           2147483648,
	NPMMaxFileSize:             524288000,
	NugetMaxFileSize:           419430400,
	PyPiMaxFileSize:            1610612736,
	TerraformModuleMaxFileSize: 1073741824,

	CargoMaxFileSize:                 new(int64(6442450944)),
	CIInstanceLevelVariables:         new(int64(25)),
	CIPipelineSize:                   new(int64(1)),
	CIActiveJobs:                     new(int64(2)),
	CIProjectSubscriptions:           new(int64(3)),
	CIPipelineSchedules:              new(int64(10)),
	CINeedsSizeLimit:                 new(int64(50)),
	CIRegisteredGroupRunners:         new(int64(1000)),
	CIRegisteredProjectRunners:       new(int64(1001)),
	DotenvVariables:                  new(int64(20)),
	DotenvSize:                       new(int64(5120)),
	EnforcementLimit:                 new(int64(15000)),
	NotificationLimit:                new(int64(14000)),
	StorageSizeLimit:                 new(int64(16000)),
	PipelineHierarchySize:            new(int64(999)),
	MaxPipelinesPerMergeTrain:        new(int64(21)),
	ServiceDeskOutboundEmailsPerHour: new(int64(100)),
	ServiceDeskOutboundEmailsPerDay:  new(int64(700)),
	WebHookCalls:                     new(int64(500)),
	WebHookCallsLow:                  new(int64(501)),
	WebHookCallsMid:                  new(int64(502)),
	LimitsHistory: map[string][]LimitChange{
		"enforcement_limit":  {{Timestamp: 1686909124, UserID: 1, Username: "root", Value: 5}},
		"notification_limit": {{Timestamp: 1686909200, UserID: 2, Username: "operator", Value: 7}},
	},
}

// wantOlderPlanLimits is olderPlanLimitJSON read as its output: the eight
// sizes, and every limit the instance did not send left nil.
var wantOlderPlanLimits = PlanLimitItem{
	ConanMaxFileSize:           3221225472,
	GenericPackagesMaxFileSize: 5368709120,
	HelmMaxFileSize:            5242880,
	MavenMaxFileSize:           2147483648,
	NPMMaxFileSize:             524288000,
	NugetMaxFileSize:           419430400,
	PyPiMaxFileSize:            1610612736,
	TerraformModuleMaxFileSize: 1073741824,
}

// TestGet_Success verifies Get when success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/plan_limits" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ConanMaxFileSize != 3221225472 {
		t.Fatalf("expected conan_max_file_size 3221225472, got %d", out.ConanMaxFileSize)
	}
	if out.GenericPackagesMaxFileSize != 5368709120 {
		t.Fatalf("expected generic_packages_max_file_size 5368709120, got %d", out.GenericPackagesMaxFileSize)
	}
}

// TestGet_WithPlanName verifies Get when with plan name.
func TestGet_WithPlanName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/plan_limits" && r.Method == http.MethodGet {
			if r.URL.Query().Get("plan_name") != "default" {
				t.Errorf("expected plan_name=default, got %s", r.URL.Query().Get("plan_name"))
			}
			testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{PlanName: "default"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.NPMMaxFileSize != 524288000 {
		t.Fatalf("expected npm_max_file_size 524288000, got %d", out.NPMMaxFileSize)
	}
}

// TestGet_EveryLimit_CarriesTheValueOfItsOwnKey verifies that each limit in
// the output is the one GitLab sent under the matching key, the eight the SDK
// decodes and the ones read from the captured response alike, and that the
// change history comes through whole.
//
// The converter is straight-line assignments and no branch, so a field read
// out of a neighbour's key answers plausibly and wrongly; only a fixture where
// no two limits agree can tell the two apart.
func TestGet_EveryLimit_CarriesTheValueOfItsOwnKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/plan_limits" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !reflect.DeepEqual(out.PlanLimitItem, wantPlanLimits) {
		t.Errorf("Get() limits = %+v, want %+v", out.PlanLimitItem, wantPlanLimits)
	}
}

// TestGet_OlderInstance_LeavesTheLimitsItDidNotSendOut verifies that a limit
// an instance sends no key for stays nil and is absent from the JSON, rather
// than published as zero.
//
// GitLab adds limits most releases, and zero is a value several of them read
// as unlimited: a zero for a limit the instance does not have would tell the
// caller something the instance never said.
func TestGet_OlderInstance_LeavesTheLimitsItDidNotSendOut(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/plan_limits" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, olderPlanLimitJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !reflect.DeepEqual(out.PlanLimitItem, wantOlderPlanLimits) {
		t.Errorf("Get() limits = %+v, want %+v", out.PlanLimitItem, wantOlderPlanLimits)
	}
	encoded, err := json.Marshal(out.PlanLimitItem)
	if err != nil {
		t.Fatalf("marshal the limits: %v", err)
	}
	var keys map[string]any
	if err = json.Unmarshal(encoded, &keys); err != nil {
		t.Fatalf("unmarshal the limits: %v", err)
	}
	if len(keys) != 8 {
		t.Errorf("published %d keys (%v), want the 8 the instance sent", len(keys), keys)
	}
}

// TestPlanLimits_UnreadableCapturedAnswer_IsAnError verifies that both
// handlers report an answer the captured-response reader cannot hold, rather
// than publishing the limits with the rest silently missing.
//
// The history is a key the SDK's struct does not declare, so a body carrying
// it in a shape nobody expects decodes cleanly there and fails only here,
// which is the one place the fault can be seen.
func TestPlanLimits_UnreadableCapturedAnswer_IsAnError(t *testing.T) {
	const unreadable = `{"conan_max_file_size": 1, "limits_history": "not an object"}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, unreadable)
	}))

	t.Run("get", func(t *testing.T) {
		if _, err := Get(t.Context(), client, GetInput{}); err == nil {
			t.Fatal("Get() error = nil, want the captured answer refused")
		}
	})
	t.Run("change", func(t *testing.T) {
		if _, err := Change(t.Context(), client, ChangeInput{PlanName: "default"}); err == nil {
			t.Fatal("Change() error = nil, want the captured answer refused")
		}
	})
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestChange_Success verifies Change when success.
func TestChange_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/plan_limits" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
			return
		}
		http.NotFound(w, r)
	}))

	size := int64(1073741824)
	out, err := Change(t.Context(), client, ChangeInput{
		PlanName:        "default",
		HelmMaxFileSize: &size,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.HelmMaxFileSize != 5242880 {
		t.Fatalf("expected helm_max_file_size 5242880, got %d", out.HelmMaxFileSize)
	}
}

// TestChange_EverySetLimit_ReachesGitLabUnderItsOwnKey verifies that the plan
// name and every limit the caller set are sent under their own keys, and that
// what GitLab answers comes back as the action's output.
//
// Nothing else guards the request: the options are eight straight-line
// assignments, so a limit routed to a neighbour's field, or left out, changes
// what the instance is told without changing what the caller is shown.
func TestChange_EverySetLimit_ReachesGitLabUnderItsOwnKey(t *testing.T) {
	limits := map[string]int64{
		"conan_max_file_size":            11,
		"generic_packages_max_file_size": 22,
		"helm_max_file_size":             33,
		"maven_max_file_size":            44,
		"npm_max_file_size":              55,
		"nuget_max_file_size":            66,
		"pypi_max_file_size":             77,
		"terraform_module_max_file_size": 88,
	}
	wantBody := map[string]any{"plan_name": "silver"}
	for key, value := range limits {
		wantBody[key] = float64(value)
	}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/application/plan_limits" || r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding the request body: %v", err)
		} else if !reflect.DeepEqual(body, wantBody) {
			t.Errorf("PUT body = %v, want %v", body, wantBody)
		}
		testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
	}))

	out, err := Change(t.Context(), client, ChangeInput{
		PlanName:                   "silver",
		ConanMaxFileSize:           new(limits["conan_max_file_size"]),
		GenericPackagesMaxFileSize: new(limits["generic_packages_max_file_size"]),
		HelmMaxFileSize:            new(limits["helm_max_file_size"]),
		MavenMaxFileSize:           new(limits["maven_max_file_size"]),
		NPMMaxFileSize:             new(limits["npm_max_file_size"]),
		NugetMaxFileSize:           new(limits["nuget_max_file_size"]),
		PyPiMaxFileSize:            new(limits["pypi_max_file_size"]),
		TerraformModuleMaxFileSize: new(limits["terraform_module_max_file_size"]),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !reflect.DeepEqual(out.PlanLimitItem, wantPlanLimits) {
		t.Errorf("Change() limits = %+v, want %+v", out.PlanLimitItem, wantPlanLimits)
	}
}

// TestChange_LimitsLeftUnset_AreOmittedFromTheRequest verifies that a limit
// the caller did not name is absent from the PUT body rather than sent as
// zero.
//
// Zero is not "leave this alone" to GitLab but "allow nothing", so sending one
// for every unnamed format would forbid every upload of it while the caller
// asked to change one number.
func TestChange_LimitsLeftUnset_AreOmittedFromTheRequest(t *testing.T) {
	wantBody := map[string]any{"plan_name": "default", "helm_max_file_size": float64(5242880)}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/application/plan_limits" || r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding the request body: %v", err)
		} else if !reflect.DeepEqual(body, wantBody) {
			t.Errorf("PUT body = %v, want %v", body, wantBody)
		}
		testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
	}))

	size := int64(5242880)
	if _, err := Change(t.Context(), client, ChangeInput{PlanName: "default", HelmMaxFileSize: &size}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestChange_Error verifies Change when error.
func TestChange_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Change(t.Context(), client, ChangeInput{PlanName: "default"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatGetMarkdown_EveryLimit_RendersTheWholeCard verifies that the plan
// limits render as one card whose every row carries the unit GitLab counts the
// limit in, a byte count with the binary-prefix size in front of the exact
// number and any other count with its unit in the label, followed by the
// change history in name order, each change with who made it and when.
func TestFormatGetMarkdown_EveryLimit_RendersTheWholeCard(t *testing.T) {
	limits := wantPlanLimits
	limits.LimitsHistory = map[string][]LimitChange{
		"notification_limit": {{Timestamp: 1686909200, UserID: 2, Username: "operator", Value: 7}},
		"enforcement_limit": {
			{Timestamp: 1686909124, UserID: 1, Username: "root", Value: 5},
			{Timestamp: 1686909300, UserID: 3, Username: "ops|lead", Value: 9},
		},
	}
	out := GetOutput{PlanLimitItem: limits}

	want := "## Plan Limits\n\n" +
		"- **Cargo Max File Size**: 6 GiB (6442450944 bytes)\n" +
		"- **Conan Max File Size**: 3 GiB (3221225472 bytes)\n" +
		"- **Generic Packages Max File Size**: 5 GiB (5368709120 bytes)\n" +
		"- **Helm Max File Size**: 5 MiB (5242880 bytes)\n" +
		"- **Maven Max File Size**: 2 GiB (2147483648 bytes)\n" +
		"- **NPM Max File Size**: 500 MiB (524288000 bytes)\n" +
		"- **NuGet Max File Size**: 400 MiB (419430400 bytes)\n" +
		"- **PyPI Max File Size**: 1.5 GiB (1610612736 bytes)\n" +
		"- **Terraform Module Max File Size**: 1 GiB (1073741824 bytes)\n" +
		"- **CI Instance-Level Variables**: 25\n" +
		"- **CI Pipeline Size (jobs in one pipeline)**: 1\n" +
		"- **CI Active Jobs (jobs in active pipelines)**: 2\n" +
		"- **CI Project Subscriptions**: 3\n" +
		"- **CI Pipeline Schedules**: 10\n" +
		"- **CI Needs Size Limit (needs per job)**: 50\n" +
		"- **CI Registered Group Runners (per group, past seven days)**: 1000\n" +
		"- **CI Registered Project Runners (per project, past seven days)**: 1001\n" +
		"- **Pipeline Hierarchy Size (downstream pipelines)**: 999\n" +
		"- **Max Pipelines per Merge Train**: 21\n" +
		"- **Dotenv Variables (per artifact)**: 20\n" +
		"- **Dotenv Size**: 5 KiB (5120 bytes)\n" +
		"- **Storage Size Limit (MiB)**: 16000\n" +
		"- **Enforcement Limit (MiB)**: 15000\n" +
		"- **Notification Limit (MiB)**: 14000\n" +
		"- **Webhook Calls (per minute, per top-level namespace)**: 500\n" +
		"- **Webhook Calls Low (per minute, per top-level namespace)**: 501\n" +
		"- **Webhook Calls Mid (per minute, per top-level namespace)**: 502\n" +
		"- **Service Desk Outbound Emails per Hour (per top-level namespace)**: 100\n" +
		"- **Service Desk Outbound Emails per Day (per top-level namespace)**: 700\n" +
		"\n### Limits History\n\n" +
		"| Limit | Value | Changed By | Changed At |\n" +
		"| --- | --- | --- | --- |\n" +
		"| enforcement_limit | 5 | root | 16 Jun 2023 09:52 UTC |\n" +
		"| enforcement_limit | 9 | ops&#124;lead | 16 Jun 2023 09:55 UTC |\n" +
		"| notification_limit | 7 | operator | 16 Jun 2023 09:53 UTC |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.plan_limits_change' to raise or lower one of these limits\n"

	if got := FormatGetMarkdown(out); got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatChangeMarkdown_SmallAndZeroLimits_RendersTheWholeCard verifies that
// the card a change returns is the card a read returns, that a limit below a
// kibibyte renders as a plain byte count, that a zero limit is written rather
// than dropped because zero is an answer GitLab gave, and that a limit the
// instance sent no key for writes no row at all.
//
// Every limit the instance sent is zero here, which is what a default
// self-managed plan sends for several of them, so each row shows which of the
// two readings GitLab gives a zero: no limit for the package file sizes and
// every limit GitLab checks through PlanLimits#exceeded? or behind a `> 0`
// guard, and a real bound for the needs list and the two dotenv limits, which
// it compares against the value directly.
func TestFormatChangeMarkdown_SmallAndZeroLimits_RendersTheWholeCard(t *testing.T) {
	zero := func() *int64 { return new(int64(0)) }
	out := ChangeOutput{
		ConanMaxFileSize:                 1023,
		HelmMaxFileSize:                  1024,
		CIInstanceLevelVariables:         zero(),
		CIPipelineSize:                   zero(),
		CIActiveJobs:                     zero(),
		CIProjectSubscriptions:           zero(),
		CIPipelineSchedules:              zero(),
		CINeedsSizeLimit:                 zero(),
		CIRegisteredGroupRunners:         zero(),
		CIRegisteredProjectRunners:       zero(),
		PipelineHierarchySize:            zero(),
		DotenvVariables:                  zero(),
		DotenvSize:                       zero(),
		StorageSizeLimit:                 zero(),
		EnforcementLimit:                 zero(),
		NotificationLimit:                zero(),
		WebHookCalls:                     zero(),
		WebHookCallsLow:                  zero(),
		WebHookCallsMid:                  zero(),
		ServiceDeskOutboundEmailsPerHour: zero(),
		ServiceDeskOutboundEmailsPerDay:  zero(),
	}

	want := "## Updated Plan Limits\n\n" +
		"- **Conan Max File Size**: 1023 bytes\n" +
		"- **Generic Packages Max File Size**: unlimited (0)\n" +
		"- **Helm Max File Size**: 1 KiB (1024 bytes)\n" +
		"- **Maven Max File Size**: unlimited (0)\n" +
		"- **NPM Max File Size**: unlimited (0)\n" +
		"- **NuGet Max File Size**: unlimited (0)\n" +
		"- **PyPI Max File Size**: unlimited (0)\n" +
		"- **Terraform Module Max File Size**: unlimited (0)\n" +
		"- **CI Instance-Level Variables**: unlimited (0)\n" +
		"- **CI Pipeline Size (jobs in one pipeline)**: unlimited (0)\n" +
		"- **CI Active Jobs (jobs in active pipelines)**: unlimited (0)\n" +
		"- **CI Project Subscriptions**: unlimited (0)\n" +
		"- **CI Pipeline Schedules**: unlimited (0)\n" +
		"- **CI Needs Size Limit (needs per job)**: 0\n" +
		"- **CI Registered Group Runners (per group, past seven days)**: unlimited (0)\n" +
		"- **CI Registered Project Runners (per project, past seven days)**: unlimited (0)\n" +
		"- **Pipeline Hierarchy Size (downstream pipelines)**: unlimited (0)\n" +
		"- **Dotenv Variables (per artifact)**: 0\n" +
		"- **Dotenv Size**: 0 bytes\n" +
		"- **Storage Size Limit (MiB)**: unlimited (0)\n" +
		"- **Enforcement Limit (MiB)**: unlimited (0)\n" +
		"- **Notification Limit (MiB)**: unlimited (0)\n" +
		"- **Webhook Calls (per minute, per top-level namespace)**: unlimited (0)\n" +
		"- **Webhook Calls Low (per minute, per top-level namespace)**: unlimited (0)\n" +
		"- **Webhook Calls Mid (per minute, per top-level namespace)**: unlimited (0)\n" +
		"- **Service Desk Outbound Emails per Hour (per top-level namespace)**: unlimited (0)\n" +
		"- **Service Desk Outbound Emails per Day (per top-level namespace)**: unlimited (0)\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.plan_limits_get' to read the plan's limits back\n"

	if got := FormatChangeMarkdown(out); got != want {
		t.Errorf("FormatChangeMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestLimitCells_Zero_ReadTheWayGitLabReadsIt verifies the cells on their own.
// For a package file size zero is GitLab's "allow any file size" and says so,
// whether the limit is one client-go models or one read from the captured
// response, and any other value keeps the byte count the change action takes
// back. The dotenv size is compared against its value directly, so its zero is
// printed as the bound it is. A count GitLab reads as no limit at zero says so
// and prints any other value bare. A captured limit the instance did not send
// writes nothing.
func TestLimitCells_Zero_ReadTheWayGitLabReadsIt(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "a zero modeled limit", got: sizeLimit(0), want: "unlimited (0)"},
		{name: "a set modeled limit", got: sizeLimit(1024), want: "1 KiB (1024 bytes)"},
		{name: "a zero captured limit", got: optionalSizeLimit(new(int64(0))), want: "unlimited (0)"},
		{name: "a set captured limit", got: optionalSizeLimit(new(int64(1023))), want: "1023 bytes"},
		{name: "a captured limit the instance did not send", got: optionalSizeLimit(nil), want: ""},
		{name: "a zero dotenv size", got: optionalFileSize(new(int64(0))), want: "0 bytes"},
		{name: "a dotenv size the instance did not send", got: optionalFileSize(nil), want: ""},
		{name: "a zero count read as no limit", got: unlimitedAtZero(new(int64(0))), want: "unlimited (0)"},
		{name: "a set count read as no limit at zero", got: unlimitedAtZero(new(int64(3))), want: "3"},
		{name: "a count the instance did not send", got: unlimitedAtZero(nil), want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

// TestFileSize_EveryMagnitude_CarriesTheUnit verifies that a byte count renders
// with the prefix a reader judges it by and the exact count the change action
// takes back, at every magnitude the helper distinguishes.
func TestFileSize_EveryMagnitude_CarriesTheUnit(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "zero", bytes: 0, want: "0 bytes"},
		{name: "below a kibibyte", bytes: 1023, want: "1023 bytes"},
		{name: "exactly a kibibyte", bytes: 1024, want: "1 KiB (1024 bytes)"},
		{name: "fractional kibibytes", bytes: 1536, want: "1.5 KiB (1536 bytes)"},
		{name: "mebibytes", bytes: 5242880, want: "5 MiB (5242880 bytes)"},
		{name: "gibibytes", bytes: 3221225472, want: "3 GiB (3221225472 bytes)"},
		{name: "tebibytes", bytes: 1099511627776, want: "1 TiB (1099511627776 bytes)"},
		{name: "pebibytes", bytes: 1125899906842624, want: "1 PiB (1125899906842624 bytes)"},
		{name: "exbibytes", bytes: 1152921504606846976, want: "1 EiB (1152921504606846976 bytes)"},
		{name: "the largest prefix the table has", bytes: 1 << 62, want: "4 EiB (4611686018427387904 bytes)"},
		{name: "negative", bytes: -1, want: "-1 bytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fileSize(tt.bytes); got != tt.want {
				t.Errorf("fileSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Change — all optional fields
// ---------------------------------------------------------------------------.

// TestChange_AllOptionalFields verifies Change when all optional fields.
func TestChange_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/plan_limits" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
			return
		}
		http.NotFound(w, r)
	}))

	v1, v2, v3, v4, v5, v6, v7 := int64(100), int64(200), int64(300), int64(400), int64(500), int64(600), int64(700)
	out, err := Change(t.Context(), client, ChangeInput{
		PlanName:                   "default",
		ConanMaxFileSize:           &v1,
		GenericPackagesMaxFileSize: &v2,
		HelmMaxFileSize:            &v3,
		MavenMaxFileSize:           &v4,
		NPMMaxFileSize:             &v5,
		NugetMaxFileSize:           &v6,
		PyPiMaxFileSize:            &v7,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ConanMaxFileSize == 0 {
		t.Error("expected non-zero conan field from response")
	}
}

// TestPlanLimits_EachHandlerReachesItsOwnEndpoint drives both handlers against
// the two endpoints GitLab serves the plan limits on, so a handler sending the
// wrong method or path fails rather than being answered by a catch-all.
func TestPlanLimits_EachHandlerReachesItsOwnEndpoint(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/application/plan_limits", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
	})
	handler.HandleFunc("PUT /api/v4/application/plan_limits", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
	})
	client := testutil.NewTestClient(t, handler)

	t.Run("get", func(t *testing.T) {
		if _, err := Get(t.Context(), client, GetInput{PlanName: "default"}); err != nil {
			t.Fatalf("Get() error = %v, want nil", err)
		}
	})
	t.Run("change", func(t *testing.T) {
		size := int64(5242880)
		if _, err := Change(t.Context(), client, ChangeInput{PlanName: "default", HelmMaxFileSize: &size}); err != nil {
			t.Fatalf("Change() error = %v, want nil", err)
		}
	})
}
