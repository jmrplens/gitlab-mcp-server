//go:build e2e

// attestations_test.go covers the two attestation actions on a project that
// has published none, on both sides of the feature flag GitLab serves them
// behind.
//
// The flag is why the file is shaped this way. GitLab's attestation routes
// answer 404 for every project slsa_provenance_statement is off for, and the
// flag ships disabled, so on a Docker instance nobody configured the routes
// are never reached at all: a scenario that asserted "lists nothing" there
// passed on the refusal of the route and held nothing about the listing. The
// test therefore asserts both answers in order, the refusal naming the flag
// while it is off, which is the only chance to see the default state, then
// with the flag pinned on an empty listing, a digest GitLab's route would not
// match refused by name, and the download of an attestation that does not
// exist refused with the hint naming the listing to find one in. The pin is
// instance-global, so the test declares the lock and an administrator, and
// the fixture puts the flag back where it found it.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/attestations"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// emptySubjectDigest is a well-formed digest nothing was ever attested
// under, in the OCI spelling the server strips to the 64 hex characters
// GitLab's route takes.
const emptySubjectDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

// TestAttestations_UnattestedProject_RefusedWithTheFlagOffAndListsNothingWithItOn
// lists the attestations of a fresh project under a digest with the feature
// flag off and then on, and asks for one by a number nothing has.
//
// The flag-off half runs on one surface, because it observes a state of the
// instance rather than a path through the catalog; the flag-on half runs on
// every surface.
//
// Replaces: TestMeta_Attestations
func TestAttestations_UnattestedProject_RefusedWithTheFlagOffAndListsNothingWithItOn(t *testing.T) {
	e := harness.New(t,
		harness.Needs(harness.Tier(edition.Ultimate), harness.NeedAdmin),
		harness.Locks(harness.LockInstanceGlobal))
	project := fixture.NewProject(e, fixture.WithNamePrefix("attest"))
	listParams := map[string]any{"project_id": project.IDParam(), "subject_digest": emptySubjectDigest}

	// The halves run in order on this test rather than as subtests, for the
	// reason the Dependency Firewall scenario gives: a harness helper fails
	// through the Env's own T, and a subtest failing the test above it
	// panics the run instead of reporting.
	if fixture.ReadFeature(e, attestations.FeatureFlag).On() {
		e.T.Logf("the instance already holds %s on, so the refusal half has nothing to observe", attestations.FeatureFlag)
	} else {
		refused := harness.Refused(e.On(harness.SurfaceDynamic), actionAttestationList, listParams, harness.FailureNotFound)
		assertMentions(e, "the listing refused with the flag off", refused, attestations.FeatureFlag, "admin.feature_set")
	}

	if !fixture.FeatureDefined(e, attestations.FeatureFlag) {
		// A flag this instance defines nowhere belongs to a release it
		// predates, and no value set for it would make the routes appear.
		e.T.Logf("this GitLab does not define %s, so the routes are not in this release and only the refusal ran",
			attestations.FeatureFlag)
		return
	}

	fixture.PinFeature(e, attestations.FeatureFlag, true)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		listed := harness.Do[attestations.ListOutput](s, actionAttestationList, listParams)
		if len(listed.Attestations) != 0 {
			e.T.Errorf("a project that attested nothing lists %d attestations: %+v", len(listed.Attestations), listed.Attestations)
		}

		// A digest GitLab's route would not match is refused by name: sent
		// as written, GitLab's 404 for it could not be told from the flag.
		harness.ExpectToolError(s, actionAttestationList,
			map[string]any{"project_id": project.IDParam(), "subject_digest": "sha256:abc123"}, "64 hex characters")

		refused := harness.Refused(s, actionAttestationDownload,
			map[string]any{"project_id": project.IDParam(), "attestation_iid": missingID}, harness.FailureNotFound)
		assertMentions(e, "the download of a missing attestation", refused, "attestation_iid", "attestation.list")
	})
}
