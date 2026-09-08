package auditshared

import (
	"regexp"
	"strings"
	"sync"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// genericUsageRe matches the placeholder Usage template ("Use to execute …
// action.") that indicates the action has no curated usage text.
var genericUsageRe = regexp.MustCompile(`(?i)^use to execute\b.*\baction\.?\s*$`)

// IsGenericUsage reports whether the Usage string is the placeholder template
// or empty.
func IsGenericUsage(usage string) bool {
	trimmed := strings.TrimSpace(usage)
	return trimmed == "" || genericUsageRe.MatchString(trimmed)
}

// WeakIndividualDescription reports whether the effective individual-tool
// description the model sees lacks the norm's "Returns: … See also: …" form.
// The effective description is the projected mcp.Tool.Description (which
// already resolves the curated-description fallback chain), so this avoids
// false positives from specs that omit IndividualTool.Description yet still
// project a good curated one.
func WeakIndividualDescription(spec toolutil.ActionSpec, projected map[string]string) bool {
	tool := strings.TrimSpace(spec.IndividualTool.Name)
	if tool == "" {
		return false
	}
	description, ok := projected[tool]
	if !ok {
		return false
	}
	return !strings.Contains(description, "Returns:") || !strings.Contains(description, "See also:")
}

// projectionCache memoizes ProjectIndividualDescriptions. Projecting the
// individual surface registers ~1071 tools with schema compilation (~10s),
// the output depends only on the compiled-in catalog (the client is an
// offline stub), and the audits only read the map. One projection per
// process therefore serves every analyzer and every test in a package; the
// one-shot CLIs are unaffected.
var projectionCache struct {
	once         sync.Once
	descriptions map[string]string
}

// specsCache memoizes CachedActionSpecs per enterprise flag, same contract:
// the returned groups are shared and must be treated as read-only.
var specsCache sync.Map // enterprise bool -> *specsResult

type specsResult struct {
	once   sync.Once
	groups []tools.ActionSpecGroup
}

// CachedIndividualDescriptions returns the projected individual-tool
// descriptions, computed once per process. The map is shared: read-only.
func CachedIndividualDescriptions(client *gitlabclient.Client) map[string]string {
	projectionCache.once.Do(func() {
		projectionCache.descriptions = ProjectIndividualDescriptions(client)
	})
	return projectionCache.descriptions
}

// CachedActionSpecs returns the collected action specs for the given tier
// selector, computed once per process and per flag. The slice and everything
// it references are shared: read-only.
func CachedActionSpecs(client *gitlabclient.Client, enterprise bool) []tools.ActionSpecGroup {
	entry, _ := specsCache.LoadOrStore(enterprise, &specsResult{})
	result, _ := entry.(*specsResult)
	result.once.Do(func() {
		result.groups = tools.CollectActionSpecs(client, enterprise)
	})
	return result.groups
}

// ProjectIndividualDescriptions returns the projected description per
// individual-tool name: the exact text the model consumes. Prefer
// CachedIndividualDescriptions unless a fresh projection is the point.
//
// It is a projection over [mcpsurface.IndividualTools], which is the one
// reader of the served surface: the listing itself, the served-schema chain
// applied to it and the memo behind it all live there.
func ProjectIndividualDescriptions(client *gitlabclient.Client) map[string]string {
	listed := mcpsurface.IndividualTools(client, edition.Ultimate)
	descriptions := make(map[string]string, len(listed))
	for _, tool := range listed {
		descriptions[tool.Name] = tool.Description
	}
	return descriptions
}

// OwnerPackage resolves the owning package for an action: the spec override
// wins, then the group owner, then the group base domain.
func OwnerPackage(group tools.ActionSpecGroup, spec toolutil.ActionSpec) string {
	if owner := strings.TrimSpace(spec.OwnerPackage); owner != "" {
		return owner
	}
	if owner := strings.TrimSpace(group.OwnerPackage); owner != "" {
		return owner
	}
	return strings.TrimSpace(group.BaseDomain)
}

// StubToken is the dummy credential the audit commands authenticate their stub
// client with. It is never sent to a real GitLab instance: the client points at
// an in-process HTTP server.
const StubToken = "audit-token" //#nosec G101 -- not a real credential, in-process stub only

// NewStubGitLabClient builds a GitLab client pointed at an in-process HTTP
// stub that answers every request with a fixed version payload. Generators
// and auditors use it to register the tool catalog offline. The returned
// cleanup func shuts the stub server down.
//
// It is [mcpsurface.NewStubClientWithToken] under the name the audit commands
// call it by: one stub, so a command cannot audit a surface built against a
// differently configured client than the one the generators describe.
func NewStubGitLabClient(token string) (client *gitlabclient.Client, cleanup func()) {
	return mcpsurface.NewStubClientWithToken(token)
}
