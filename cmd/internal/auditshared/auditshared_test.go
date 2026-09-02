// auditshared_test.go covers the analysis helpers the discovery auditors
// share: the placeholder-usage probe, the projected individual-description
// check, owner-package resolution, the per-process caches, and the offline
// stub GitLab client.
package auditshared

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestIsGenericUsage_Scenarios_ClassifiesPlaceholderText verifies that the
// placeholder template and blank strings are generic while curated usage
// text is not, including the case-insensitive and whitespace-tolerant forms.
func TestIsGenericUsage_Scenarios_ClassifiesPlaceholderText(t *testing.T) {
	tests := []struct {
		name  string
		usage string
		want  bool
	}{
		{name: "empty", usage: "", want: true},
		{name: "whitespace only", usage: "  \n\t", want: true},
		{name: "placeholder template", usage: "Use to execute the list action.", want: true},
		{name: "placeholder without period", usage: "use to execute branch action", want: true},
		{name: "placeholder with trailing whitespace", usage: "  Use to execute create action.  ", want: true},
		{name: "curated usage", usage: "Use to list the branches of a project.", want: false},
		{name: "placeholder prefix but different ending", usage: "Use to execute a pipeline and wait.", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGenericUsage(tt.usage); got != tt.want {
				t.Errorf("IsGenericUsage(%q) = %v, want %v", tt.usage, got, tt.want)
			}
		})
	}
}

// TestWeakIndividualDescription_Scenarios_ChecksProjectedText verifies the
// check reads the projected description of the spec's individual tool and
// only flags a description missing the "Returns:" or "See also:" sections;
// specs without an individual tool or without a projection are never weak.
func TestWeakIndividualDescription_Scenarios_ChecksProjectedText(t *testing.T) {
	projected := map[string]string{
		"gitlab_full":       "Lists things. Returns: a list. See also: gitlab_other.",
		"gitlab_no_returns": "Lists things. See also: gitlab_other.",
		"gitlab_no_see":     "Lists things. Returns: a list.",
	}
	tests := []struct {
		name string
		tool string
		want bool
	}{
		{name: "no individual tool", tool: "", want: false},
		{name: "whitespace tool name", tool: "   ", want: false},
		{name: "tool not projected", tool: "gitlab_unknown", want: false},
		{name: "complete description", tool: "gitlab_full", want: false},
		{name: "missing returns", tool: "gitlab_no_returns", want: true},
		{name: "missing see also", tool: "gitlab_no_see", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := toolutil.ActionSpec{IndividualTool: toolutil.IndividualToolSpec{Name: tt.tool}}
			if got := WeakIndividualDescription(spec, projected); got != tt.want {
				t.Errorf("WeakIndividualDescription(%q) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}

// TestOwnerPackage_Scenarios_ResolvesInPrecedenceOrder verifies the owner
// resolution order: the spec override wins, then the group owner, then the
// group base domain, with surrounding whitespace trimmed at every level.
func TestOwnerPackage_Scenarios_ResolvesInPrecedenceOrder(t *testing.T) {
	tests := []struct {
		name  string
		group tools.ActionSpecGroup
		spec  toolutil.ActionSpec
		want  string
	}{
		{
			name:  "spec override wins",
			group: tools.ActionSpecGroup{OwnerPackage: "groupowner", BaseDomain: "domain"},
			spec:  toolutil.ActionSpec{OwnerPackage: " specowner "},
			want:  "specowner",
		},
		{
			name:  "group owner when spec has none",
			group: tools.ActionSpecGroup{OwnerPackage: " groupowner ", BaseDomain: "domain"},
			spec:  toolutil.ActionSpec{OwnerPackage: "  "},
			want:  "groupowner",
		},
		{
			name:  "base domain as last resort",
			group: tools.ActionSpecGroup{BaseDomain: " domain "},
			spec:  toolutil.ActionSpec{},
			want:  "domain",
		},
		{
			name:  "nothing set",
			group: tools.ActionSpecGroup{},
			spec:  toolutil.ActionSpec{},
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OwnerPackage(tt.group, tt.spec); got != tt.want {
				t.Errorf("OwnerPackage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNewStubGitLabClient_Default_AnswersVersionAndClosesOnCleanup verifies
// the stub answers the version endpoint with its fixed payload through the
// returned client, and that the cleanup shuts the stub down so a later
// connection to it is refused.
func TestNewStubGitLabClient_Default_AnswersVersionAndClosesOnCleanup(t *testing.T) {
	client, cleanup := NewStubGitLabClient("stub-token")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	version, err := client.Ping(ctx)
	if err != nil || version != "17.0.0" {
		t.Fatalf("Ping() = %q, %v; want the stub's fixed 17.0.0 payload", version, err)
	}

	stubURL := client.GL().BaseURL()
	if stubURL.Hostname() != "127.0.0.1" {
		t.Fatalf("client base URL = %q, want an in-process loopback stub", stubURL)
	}
	cleanup()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+stubURL.Host+"/", http.NoBody)
	if err != nil {
		t.Fatalf("build probe request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("connection after cleanup succeeded, want the stub to be closed")
	}
}

// TestCachedActionSpecs_RepeatedCalls_ShareOneCollection verifies the cache
// collects the catalog once per tier flag: repeated calls return the same
// backing slice, the two tiers are collected independently, and the
// enterprise collection is at least as large as the free one.
func TestCachedActionSpecs_RepeatedCalls_ShareOneCollection(t *testing.T) {
	client, cleanup := NewStubGitLabClient("stub-token")
	t.Cleanup(cleanup)

	enterprise := CachedActionSpecs(client, true)
	if len(enterprise) == 0 {
		t.Fatal("CachedActionSpecs(enterprise) returned no groups")
	}
	if again := CachedActionSpecs(client, true); len(again) != len(enterprise) || &again[0] != &enterprise[0] {
		t.Fatal("CachedActionSpecs(enterprise) second call did not return the cached slice")
	}
	free := CachedActionSpecs(client, false)
	if len(free) == 0 || len(free) > len(enterprise) {
		t.Fatalf("CachedActionSpecs(free) = %d groups, enterprise = %d; want 0 < free <= enterprise", len(free), len(enterprise))
	}
}

// TestCachedIndividualDescriptions_RepeatedCalls_ProjectOnce verifies the
// projection registers the individual surface over a real tools/list
// round-trip once per process: the map carries the model-facing text of a
// well-known tool, and a second call hands back the very same map.
func TestCachedIndividualDescriptions_RepeatedCalls_ProjectOnce(t *testing.T) {
	client, cleanup := NewStubGitLabClient("stub-token")
	t.Cleanup(cleanup)

	descriptions := CachedIndividualDescriptions(client)
	if len(descriptions) < 800 {
		t.Fatalf("projected %d descriptions, want the full individual surface", len(descriptions))
	}
	if description := descriptions["gitlab_project_get"]; !strings.Contains(description, "Returns:") {
		t.Fatalf("gitlab_project_get description = %q, want the projected Returns section", description)
	}
	again := CachedIndividualDescriptions(client)
	if reflect.ValueOf(again).Pointer() != reflect.ValueOf(descriptions).Pointer() {
		t.Fatal("second call returned a different map, want the shared cached map")
	}
}
