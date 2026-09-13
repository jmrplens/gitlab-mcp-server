//go:build e2e

// attestations_test.go covers the two attestation actions on a project that
// has published none: the listing by subject digest answers empty, and the
// download of an attestation that does not exist is refused with the hint
// naming the listing to find one in.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/attestations"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// emptySubjectDigest is a well-formed digest nothing was ever attested
// under.
const emptySubjectDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

// TestAttestations_UnattestedProject_ListsNothingAndRefusesADownload lists
// the attestations of a fresh project under a digest and asks for one by a
// number nothing has, on every surface.
//
// Replaces: TestMeta_Attestations
func TestAttestations_UnattestedProject_ListsNothingAndRefusesADownload(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("attest"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)

		listed := harness.Do[attestations.ListOutput](s, actionAttestationList,
			map[string]any{"project_id": project.IDParam(), "subject_digest": emptySubjectDigest})
		if len(listed.Attestations) != 0 {
			e.T.Errorf("a project that attested nothing lists %d attestations: %+v", len(listed.Attestations), listed.Attestations)
		}

		refused := harness.Refused(s, actionAttestationDownload,
			map[string]any{"project_id": project.IDParam(), "attestation_iid": missingID}, harness.FailureNotFound)
		assertMentions(e, "the download of a missing attestation", refused, "attestation_iid", "gitlab_attestation", "gitlab_list_attestations")
	})
}
