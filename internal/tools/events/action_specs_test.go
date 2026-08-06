// action_specs_test.go contains unit tests for the events ActionSpec
// metadata, verifying every action carries a description.
package events

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestUserActionSpecs_Descriptions verifies the R-META individual-tool
// descriptions follow the "Returns: … See also: …" form.
func TestUserActionSpecs_Descriptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	for _, spec := range UserActionSpecs(client) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			desc := spec.IndividualTool.Description
			if desc == "" {
				t.Fatal("empty description")
			}
			if !contains(desc, "Returns:") || !contains(desc, "See also:") {
				t.Errorf("description missing Returns/See also: %q", desc)
			}
		})
	}
}
