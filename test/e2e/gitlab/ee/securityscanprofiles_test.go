//go:build e2e

// securityscanprofiles_test.go covers the scan profile lifecycle on a
// project in a group: attach by a scan type with no default profile and
// read the refusal, attach the built-in dependency scanning profile by its
// name, read the project's statuses to learn the persisted profile's ID,
// and detach it by that ID.
//
// Three facts of GitLab's shape it: a profile attaches only to a project
// under a group namespace, since a personal namespace has no shared root
// for it; attach takes the name of a default profile and finds or creates
// the namespace's copy of it, which not every scan type has; and detach
// takes the persisted profile's numeric ID, which only the status listing
// reveals.

package ee

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/securityscanprofiles"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// dependencyScanning is the scan type whose default profile the test
// attaches, and the one the provisioning script's feature flags enable.
const dependencyScanning = "dependency_scanning"

// scanTypeWithoutDefaultProfile is a scan type GitLab defines and builds no
// default profile for, so attach refuses it by name.
const scanTypeWithoutDefaultProfile = "container_scanning"

// scanProfileNotConfigured is the status GitLab lists a profile with while
// no project holds it attached: the listing enumerates every profile of the
// scan types the project can hold, so a detach shows as this status and
// never as an absence.
const scanProfileNotConfigured = "NOT_CONFIGURED"

// TestSecurityScanProfiles_ProjectInGroup_AttachesListsAndDetaches walks
// the lifecycle once per surface on a project of its own, since a profile
// attached by one surface would already be present for the next.
//
// Replaces: TestMeta_SecurityScanProfiles
func TestSecurityScanProfiles_ProjectInGroup_AttachesListsAndDetaches(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("scanprof"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("scanprof"), fixture.InGroup(group))
		if !strings.HasPrefix(project.Path, group.Path+"/") {
			e.T.Fatalf("project %s is not under group %s, which a scan profile needs", project.Path, group.Path)
		}

		assertAttachRefusesScanTypeWithoutDefaultProfile(e, s, project.ID)

		attached := harness.Do[securityscanprofiles.MutationOutput](s, actionScanProfileAttach, map[string]any{
			"security_scan_profile_id": dependencyScanning, "project_ids": []int64{project.ID},
		})
		if attached.Status != "success" {
			e.T.Fatalf("attach answered status %q, want success: %+v", attached.Status, attached)
		}

		statuses := harness.Do[securityscanprofiles.ListProjectStatusesOutput](s, actionScanProfileListProjectStatuses,
			map[string]any{"project_full_path": project.Path})
		if statuses.ProjectFullPath != project.Path {
			e.T.Errorf("the status listing echoes %q, want %q", statuses.ProjectFullPath, project.Path)
		}
		// The listing names every profile of the scan types the project can
		// hold, attached or not, with a status per profile; what the attach
		// changes is the status, which is what the detach below reverts.
		var profileID, attachedStatus string
		for _, status := range statuses.Statuses {
			if strings.EqualFold(status.ScanProfile.ScanType, dependencyScanning) {
				profileID, attachedStatus = status.ScanProfile.ID, status.Status
			}
		}
		if profileID == "" {
			e.T.Fatalf("the attached %s profile is not among the project's statuses: %+v", dependencyScanning, statuses.Statuses)
		}
		if attachedStatus == scanProfileNotConfigured {
			e.T.Errorf("the attached %s profile is still %s in the project's statuses: %+v", dependencyScanning, attachedStatus, statuses.Statuses)
		}

		detached := harness.Do[securityscanprofiles.MutationOutput](s, actionScanProfileDetach, map[string]any{
			"security_scan_profile_id": profileID, "project_ids": []int64{project.ID},
		})
		if detached.Status != "success" {
			e.T.Errorf("detach answered status %q, want success: %+v", detached.Status, detached)
		}
		// A success answer is the mutation's word; the listing is GitLab's,
		// and a detached profile stays listed with its status back to not
		// configured.
		after := harness.Do[securityscanprofiles.ListProjectStatusesOutput](s, actionScanProfileListProjectStatuses,
			map[string]any{"project_full_path": project.Path})
		detachedStatus := ""
		for _, status := range after.Statuses {
			if status.ScanProfile.ID == profileID {
				detachedStatus = status.Status
			}
		}
		if detachedStatus != scanProfileNotConfigured {
			e.T.Errorf("the detached profile %s reads %q in the project's statuses, want %s (was %s while attached): %+v",
				profileID, detachedStatus, scanProfileNotConfigured, attachedStatus, after.Statuses)
		}
	})
}

// assertAttachRefusesScanTypeWithoutDefaultProfile attaches by
// container_scanning, a scan type GitLab builds no default profile for, and
// holds the refusal to naming the default profiles that attach does take:
// GitLab's own answer is a generic resource-not-available error, and the
// action used to offer this very name.
func assertAttachRefusesScanTypeWithoutDefaultProfile(e *harness.Env, s *harness.Session, projectID int64) {
	e.T.Helper()
	refused, refusal := harness.Try[securityscanprofiles.MutationOutput](s, actionScanProfileAttach, map[string]any{
		"security_scan_profile_id": scanTypeWithoutDefaultProfile, "project_ids": []int64{projectID},
	})
	if refusal == nil {
		e.T.Errorf("attach by %s answered %+v, want GitLab's refusal of a scan type with no default profile", scanTypeWithoutDefaultProfile, refused)
		return
	}
	assertMentions(e, "attach by "+scanTypeWithoutDefaultProfile, refusal.Error(),
		securityscanprofiles.DefaultProfileNames, "have no default profile")
}
