// group_integration_test.go holds the unit test of integrationPath, the one
// place the project and group integration handlers build the path of a single
// integration. The handlers themselves are driven through httptest in
// set_integration_test.go.
package integrations

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestIntegrationPath_AwkwardSlug_StaysOneSegment asserts that the scope id and
// the slug are each escaped into exactly one segment, for both scopes.
//
// The slug is the reason this exists. It is free text a caller types, so one
// carrying a slash names another endpoint of the same scope unless it is
// escaped, and no request a real integration makes can show whether it was:
// every slug GitLab has is lowercase letters and hyphens, which escaping leaves
// as they are. So the property is held here, on the function the four raw
// requests share, rather than through a request whose slug no integration has,
// which the request inventory would record as an endpoint GitLab does not
// have. The dots are written "%2E" because gl.PathEscape writes them that way;
// the transport restores them on the wire, which the handler tests assert.
func TestIntegrationPath_AwkwardSlug_StaysOneSegment(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		id     toolutil.StringOrInt
		slug   string
		want   string
	}{
		{
			name:   "a group path and a slug that climbs out of its endpoint",
			prefix: groupsPathPrefix,
			id:     "100% my group/sub.group",
			slug:   "../users/1 %",
			want:   "groups/100%25%20my%20group%2Fsub%2Egroup/integrations/%2E%2E%2Fusers%2F1%20%25",
		},
		{
			name:   "a project id and a slug naming a sibling integration",
			prefix: projectsPathPrefix,
			id:     "42",
			slug:   "jira/../slack",
			want:   "projects/42/integrations/jira%2F%2E%2E%2Fslack",
		},
		{
			name:   "a slug GitLab has, which escaping leaves as it is",
			prefix: groupsPathPrefix,
			id:     "7",
			slug:   "custom-issue-tracker",
			want:   "groups/7/integrations/custom-issue-tracker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := integrationPath(tt.prefix, tt.id, tt.slug)
			if got != tt.want {
				t.Errorf("integrationPath(%q, %q, %q) = %q, want %q", tt.prefix, tt.id, tt.slug, got, tt.want)
			}
			if segments := strings.Split(got, "/"); len(segments) != 4 {
				t.Errorf("integrationPath(%q, %q, %q) = %q, %d segments, want 4 whatever the values carry",
					tt.prefix, tt.id, tt.slug, got, len(segments))
			}
		})
	}
}
