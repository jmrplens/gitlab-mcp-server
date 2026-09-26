// audit_shared_test.go covers the analysis helpers the discovery auditors
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

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestIsGenericUsage_Scenarios_ClassifiesPlaceholderText verifies that the
// placeholder template and blank strings are generic while curated usage
// text is not, including the case-insensitive and whitespace-tolerant forms.
//
// The last four curated cases each hold one edge of the template to the whole
// text: its wording after a lead-in, its wording followed by more guidance,
// and "execute" and "action" as parts of longer words. Without the anchor or
// the word boundary each pins, that curated text would be reported generic.
func TestIsGenericUsage_Scenarios_ClassifiesPlaceholderText(t *testing.T) {
	tests := []struct {
		name  string
		usage string
		want  bool
	}{
		{name: "empty", usage: "", want: true},
		{name: "whitespace only", usage: "  \n\t", want: true},
		{name: "placeholder template", usage: "Use to execute the list action.", want: true},
		{name: "placeholder as the tree writes it", usage: "Use to execute runners domain action.", want: true},
		{name: "placeholder without period", usage: "use to execute branch action", want: true},
		{name: "placeholder with trailing whitespace", usage: "  Use to execute create action.  ", want: true},
		{name: "curated usage", usage: "Use to list the branches of a project.", want: false},
		{name: "placeholder prefix but different ending", usage: "Use to execute a pipeline and wait.", want: false},
		{name: "template wording after a lead-in", usage: "Read the job log first, then use to execute the retry action.", want: false},
		{name: "template wording followed by guidance", usage: "Use to execute the retry action, then poll the job until it finishes.", want: false},
		{name: "execute as the start of a longer word", usage: "Use to executes the scheduled play action.", want: false},
		{name: "action as the end of a longer word", usage: "Use to execute a merge request interaction.", want: false},
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
//
// The padded name is the case that holds the lookup to the trimmed spelling:
// without the trim it misses the projection and the spec reads as healthy
// rather than as the weak description it is. The weak text planted under the
// empty name is what makes the no-tool guard observable: without it that spec
// would read as healthy only because the lookup missed.
func TestWeakIndividualDescription_Scenarios_ChecksProjectedText(t *testing.T) {
	projected := map[string]string{
		"":                  "Lists things.",
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
		{name: "padded name resolves to the projection", tool: "  gitlab_no_returns  ", want: true},
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

// tokenRecorder is an http.RoundTripper that keeps the PRIVATE-TOKEN header
// of every request it forwards. http.Client calls it on the goroutine that
// sent the request, so the test reads what it wrote without synchronization.
type tokenRecorder struct {
	next http.RoundTripper
	seen *[]string
}

// RoundTrip records the request's PRIVATE-TOKEN header and forwards it.
func (r tokenRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	*r.seen = append(*r.seen, req.Header.Get("PRIVATE-TOKEN"))
	return r.next.RoundTrip(req)
}

// TestNewStubGitLabClient_Token_SendsTheTokenItWasGiven verifies the stub
// client authenticates with the caller's token. Every command passes
// StubToken, so a delegation that dropped its argument for that constant would
// read the same from each of them; only the header on the wire tells the two
// apart, which is why the token here is one no constant of the package shares.
func TestNewStubGitLabClient_Token_SendsTheTokenItWasGiven(t *testing.T) {
	const token = "caller-chosen-stub-token"
	client, cleanup := NewStubGitLabClient(token)
	t.Cleanup(cleanup)

	var seen []string
	httpClient := client.GL().HTTPClient()
	next := httpClient.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	httpClient.Transport = tokenRecorder{next: next, seen: &seen}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, _, err := client.GL().Version.GetVersion(gl.WithContext(ctx)); err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("no request reached the transport, want the version request")
	}
	for i, got := range seen {
		if got != token {
			t.Errorf("request %d PRIVATE-TOKEN = %q, want the caller's %q", i, got, token)
		}
	}
}

// TestCachedActionSpecs_RepeatedCalls_ShareOneCollectionPerFlag verifies the
// cache collects the catalog once per enterprise flag: each flag's repeated
// call hands back that flag's own backing slice, and the two flags are cached
// apart rather than sharing one entry, which is what the sync.Map key is for.
//
// What it deliberately does not assert is that the two collections differ.
// Every one of the 46 builders in internal/tools/action_specs.go declares the
// flag as `_ bool`, so CollectActionSpecs returns the same catalog either way
// and the edition tags are gated later by the tier filter. A test comparing
// the two sizes would therefore either pin that equality as intended or fail;
// the flag's journey past this cache cannot be observed from here at all.
func TestCachedActionSpecs_RepeatedCalls_ShareOneCollectionPerFlag(t *testing.T) {
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
	if &free[0] == &enterprise[0] {
		t.Fatal("both flags returned one backing slice, want one cache entry per flag")
	}
	if again := CachedActionSpecs(client, false); len(again) != len(free) || &again[0] != &free[0] {
		t.Fatal("CachedActionSpecs(free) second call did not return the cached slice")
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

// TestCachedIndividualDescriptions_CatalogSpecs_EveryIndividualToolIsProjected
// verifies the projection holds a description for every individual tool the
// collected catalog names, Premium and Ultimate ones included. Both discovery
// audits read a spec the projection lacks as healthy, so a surface listed
// below Ultimate would pass every licensed description unread while the count
// of Free tools still looked like the whole surface.
func TestCachedIndividualDescriptions_CatalogSpecs_EveryIndividualToolIsProjected(t *testing.T) {
	client, cleanup := NewStubGitLabClient(StubToken)
	t.Cleanup(cleanup)

	projected := CachedIndividualDescriptions(client)
	checked := make(map[edition.Tier]int)
	var missing []string
	for _, group := range CachedActionSpecs(client, true) {
		for _, spec := range group.Actions {
			name := strings.TrimSpace(spec.IndividualTool.Name)
			if name == "" {
				continue
			}
			tier := edition.TierFromEdition(spec.Edition)
			checked[tier]++
			if _, ok := projected[name]; !ok {
				missing = append(missing, name+" ("+tier.String()+")")
			}
		}
	}
	if checked[edition.Premium] == 0 || checked[edition.Ultimate] == 0 {
		t.Fatalf("individual tools checked per tier = %v, want Premium and Ultimate ones among them", checked)
	}
	if len(missing) > 0 {
		t.Fatalf("%d of the catalog's individual tools have no projected description, first %q", len(missing), missing[:min(len(missing), 5)])
	}
}
